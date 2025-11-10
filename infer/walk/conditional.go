// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walk

import (
	"runtime"
	"sync"

	"github.com/js-arias/earth"
)

type likeChanType struct {
	like [][]float64
	raw  [][]float64

	w     *walkModel
	age   int64
	cat   int
	steps int

	answer chan likeChanAnswer
}

var likeChanMutex sync.Mutex
var openLikeChan bool
var likeChan chan likeChanType

// StartDown prepares the package for a down-pass.
// Use cpu to define the number of process
// used for the reconstruction.
// The default (zero) uses all available CPU.
// After all optimization is done,
// use EndDown to close the goroutines.
func StartDown(cpu int, pix *earth.Pixelation, traits int) {
	likeChanMutex.Lock()
	defer likeChanMutex.Unlock()

	if openLikeChan {
		return
	}

	if cpu == 0 {
		cpu = runtime.NumCPU()
	}

	likeChan = make(chan likeChanType, cpu*2)
	for range cpu {
		go downLike(likeChan, pix.Len(), traits)
	}
	openLikeChan = true
}

// EndDown closes the goroutines used for the down-pass.
func EndDown() {
	likeChanMutex.Lock()
	defer likeChanMutex.Unlock()
	close(likeChan)
	openLikeChan = false
}

type likeChanAnswer struct {
	rawLike [][]float64
	cat     int
}

func downLike(c chan likeChanType, sz, traits int) {
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
		stepLike := catConditional(cc.w, prev, curr, cc.age, cc.steps)
		for i := range stepLike {
			copy(cc.raw[i], stepLike[i])
		}
		cc.answer <- likeChanAnswer{
			rawLike: cc.raw,
			cat:     cc.cat,
		}
	}
}

func catConditional(w *walkModel, prev, curr [][]float64, age int64, steps int) [][]float64 {
	for range steps {
		for i := range prev {
			prev[i], curr[i] = curr[i], prev[i]
		}
		for i := range curr {
			stage := w.stage(age, i)
			for px := range curr[i] {
				var sum float64
				for _, nx := range stage.move[px] {
					sum += nx.prob * prev[i][nx.id]
				}
				curr[i][px] = sum
			}
		}
	}
	return curr
}
