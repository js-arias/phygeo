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
	like []float64
	raw  []float64

	w     *walkModel
	age   int64
	tr    int
	steps []int

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
func StartDown(cpu int, pix *earth.Pixelation) {
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
		go downLike(likeChan, pix.Len())
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
	rawLike []float64
	tr      int
}

func downLike(c chan likeChanType, sz int) {
	prev := make([]float64, sz)
	curr := make([]float64, sz)
	for cc := range c {
		for _, s := range cc.steps {
			copy(curr, cc.like)
			stepLike := catConditional(cc.w, prev, curr, cc.age, cc.tr, s)
			for px, p := range stepLike {
				cc.raw[px] += p
			}
		}
		cc.answer <- likeChanAnswer{
			rawLike: cc.raw,
			tr:      cc.tr,
		}
	}
}

func catConditional(w *walkModel, prev, curr []float64, age int64, tr, steps int) []float64 {
	stage := w.stage(age, tr)
	for range steps {
		prev, curr = curr, prev
		for px := range curr {
			var sum float64
			for _, nx := range stage.move[px] {
				sum += nx.prob * prev[nx.id]
			}
			curr[px] = sum
		}
	}
	return curr
}
