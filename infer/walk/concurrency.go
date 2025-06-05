// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walk

import (
	"runtime"
	"sync"
	"time"
)

type likeChanType struct {
	start, end int
	like       [][]float64
	scaleProb  [][]float64
	maxLn      float64

	w     *walkModel
	age   int64
	steps []int

	walkers int
	times   int

	wg *sync.WaitGroup
}

var likeChan chan likeChanType

// Start prepares the package for a down-pass.
// Use cpu to define the number of process
// used for the reconstruction.
// The default (zero) uses all available CPU.
// After all optimization is done,
// use End to close the goroutines.
func Start(cpu int) {
	if cpu == 0 {
		cpu = runtime.NumCPU()
	}
	likeChan = make(chan likeChanType, cpu*2)
	for range cpu {
		go runPixLike()
	}
}

// End closes the goroutines used for the down-pass.
func End() {
	close(likeChan)
	time.Sleep(10 * time.Second)
}
