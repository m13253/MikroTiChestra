/*
  MIT License
  Copyright (c) 2018, 2020 Star Brilliant
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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type config struct {
	ConfigFile   string
	KnownHosts   string
	InitialDelay time.Duration
	Connections  []*connConfig

	TracksDefined      map[uint16]struct{}
	OtherTracksDefined bool
}

type connConfig struct {
	Name string

	Tracks   connTracksConfig
	Host     string
	Port     string
	Username string
	Password string
}

type connTracksConfig struct {
	Map         map[uint16]struct{}
	OtherTracks bool
}

func (conf *config) parseConfigFile() error {
	f, err := os.Open(conf.ConfigFile)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := bufio.NewReader(f)

	currentConn := conf.newConnection()
	currentConnValid := false
	if conf.TracksDefined == nil {
		conf.TracksDefined = make(map[uint16]struct{})
	}

	for {
		line, lineerr := buf.ReadString('\n')
		if lineerr != nil {
			if lineerr == io.EOF {
				break
			}
			return err
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			if lineerr == io.EOF {
				break
			}
			continue
		}
		// I do not want to use reflect.Value, they are too ugly
		switch fields[0] {
		case "KnownHosts":
			err = conf.parseConfigString(fields, &conf.KnownHosts)
			if err == nil {
				conf.KnownHosts = os.ExpandEnv(conf.KnownHosts)
			}
		case "InitialDelay":
			err = conf.parseConfigDuration(fields, &conf.InitialDelay)
		case "Connection":
			if currentConnValid {
				err = conf.appendConnection(currentConn)
				if err != nil {
					return err
				}
				currentConn = conf.newConnection()
			} else {
				currentConnValid = true
			}
			err = conf.parseConfigString(fields, &currentConn.Name)
		case "Track":
			currentConnValid = true
			err = conf.parseConfigTracks(fields, &currentConn.Tracks)
		case "Host":
			currentConnValid = true
			err = conf.parseConfigString(fields, &currentConn.Host)
		case "Port":
			currentConnValid = true
			err = conf.parseConfigString(fields, &currentConn.Port)
		case "Username":
			currentConnValid = true
			err = conf.parseConfigString(fields, &currentConn.Username)
		case "Password":
			currentConnValid = true
			err = conf.parseConfigString(fields, &currentConn.Password)
		}
		if err != nil {
			return err
		}
	}

	if conf.KnownHosts == "" {
		home, ok := os.LookupEnv("HOME")
		if !ok {
			home, ok = os.LookupEnv("USERPROFILE")
		}
		conf.KnownHosts = filepath.Join(home, ".ssh", "known_hosts")
	}

	if currentConnValid {
		err = conf.appendConnection(currentConn)
		if err != nil {
			return err
		}
	} else {
		return errors.New("no SSH connections configured")
	}
	if len(conf.TracksDefined) == 0 && !conf.OtherTracksDefined {
		fmt.Println("Warning: no tracks configured")
	} else if !conf.OtherTracksDefined {
		fmt.Println("Warning: no SSH connections set to \"Track Other\"")
	}

	return nil
}

func (conf *config) newConnection() *connConfig {
	return &connConfig{
		Tracks: connTracksConfig{
			Map: make(map[uint16]struct{}),
		},
	}
}

func (conf *config) appendConnection(currentConn *connConfig) error {
	if currentConn.Host == "" {
		if currentConn.Name == "" {
			return fmt.Errorf("Host not defined for connection (unnamed)")
		}
		return fmt.Errorf("Host not defined for connection %q", currentConn.Name)
	}
	if currentConn.Name == "" {
		currentConn.Name = currentConn.Host
	}
	conf.Connections = append(conf.Connections, currentConn)
	return nil
}

func (conf *config) parseConfigDuration(fields []string, dest *time.Duration) error {
	if len(fields) != 2 {
		return fmt.Errorf("syntax error in option %q", fields[0])
	}
	duration, err := time.ParseDuration(fields[1])
	if err != nil {
		return err
	}
	if duration < 0 {
		return errors.New("duration is negative")
	}
	*dest = duration
	return nil
}

func (conf *config) parseConfigTracks(fields []string, dest *connTracksConfig) error {
	for i := 1; i < len(fields); i++ {
		if fields[i] == "Other" {
			dest.OtherTracks = true
			conf.OtherTracksDefined = true
		} else {
			value, err := strconv.ParseUint(fields[1], 0, 16)
			if err != nil {
				return err
			}
			trackID := uint16(value)
			dest.Map[trackID] = struct{}{}
			conf.TracksDefined[trackID] = struct{}{}
		}
	}
	return nil
}

func (conf *config) parseConfigString(fields []string, dest *string) error {
	if len(fields) > 2 {
		return fmt.Errorf("space is not allowed in option %q", fields[0])
	}
	if len(fields) == 1 {
		*dest = ""
	} else {
		*dest = fields[1]
	}
	return nil
}
