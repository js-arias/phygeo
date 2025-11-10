// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walk

import (
	"runtime"
	"sync"

	"github.com/js-arias/earth"
)

var margChanMutex sync.Mutex
var openMargChan bool
var margChan chan likeChanType

// StartUp prepares the package for an up-pass.
// Use cpu to define the number of process
// used for the reconstruction.
// The default (zero) uses all available CPU.
// After all optimization is done,
// use EndUp to close the goroutines.
func StartUp(cpu int, pix *earth.Pixelation, traits int) {
	margChanMutex.Lock()
	defer margChanMutex.Unlock()

	if openMargChan {
		return
	}

	if cpu == 0 {
		cpu = runtime.NumCPU()
	}

	margChan = make(chan likeChanType, cpu*2)
	for range cpu {
		go upMarginal(margChan, pix.Len(), traits)
	}
	openMargChan = true
}

// EndUp closes the goroutines used for the up-pass.
func EndUp() {
	margChanMutex.Lock()
	defer margChanMutex.Unlock()
	close(margChan)
	openMargChan = false
}

func upMarginal(c chan likeChanType, sz, traits int) {
	prev := make([][]float64, traits)
	curr := make([][]float64, traits)
	for i := range prev {
		prev[i] = make([]float64, sz)
		curr[i] = make([]float64, sz)
	}
	for cc := range c {
		for i := range curr {
			copy(curr[i], cc.like[i])
		}
		stepMarg := catMarginal(cc.w, prev, curr, cc.age, cc.steps)
		for i := range stepMarg {
			copy(cc.raw[i], stepMarg[i])
		}
		cc.answer <- likeChanAnswer{
			rawLike: cc.raw,
			cat:     cc.cat,
		}
	}
}

func catMarginal(w *walkModel, prev, curr [][]float64, age int64, steps int) [][]float64 {
	for range steps {
		for i := range prev {
			prev[i], curr[i] = curr[i], prev[i]

			// reset values
			for px := range curr[i] {
				curr[i][px] = 0
			}
		}
		for i := range prev {
			stage := w.stage(age, i)
			for px, p := range prev[i] {
				for _, nx := range stage.move[px] {
					curr[i][nx.id] += p * nx.prob
				}
			}
		}
	}
	return curr
}
