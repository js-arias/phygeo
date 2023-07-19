// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package diffusion

import (
	"math"
	"sync"
	"time"

	"github.com/js-arias/earth"
	"golang.org/x/exp/rand"
)

func init() {
	rand.Seed(uint64(time.Now().UnixNano()))
}

type simChan struct {
	particle int
	answer   chan struct{}
}

func doSim(pc chan simChan, t *Tree, size int) {
	density := make([]likePix, 0, size)
	for c := range pc {
		root := t.nodes[t.t.Root()]
		source := t.simulateRoot(c.particle, density)
		root.simulate(t, c.particle, source, density)
		c.answer <- struct{}{}
	}
}

// SrcDest contains the pixels of a particle simulation.
type SrcDest struct {
	// ID of the source pixel
	From int

	// ID of the destination pixel
	To int
}

// Simulate performs stochastic mappings
// for the given number of particles.
func (t *Tree) Simulate(particles int) {
	root := t.nodes[t.t.Root()]
	root.scaleLike(t, particles)

	sChan := make(chan simChan, numCPU*2)
	for i := 0; i < numCPU; i++ {
		go doSim(sChan, t, t.landscape.Pixelation().Len())
	}

	var wg sync.WaitGroup
	for p := 0; p < particles; p++ {
		wg.Add(1)
		go func(p int) {
			a := make(chan struct{})
			sChan <- simChan{
				particle: p,
				answer:   a,
			}
			<-a
			wg.Done()
		}(p)
	}
	wg.Wait()
}

func (n *node) scaleLike(t *Tree, p int) {
	for _, st := range n.stages {
		st.particles = make([]SrcDest, p)
		st.scaled = make(map[int]float64, len(st.logLike))

		tp := t.landscape.Stage(t.landscape.ClosestStageAge(st.age))
		rot := t.rot.OldToYoung(st.age)

		max := -math.MaxFloat64
		for px, p := range st.logLike {
			// skip pixels with 0 prior
			prior, ok := t.logPrior[tp[px]]
			if !ok {
				continue
			}

			if rot != nil {
				// skip pixels that are invalid in the next stage rotation
				if pxs := rot.Rot[px]; len(pxs) == 0 {
					continue
				}
			}

			p += prior
			st.scaled[px] = p
			if p > max {
				max = p
			}
		}

		// scale
		for px, p := range st.scaled {
			st.scaled[px] = math.Exp(p - max)
		}
	}

	for _, c := range t.t.Children(n.id) {
		nc := t.nodes[c]
		nc.scaleLike(t, p)
	}
}

// SimulateRoot get the first pixel at the root,
// and return it.
func (t *Tree) simulateRoot(p int, density []likePix) int {
	root := t.nodes[t.t.Root()]
	rs := root.stages[0]

	// set density
	var max float64
	density = density[:0]
	for px, p := range rs.scaled {
		density = append(density, likePix{
			px:   px,
			like: p,
		})
		if p > max {
			max = p
		}
	}

	dest := rs.pick(p, -1, max, density)
	return rotPix(t.rot, t.landscape, dest, rs.age, t.pp)
}

func (n *node) simulate(t *Tree, p, source int, density []likePix) {
	n.stages[0].particles[p] = SrcDest{
		From: source,
		To:   source,
	}

	for i := 1; i < len(n.stages); i++ {
		ts := n.stages[i]
		source = ts.simulate(t, p, source, density)
	}

	for _, cID := range t.t.Children(n.id) {
		c := t.nodes[cID]
		c.simulate(t, p, source, density)
	}
}

func (ts *timeStage) simulate(t *Tree, p, source int, density []likePix) int {
	var max float64

	// calculate density
	density = density[:0]
	// use distance matrix
	if t.dm != nil {
		for px, p := range ts.scaled {
			p *= ts.pdf.ProbRingDist(t.dm.At(source, px))
			density = append(density, likePix{
				px:   px,
				like: p,
			})
			if p > max {
				max = p
			}
		}
	} else {
		pix := t.landscape.Pixelation()
		pt1 := pix.ID(source).Point()
		for px, p := range ts.scaled {
			pt2 := pix.ID(px).Point()
			dist := earth.Distance(pt1, pt2)
			p *= ts.pdf.Prob(dist)
			density = append(density, likePix{
				px:   px,
				like: p,
			})
			if p > max {
				max = p
			}
		}
	}

	dest := ts.pick(p, source, max, density)
	return rotPix(t.rot, t.landscape, dest, ts.age, t.pp)
}

// Pick pixel picks a pixel from a destination density
// at the scale of the density,
// store it,
// and return the destination pixel.
func (ts *timeStage) pick(p, source int, scale float64, density []likePix) int {
	var dest int
	for {
		i := rand.Intn(len(density))
		accept := density[i].like / scale
		if rand.Float64() < accept {
			dest = density[i].px
			ts.particles[p] = SrcDest{
				From: source,
				To:   dest,
			}
			break
		}
	}
	return dest
}
