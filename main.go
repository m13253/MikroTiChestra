/*
  MIT License
  Copyright (c) 2020 Star Brilliant
  Permission is hereby granted, free of charge, to any person obtaining a copy
  of this software and associated documentation files (the "Software"), to deal
  in the Software without restriction, including without limitation the rights
  to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
  copies of the Software, and to permit persons to whom the Software is
  furnished to do so, subject to the following conditions:
  The above copyright notice and this permission notice shall be included in
  all copies or substantial portions of the Software.
  THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
  IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
  FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
  AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
  LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
  OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
  SOFTWARE.
*/

package main

import (
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/m13253/midimark"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type application struct {
	conf config

	knownHosts ssh.HostKeyCallback
	songs      []song
}

type song struct {
	Sequence *midimark.Sequence
	Duration time.Duration
}

type debugEvent struct {
	Hostname    string
	RawLine     string
	Frequency   float64
	LengthMilli int64
	OnFinished  *sync.WaitGroup
}

func main() {
	fmt.Println("         MikroTiChestra")
	fmt.Println("Copyright (c) 2020 Star Brilliant")
	fmt.Println("=================================")
	fmt.Println()

	app := &application{}
	flag.StringVar(&app.conf.ConfigFile, "conf", "MikroTiChestra.conf", "Configure file path")
	flag.Parse()
	app.run()

	fmt.Println()
	fmt.Println("=================================")
	fmt.Println("         MikroTiChestra")
	fmt.Println("Copyright (c) 2020 Star Brilliant")
}

func (app *application) run() {
	fmt.Printf("Loading configuration file: %s\n", app.conf.ConfigFile)
	err := app.conf.parseConfigFile()
	if err != nil {
		fmt.Printf("Failed to load config file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Loading known_hosts file: %s\n", app.conf.KnownHosts)
	app.knownHosts, err = knownhosts.New(app.conf.KnownHosts)
	if err != nil {
		fmt.Printf("Failed to load known_hosts: %v\n", err)
		os.Exit(1)
	}

	midiFiles := flag.Args()
	if len(midiFiles) == 0 {
		fmt.Println()
		fmt.Println("Please specify which MIDI files to load using command line arguments.")
		os.Exit(1)
	}

	totalDuration := time.Duration(0)
	for _, filename := range midiFiles {
		fmt.Printf("Loading %s\n", filename)
		seq, err := app.loadMIDIFile(filename)
		if err != nil {
			fmt.Printf("%s: %v\n", filename, err)
			os.Exit(1)
		}
		duration := app.determineSongDuration(seq)
		app.songs = append(app.songs, song{seq, duration})
		totalDuration += duration
	}
	fmt.Printf("Total duration: %v\n", totalDuration)

	var onConnected sync.WaitGroup
	onConnected.Add(len(app.conf.Connections))
	startTimeChan := make(chan time.Time, len(app.conf.Connections))
	var onFinished sync.WaitGroup
	onFinished.Add(len(app.conf.Connections))
	printDebugChan := make(chan debugEvent, 2*len(app.conf.Connections))
	var onDebugPrinterFinished sync.WaitGroup
	onDebugPrinterFinished.Add(1)
	app.debugEventPrinter(printDebugChan, &onDebugPrinterFinished)

	for _, connConf := range app.conf.Connections {
		c := &connection{
			AppConf:         &app.conf,
			ConnConf:        connConf,
			KnownHosts:      app.knownHosts,
			Songs:           app.songs,
			PrintDebugEvent: printDebugChan,
			OnConnected:     &onConnected,
			StartTime:       startTimeChan,
		}
		go func(c *connection, onFinished *sync.WaitGroup) {
			defer onFinished.Done()
			err := c.Start()
			if err != nil {
				var wg sync.WaitGroup
				c.PrintDebugEvent <- debugEvent{
					Hostname:   c.ConnConf.Name,
					RawLine:    err.Error(),
					OnFinished: &wg,
				}
				wg.Wait()
				os.Exit(1)
			}
		}(c, &onFinished)
	}

	onConnected.Wait()
	startTime := time.Now().Add(app.conf.InitialDelay)
	for i := 0; i < len(app.conf.Connections); i++ {
		startTimeChan <- startTime
	}
	close(startTimeChan)

	onFinished.Wait()
	close(printDebugChan)
	onDebugPrinterFinished.Wait()
}

func (app *application) loadMIDIFile(filename string) (*midimark.Sequence, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return midimark.DecodeSequenceFromSMF(f, func(err error) {
		fmt.Printf("%s: %v\n", filename, err)
	})
}

func (app *application) determineSongDuration(seq *midimark.Sequence) time.Duration {
	maxDuration := time.Duration(0)
	for _, mtrk := range seq.Tracks {
		if len(mtrk.Events) == 0 {
			continue
		}
		maxTick := mtrk.Events[len(mtrk.Events)-1].Common().AbsTick
		duration := mtrk.ConvertAbsTickToDuration(maxTick)
		if duration > maxDuration {
			maxDuration = duration
		}
	}
	return maxDuration
}

func (app *application) debugEventPrinter(ch <-chan debugEvent, wg *sync.WaitGroup) {
	go func(ch <-chan debugEvent, wg *sync.WaitGroup) {
		colorGreen := color.New(color.FgGreen)
		colorCyan := color.New(color.FgCyan)
		colorMagenta := color.New(color.FgMagenta)
		colorYellow := color.New(color.FgYellow)
		for e := range ch {
			color.Unset()
			if e.Hostname != "" {
				fmt.Print("[")
				colorCyan.Print(e.Hostname)
				fmt.Print("] ")
			}
			if e.RawLine != "" {
				fmt.Println(e.RawLine)
			} else {
				fmt.Print("> ")
				colorCyan.Print(":")
				colorMagenta.Print("beep")
				fmt.Print(" ")
				colorGreen.Print("as-value")
				fmt.Print(" ")
				colorGreen.Print("frequency")
				colorYellow.Print("=")
				fmt.Printf("%-5.0f ", e.Frequency)
				colorGreen.Print("length")
				colorYellow.Print("=")
				fmt.Printf("%dms", e.LengthMilli)
				colorYellow.Print(";")
				fmt.Println()
			}
			if e.OnFinished != nil {
				e.OnFinished.Done()
			}
		}
		wg.Done()
	}(ch, wg)
}
