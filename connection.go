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
	"bufio"
	"fmt"
	"io"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/m13253/midimark"
	"golang.org/x/crypto/ssh"
)

const DefaultTimeout = 1 * time.Minute

type connection struct {
	AppConf    *config
	ConnConf   *connConfig
	KnownHosts ssh.HostKeyCallback
	Songs      []song

	PrintDebugEvent chan<- debugEvent
	OnConnected     *sync.WaitGroup
	StartTime       <-chan time.Time
}

type note struct {
	SongID    int
	Event     midimark.Event
	MTrk      *midimark.MTrk
	SongStart time.Duration
}

func (c *connection) Start() error {
	notes := c.loadNotes()

	port := c.ConnConf.Port
	if port == "" {
		port = "22"
	}
	addr := net.JoinHostPort(c.ConnConf.Host, port)
	sshConf := &ssh.ClientConfig{
		User: c.ConnConf.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(c.ConnConf.Password),
		},
		HostKeyCallback: c.KnownHosts,
		BannerCallback: func(message string) error {
			sc := bufio.NewScanner(strings.NewReader(message))
			for sc.Scan() {
				c.PrintDebugEvent <- debugEvent{
					Hostname: c.ConnConf.Name,
					RawLine:  sc.Text(),
				}
			}
			return nil
		},
		Timeout: DefaultTimeout,
	}

	c.PrintDebugEvent <- debugEvent{
		Hostname: c.ConnConf.Name,
		RawLine:  fmt.Sprintf("Connecting to %s", addr),
	}

	sshClient, err := ssh.Dial("tcp", addr, sshConf)
	if err != nil {
		c.OnConnected.Done()
		<-c.StartTime
		return err
	}
	defer sshClient.Close()

	sshSession, err := sshClient.NewSession()
	if err != nil {
		c.OnConnected.Done()
		<-c.StartTime
		return err
	}
	defer sshSession.Close()

	var stdoutFinished sync.WaitGroup
	stdoutFinished.Add(1)
	stdout := c.pipeToStdout(&stdoutFinished)
	defer stdoutFinished.Wait()
	defer stdout.Close()
	sshSession.Stdout = stdout
	sshSession.Stderr = stdout

	var stdin *io.PipeWriter
	sshSession.Stdin, stdin = io.Pipe()

	err = sshSession.Shell()
	if err != nil {
		stdin.Close()
		c.OnConnected.Done()
		<-c.StartTime
		return err
	}
	defer sshSession.Wait()
	defer stdin.Close()

	c.PrintDebugEvent <- debugEvent{
		Hostname: c.ConnConf.Name,
		RawLine:  fmt.Sprintf("Connected to %s", addr),
	}
	c.OnConnected.Done()

	startTime, ok := <-c.StartTime
	if !ok {
		panic("internal error: start time is invalid")
	}

	currentRPN := uint16(0x0000)
	currentData := uint16(0xffff)
	RPN := [3]uint16{
		0x00: 0x0100, // Pitch wheel range: (value>>7)+(value&0x7f)/100 semitones
		0x01: 0x2000, // Fine tuning: (value-0x2000)/8192 semitones
		0x02: 0x2000, // Coarse tuning: (value>>7)-0x40 semitones
	}
	pitchWheel := int16(0)

	for _, note := range notes {
		songAbsTick := note.Event.Common().AbsTick
		songAbsTime := note.MTrk.ConvertAbsTickToDuration(songAbsTick)
		startAbsTime := note.SongStart + songAbsTime
		durationToSleep := startAbsTime - time.Now().Sub(startTime)
		time.Sleep(durationToSleep)

		switch event := note.Event.(type) {
		case *midimark.EventNoteOn:
			pitchWheelRange := float64(RPN[0]>>7) + float64(RPN[0]&0x7f)/100
			fineTuning := (float64(RPN[1]) - 0x2000) / 8192
			coarseTuning := float64(RPN[2]>>7) - 0x40

			pitch := float64(event.Key) + float64(pitchWheel)*float64(pitchWheelRange)/8192 + fineTuning + coarseTuning
			frequency := midiNoteToHertz(pitch)
			var length time.Duration
			if event.RelatedNoteOff != nil {
				songAbsTickOff := event.RelatedNoteOff.AbsTick
				songAbsTimeOff := note.MTrk.ConvertAbsTickToDuration(songAbsTickOff)
				length = songAbsTimeOff - songAbsTime
			} else {
				length = 1 * time.Second
			}
			if length <= 0 {
				continue
			}
			lengthMilli := int64((length + 999999*time.Nanosecond) / time.Millisecond)
			_, err := fmt.Fprintf(stdin, ":beep frequency=%.0f length=%dms as-value;\n", frequency, lengthMilli)
			if err != nil {
				return err
			}
			c.PrintDebugEvent <- debugEvent{
				Hostname:    c.ConnConf.Name,
				Frequency:   frequency,
				LengthMilli: lengthMilli,
			}
		case *midimark.EventPitchWheelChange:
			pitchWheel = event.Pitch
		case *midimark.EventControlChange:
			switch event.Control {
			case 0x06: // Data entry MSB
				currentData = uint16(event.Value)<<7 | (currentData & 0x407f)
				if (currentData&0xc000) == 0 && currentRPN <= 2 {
					RPN[currentRPN] = currentData
				}
			case 0x26: // Data entry LSB
				currentData = (currentData & 0xbf80) | uint16(event.Value)
				if (currentData&0xc000) == 0 && currentRPN <= 2 {
					RPN[currentRPN] = currentData
				}
			case 0x60: // Data +1
				if currentRPN <= 2 {
					RPN[currentRPN] = (RPN[currentRPN] + 1) & 0x3fff
				}
			case 0x61: // Data -1
				if currentRPN <= 2 {
					RPN[currentRPN] = (RPN[currentRPN] - 1) & 0x3fff
				}
			case 0x64: // RPN LSB
				currentRPN = (currentRPN & 0xbf80) | uint16(event.Value)
			case 0x65: // RPN MSB
				currentRPN = uint16(event.Value)<<7 | (currentRPN & 0x407f)
			}
		}
	}

	c.PrintDebugEvent <- debugEvent{
		Hostname: c.ConnConf.Name,
		RawLine:  "Closing connection",
	}
	return nil
}

func (c *connection) loadNotes() []note {
	songStart := time.Duration(0)
	var notes []note
	for songID, song := range c.Songs {
		for trackID, mtrk := range song.Sequence.Tracks {
			if _, ok := c.ConnConf.Tracks.Map[uint16(trackID)]; !ok {
				if !c.ConnConf.Tracks.OtherTracks {
					continue
				}
				if _, ok := c.AppConf.TracksDefined[uint16(trackID)]; ok {
					continue
				}
			}
			for _, event := range mtrk.Events {
				switch event := event.(type) {
				case *midimark.EventNoteOn, *midimark.EventPitchWheelChange:
				case *midimark.EventControlChange:
					switch event.Control {
					case 0x06: // Data entry MSB
					case 0x26: // Data entry LSB
					case 0x60: // Data +1
					case 0x61: // Data -1
					case 0x64: // RPN LSB
					case 0x65: // RPN MSB
					default:
						continue
					}
				default:
					continue
				}
				notes = append(notes, note{
					SongID:    songID,
					Event:     event,
					MTrk:      mtrk,
					SongStart: songStart,
				})
			}
		}
		songStart += song.Duration
	}
	sort.Slice(notes, func(i, j int) bool {
		if notes[i].SongID < notes[j].SongID {
			return true
		}
		if notes[i].SongID == notes[j].SongID {
			commonI, commonJ := notes[i].Event.Common(), notes[j].Event.Common()
			return commonI.AbsTick < commonJ.AbsTick ||
				(commonI.AbsTick == commonJ.AbsTick && commonI.FilePosition == commonJ.FilePosition)
		}
		return false
	})
	return notes
}

func (c *connection) pipeToStdout(wg *sync.WaitGroup) *io.PipeWriter {
	r, w := io.Pipe()
	go func(r *io.PipeReader, name string, wg *sync.WaitGroup) {
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			c.PrintDebugEvent <- debugEvent{
				Hostname: c.ConnConf.Name,
				RawLine:  sc.Text(),
			}
		}
		r.Close()
		wg.Done()
	}(r, c.ConnConf.Name, wg)
	return w
}
