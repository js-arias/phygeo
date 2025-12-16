// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walk

import (
	"math/rand/v2"
	"runtime"
	"sync"

	"github.com/js-arias/earth"
)

// A pointLocation is the pixel location
// and trait state
// of a particle in a Path
type pointLocation struct {
	pixel int
	trait int
}

// A Path is a sequence of locations
// in a lineage.
type Path struct {
	locs   []pointLocation
	cat    int
	traits []string
}

// Cat returns the category of the relaxed random walk
// for the particle.
func (p Path) Cat() int {
	return p.cat
}

// Len returns the number of steps
// in a particle path.
func (p Path) Len() int {
	return len(p.locs)
}

// Pos return the location and trait
// of particle in a trait
// at a given time.
func (p Path) Pos(step int) (pixel int, trait string) {
	pL := p.locs[step]
	return pL.pixel, p.traits[pL.trait]
}

type pathChanType struct {
	cond      [][]float64
	particles []Path

	w     *walkModel
	age   int64
	cat   int
	steps int

	answer chan pathChanAnswer
}

var pathChanMutex sync.Mutex
var openPathChan bool
var pathChan chan pathChanType

type pathChanAnswer struct {
	particles []Path
	cat       int
}

// StartMap prepares the package for stochastic map simulations.
// Use cpu to define the number of process
// used for the reconstruction.
// The default (zero) uses all available CPU.
// After all optimization is done,
// use EndMap to close the goroutines.
func StartMap(cpu int, pix *earth.Pixelation, traits, particles int) {
	pathChanMutex.Lock()
	defer pathChanMutex.Unlock()

	if openPathChan {
		return
	}

	if cpu == 0 {
		cpu = runtime.NumCPU()
	}

	pathChan = make(chan pathChanType, cpu*2)
	for range cpu {
		go mapSim(pathChan, pix.Len(), traits, particles)
	}
	openPathChan = true
}

// EndMap closes the goroutines used for the stochastic maps.
func EndMap() {
	pathChanMutex.Lock()
	defer pathChanMutex.Unlock()
	close(pathChan)
	openPathChan = false
}

func mapSim(c chan pathChanType, sz, traits, particles int) {
	prev := make([][]float64, traits)
	curr := make([][]float64, traits)
	for i := range prev {
		prev[i] = make([]float64, sz)
		curr[i] = make([]float64, sz)
	}
	ids := make([]int, particles)

	for cc := range c {
		ids = ids[:0]
		for i := range cc.particles {
			if cc.particles[i].cat == cc.cat {
				ids = append(ids, i)
			}
		}
		if len(ids) == 0 {
			// skip un-sampled categories
			cc.answer <- pathChanAnswer{
				particles: cc.particles,
				cat:       cc.cat,
			}
			continue
		}
		for step := range cc.steps {
			for i := range curr {
				copy(curr[i], cc.cond[i])
			}
			// we have already done the last step,
			// so we remove one step
			stepCond := catConditional(cc.w, prev, curr, cc.age, cc.steps-step-1)
			for _, id := range ids {
				loc := cc.particles[id].locs[step]
				stage := cc.w.stage(cc.age, loc.trait)
				move := stage.move[loc.pixel]
				var sum float64
				for _, nx := range move {
					sum += nx.prob * stepCond[loc.trait][nx.id]
				}
				for {
					nxPix := rand.IntN(len(move))
					nx := move[nxPix]
					p := nx.prob * stepCond[loc.trait][nx.id] / sum
					if rand.Float64() < p {
						cc.particles[id].locs[step+1] = pointLocation{
							pixel: nx.id,
							trait: loc.trait,
						}
						break
					}
				}
			}
		}
		cc.answer <- pathChanAnswer{
			particles: cc.particles,
			cat:       cc.cat,
		}
	}
}
