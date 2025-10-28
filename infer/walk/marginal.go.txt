// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walk

import (
	"runtime"
	"sync"

	"github.com/js-arias/earth"
)

type margChanType struct {
	start []float64
	end   []float64
	raw   []float64

	w     *walkModel
	age   int64
	tr    int
	steps []int

	answer chan likeChanAnswer
}

var margChanMutex sync.Mutex
var openMargChan bool
var margChan chan margChanType

// StartUp prepares the package for an up-pass.
// Use cpu to define the number of process
// used for the reconstruction.
// The default (zero) uses all available CPU.
// After all optimization is done,
// use EndUp to close the goroutines.
func StartUp(cpu int, pix *earth.Pixelation) {
	margChanMutex.Lock()
	defer margChanMutex.Unlock()

	if openMargChan {
		return
	}

	if cpu == 0 {
		cpu = runtime.NumCPU()
	}

	margChan = make(chan margChanType, cpu*2)
	for range cpu {
		go upMarginal(margChan, pix.Len())
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

func upMarginal(c chan margChanType, sz int) {
	prev := make([]float64, sz)
	curr := make([]float64, sz)
	for cc := range c {
		for _, s := range cc.steps {
			copy(curr, cc.start)
			stepMarg := catMarginal(cc.w, prev, curr, cc.age, cc.tr, s)
			for px, p := range stepMarg {
				cc.raw[px] += p * cc.end[px]
			}
		}
		cc.answer <- likeChanAnswer{
			rawLike: cc.raw,
			tr:      cc.tr,
		}
	}
}

func catMarginal(w *walkModel, prev, curr []float64, age int64, tr, steps int) []float64 {
	stage := w.stage(age, tr)
	for range steps {
		prev, curr = curr, prev
		for px := range curr {
			curr[px] = 0
		}
		for px, p := range prev {
			for _, nx := range stage.move[px] {
				curr[nx.id] += nx.prob * p
			}
		}
	}
	return curr
}
