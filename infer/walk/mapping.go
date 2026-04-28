// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walk

import (
	"math/rand/v2"
	"sync"
)

// PathChan is used to communicate the paths
type PathChan struct {
	// Tree name
	Tree string

	// Node ID
	Node int

	// Age of the time stage
	Age int64

	// Particles
	Particles []Path
}

func (n *node) fullMap(t *Tree, pc chan PathChan) {
	n.mapUppass(t, pc)
	var wg sync.WaitGroup
	for _, c := range t.t.Children(n.id) {
		wg.Add(1)
		go func(c int) {
			nc := t.nodes[c]
			nc.fullMap(t, pc)
			wg.Done()
		}(c)
	}
	wg.Wait()
}

func (n *node) mapUppass(t *Tree, pc chan PathChan) {
	tmpEnd := make([][]float64, len(t.landProb))
	for i := range tmpEnd {
		tmpEnd[i] = make([]float64, t.tp.Pixelation().Len())
	}

	if t.t.IsRoot(n.id) {
		// At the root the marginal is the scaled likelihood
		// (we already have add the pixel priors in the down-pass)
		rs := n.stages[0]
		scaleLogProb(tmpEnd, rs.logLike)

		locs := make([]Path, t.particles)

		// In the root there is a single particle
		steps := 1
		for i := range locs {
			locs[i].locs = make([]pointLocation, steps)

			// pick the pixel
			for {
				trait := rand.IntN(len(t.landProb))
				px := rand.IntN(t.tp.Pixelation().Len())
				if rand.Float64() < tmpEnd[trait][px] {
					locs[i].locs[0] = pointLocation{
						pixel: px,
						trait: trait,
					}
					break
				}
			}
		}
		rs.locs = locs

		pc <- PathChan{
			Tree:      t.Name(),
			Node:      rs.node.id,
			Age:       rs.age,
			Particles: locs,
		}
	}

	// internodes
	// the first stage was already updated
	for i := 1; i < len(n.stages); i++ {
		ts := n.stages[i]
		prev := n.stages[i-1]

		// initialize the path
		last := len(prev.locs[0].locs) - 1
		paths := make([]Path, t.particles)
		steps := ts.steps + 1 // we add one for the first step
		for j := range paths {
			paths[j].locs = make([]pointLocation, steps)
			paths[j].locs[0] = prev.locs[j].locs[last]
		}

		// rotate if there is an state change
		rotAge := t.rot.ClosestStageAge(ts.age)
		prevAge := t.rot.ClosestStageAge(prev.age)
		if prevAge != rotAge {
			age := t.tp.ClosestStageAge(ts.age)
			rot := t.rot.OldToYoung(prevAge)
			for j := range paths {
				state := paths[j].locs[0].trait
				prior := t.landProb[state].StageProb(age)
				paths[j].locs[0].pixel = rotPixel(rot.Rot, paths[j].locs[0].pixel, prior.Prior)
			}
		}

		scaleLogProb(tmpEnd, ts.logLike)
		paths = ts.simMap(t, tmpEnd, paths)
		last = len(paths[0].locs) - 1
		locs := make([]Path, t.particles)
		for j := range locs {
			locs[j].locs = make([]pointLocation, 2)
			locs[j].locs[0] = paths[j].locs[0]
			locs[j].locs[1] = paths[j].locs[last]
		}
		ts.locs = locs

		pc <- PathChan{
			Tree:      t.Name(),
			Node:      ts.node.id,
			Age:       ts.age,
			Particles: paths,
		}
	}

	if !t.t.IsTerm(n.id) {
		split := n.stages[len(n.stages)-1]
		last := len(split.locs[0].locs) - 1

		// In a split node
		// copy the particles in the descendants.
		for _, d := range t.t.Children(n.id) {
			c := t.nodes[d]
			cs := c.stages[0]

			locs := make([]Path, t.particles)

			// In the split there is a single particle
			steps := 1
			for j := range locs {
				locs[j].locs = make([]pointLocation, steps)
				locs[j].locs[0] = split.locs[j].locs[last]
			}
			cs.locs = locs

			pc <- PathChan{
				Tree:      t.Name(),
				Node:      cs.node.id,
				Age:       cs.age,
				Particles: locs,
			}
		}
	}
}

func (ts *timeStage) simMap(t *Tree, end [][]float64, paths []Path) []Path {
	age := t.tp.ClosestStageAge(ts.age)

	answer := make(chan pathChanAnswer)
	go func() {
		pathChan <- pathChanType{
			cond:      end,
			particles: paths,
			w:         t.landProb,
			age:       age,
			steps:     ts.steps,
			answer:    answer,
		}
	}()

	<-answer
	close(answer)
	return paths
}
