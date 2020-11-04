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

import "math"

// Notes are tuned using Equal Temperament.
// MikroTik restricts the frequency in 20 Hz - 20,000 Hz.
// Therefore, frequencies beyond this range are substituted using its harmonics.
func midiNoteToHertz(note float64) float64 {
	freq := 440 * math.Pow(2, (note-69)/12)
	if freq < 20 {
		if freq >= 20/3 {
			return freq * 3
		}
		if freq >= 4 {
			return freq * 5
		}
		return 20
	}
	if freq > 20000 {
		if freq <= 60000 {
			return freq / 3
		}
		if freq <= 100000 {
			return freq / 5
		}
		return 20000
	}
	return freq
}
