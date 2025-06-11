// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walk

import (
	"runtime"
	"sync"

	"github.com/js-arias/earth/model"
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
}

type pathChanType struct {
	start, end int
	src        []int
	t          []int
	density    [][]float64
	path       []*Path

	w     *walkModel
	rot   *model.Rotation
	age   int64
	steps []int

	wg *sync.WaitGroup
}

// UpPass sends particles to approximate the empirical pixel posterior
// of all nodes.
// Use cpu to defined the number of process
// used for the estimation.
// The default (zero) uses all available CPU.
func (t *Tree) UpPass(cpu, particles int) {
	if cpu == 0 {
		cpu = runtime.NumCPU()
	}

	maxSteps := 0
	numCats := 0
	for _, n := range t.nodes {
		for _, st := range n.stages {
			steps := st.steps
			if len(steps) == 0 {
				continue
			}
			if v := steps[len(steps)-1]; v > maxSteps {
				maxSteps = v
			}
			numCats = len(st.steps)
		}
	}
	pathChan := make(chan pathChanType, cpu*2)
	for range cpu {
		go runSimPath(pathChan, t.walkers, numCats, maxSteps)
	}

	root := t.nodes[t.t.Root()]
	rs := root.stages[0]
	density := make([][]float64, len(rs.logLike))
	for i := range density {
		density[i] = make([]float64, len(rs.logLike[i]))
	}
	src := make([]int, particles)
	ts := make([]int, particles)
	root.upPass(t, pathChan, density, src, ts, cpu)

	close(pathChan)
}
