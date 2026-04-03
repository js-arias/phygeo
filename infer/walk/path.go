// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walk

import (
	"encoding/gob"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/js-arias/earth"
	"github.com/js-arias/phygeo/infer/walker"
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
	locs []pointLocation
	cat  int
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
func (p Path) Pos(step int) (pixel, trait int) {
	pL := p.locs[step]
	return pL.pixel, pL.trait
}

type pathChanType struct {
	cond      [][]float64
	particles []Path

	w     walker.Model
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
	stepCond := make([][]float64, traits)
	for i := range prev {
		prev[i] = make([]float64, sz)
		curr[i] = make([]float64, sz)
		stepCond[i] = make([]float64, sz)
	}
	ids := make([]int, particles)

	// ensure temporal data will be deleted on a panic
	lastDir := ""
	defer func() {
		if lastDir != "" {
			os.RemoveAll(lastDir)
		}
	}()

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

		dir, err := os.MkdirTemp("", "tmp-up")
		if err != nil {
			panic(err)
		}
		lastDir = dir

		for i := range curr {
			copy(curr[i], cc.cond[i])
		}
		// we have already done the last step,
		// so we remove one step
		catConditionalWrite(dir, cc.w, prev, curr, cc.age, cc.steps-1)

		stages := make([]walker.StageProb, len(cc.w.Traits()))
		for i := range stages {
			stages[i] = cc.w.StageProb(cc.age, i)
		}

		for step := range cc.steps {
			f, err := os.Open(filepath.Join(dir, fmt.Sprintf("s%d", step)))
			if err != nil {
				panic(err)
			}
			d := gob.NewDecoder(f)
			if err := d.Decode(&stepCond); err != nil {
				panic(err)
			}
			f.Close()

			for _, id := range ids {
				loc := cc.particles[id].locs[step]
				stage := stages[loc.trait]
				move := stage.Move[loc.pixel]
				var sum float64
				for _, nx := range move {
					sum += nx.Prob * stepCond[loc.trait][nx.ID]
				}
				for {
					nxPix := rand.IntN(len(move))
					nx := move[nxPix]
					p := nx.Prob * stepCond[loc.trait][nx.ID] / sum
					if rand.Float64() < p {
						cc.particles[id].locs[step+1] = pointLocation{
							pixel: nx.ID,
							trait: loc.trait,
						}
						break
					}
				}
			}
		}
		os.RemoveAll(dir)
		lastDir = ""
		cc.answer <- pathChanAnswer{
			particles: cc.particles,
			cat:       cc.cat,
		}
	}
}

func catConditionalWrite(dir string, w walker.Model, prev, curr [][]float64, age int64, steps int) {
	// The most recent step
	f, err := os.Create(filepath.Join(dir, fmt.Sprintf("s%d", steps)))
	if err != nil {
		panic(err)
	}
	e := gob.NewEncoder(f)
	e.Encode(curr)
	f.Close()

	// do the conditionals
	stages := make([]walker.StageProb, len(curr))
	for i := range stages {
		stages[i] = w.StageProb(age, i)
	}
	for s := range steps {
		for i := range prev {
			prev[i], curr[i] = curr[i], prev[i]
		}
		for i := range curr {
			stage := stages[i]
			for px := range curr[i] {
				var sum float64
				for _, nx := range stage.Move[px] {
					sum += nx.Prob * prev[i][nx.ID]
				}
				curr[i][px] = sum
			}
		}

		// encode step
		f, err := os.Create(filepath.Join(dir, fmt.Sprintf("s%d", steps-s-1)))
		if err != nil {
			panic(err)
		}
		e := gob.NewEncoder(f)
		e.Encode(curr)
		f.Close()
	}
}
