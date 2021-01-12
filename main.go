package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/mum4k/termdash"
	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/container"
	"github.com/mum4k/termdash/linestyle"
	"github.com/mum4k/termdash/terminal/tcell"
	"github.com/mum4k/termdash/terminal/terminalapi"
	"github.com/mum4k/termdash/widgets/linechart"
	"github.com/phayes/permbits"
)

func sineInputs() []float64 {
	var res []float64

	for i := 0; i < 200; i++ {
		v := math.Sin(float64(i) / 100 * math.Pi)
		res = append(res, v)
	}
	return res
}

func playLineChart(ctx context.Context, lc *linechart.LineChart, delay time.Duration, values chan float64) {

	ticker := time.NewTicker(delay)
	defer ticker.Stop()

	var yAxis []float64
	for {
		select {
		case <-ticker.C:
			value := <-values

			yAxis = append(yAxis, value)
			lc.Series("time", yAxis,
				linechart.SeriesCellOpts(cell.FgColor(cell.ColorNumber(33))),
			)
		case <-ctx.Done():
			return
		}
	}
}

func main() {
	interval, _ := time.ParseDuration("1s")
	if len(os.Args) >= 1 {
		if d, err := time.ParseDuration(os.Args[1]); err != nil {
			panic(err)
		} else {
			interval = d
		}
	}

	maxTime := 3 * time.Second
	if len(os.Args) >= 2 {
		if d, err := time.ParseDuration(os.Args[2]); err != nil {
			panic(err)
		} else {
			maxTime = d
		}
	}
	domain := ""
	inputFile, _ := ioutil.TempFile("", "")
	if len(os.Args) >= 3 {
		userFile := os.Args[3]
		if strings.Index(userFile, ".") > 0 {
			domain = userFile
		} else if _, err := os.Stat(userFile); os.IsNotExist(err) {
			inputFile, _ = os.Create(userFile)
		} else {
			inputFile, _ = os.Open(userFile)
		}
	}

	outputFile, _ := ioutil.TempFile("", "")

	if domain == "" {
		nanoPath, _ := exec.LookPath("nano")
		if proc, err := os.StartProcess(nanoPath, []string{"nano", inputFile.Name()}, &os.ProcAttr{
			Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		}); err != nil {
			panic(err)
		} else if _, err = proc.Wait(); err != nil {
			panic(err)
		}
	} else {
		ioutil.WriteFile(inputFile.Name(), []byte("curl "+domain), 06)
	}

	var buf []byte
	if bytes, err := ioutil.ReadFile(inputFile.Name()); err != nil {
		panic(err)
	} else {
		buf = bytes
	}

	curlOrig := strings.TrimSpace(string(buf))
	curlLines := strings.Split(curlOrig, "\n")
	var betterCurlLines []string
	for _, line := range curlLines {
		betterCurlLines = append(betterCurlLines, strings.TrimSuffix(line, "\\"))
	}

	title := " " + strings.Split(curlOrig, " ")[1] + " "

	curlArgs := []string{
		"-L", "--silent", "-o /dev/null",
		"-w '%{time_starttransfer}\\n'", "--max-time " + fmt.Sprintf("%f", maxTime.Seconds()),
	}
	curlCmd := strings.Join(append(betterCurlLines, curlArgs...), " \\\n")
	ioutil.WriteFile(outputFile.Name(), []byte(curlCmd), 0)

	bits := permbits.UserRead
	bits.SetUserExecute(true)

	if err := os.Chmod(outputFile.Name(), os.FileMode(bits)); err != nil {
		panic(err)
	}

	values := make(chan float64, 100)
	go func() {
		for {
			var stdout bytes.Buffer
			cmd := exec.Command("/usr/bin/env", "sh", "-c", outputFile.Name())
			cmd.Stdout = &stdout

			if err := cmd.Run(); err != nil {
				values <- -1.0
				continue
			} else {
				out := strings.TrimSuffix(string(stdout.String()), "\n")
				if value, err := strconv.ParseFloat(out, 64); err != nil {
					values <- -2.0
				} else {
					values <- value
				}
			}

			time.Sleep(interval)
		}
	}()

	t, err := tcell.New()
	if err != nil {
		panic(err)
	}
	defer t.Close()

	const redrawInterval = 100 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	lc, err := linechart.New(
		linechart.AxesCellOpts(cell.FgColor(cell.ColorRed)),
		linechart.YLabelCellOpts(cell.FgColor(cell.ColorGreen)),
		linechart.XLabelCellOpts(cell.FgColor(cell.ColorCyan)),
	)

	if err != nil {
		panic(err)
	}
	go playLineChart(ctx, lc, redrawInterval/3, values)
	c, err := container.New(
		t,
		container.Border(linestyle.Light),
		container.BorderTitle(title),
		container.PlaceWidget(lc),
	)
	if err != nil {
		panic(err)
	}

	quitter := func(k *terminalapi.Keyboard) {
		if k.Key == 'q' || k.Key == 'Q' {
			cancel()
		}
	}

	if err := termdash.Run(ctx, t, c, termdash.KeyboardSubscriber(quitter), termdash.RedrawInterval(redrawInterval)); err != nil {
		panic(err)
	}
}
