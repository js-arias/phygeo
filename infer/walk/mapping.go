// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walk

import (
	"math/rand/v2"
	"sync"
)

func (n *node) fullMap(t *Tree) {
	n.mapUppass(t)
	var wg sync.WaitGroup
	for _, c := range t.t.Children(n.id) {
		wg.Add(1)
		go func(c int) {
			nc := t.nodes[c]
			nc.fullMap(t)
			wg.Done()
		}(c)
	}
	wg.Wait()
}

func (n *node) mapUppass(t *Tree) {
	tmpEnd := make([][][]float64, len(t.landProb))
	for i := range tmpEnd {
		tmpEnd[i] = make([][]float64, len(t.landProb[i].traits))
		for j := range tmpEnd[i] {
			tmpEnd[i][j] = make([]float64, t.tp.Pixelation().Len())
		}
	}

	if t.t.IsRoot(n.id) {
		// At the root the marginal is the scaled likelihood
		// (we already have add the pixel priors in the down-pass)
		rs := n.stages[0]
		scaleLogProb(tmpEnd, rs.logLike)

		paths := make([]Path, t.particles)

		// In the root there is a single particle
		steps := 1
		for i := range paths {
			paths[i].locs = make([]pointLocation, steps)

			// pick the pixel
			for {
				cat := rand.IntN(len(t.landProb))
				trait := rand.IntN(len(t.landProb[cat].traits))
				px := rand.IntN(t.tp.Pixelation().Len())
				if rand.Float64() < tmpEnd[cat][trait][px] {
					paths[i].cat = cat
					paths[i].traits = t.landProb[cat].traits
					paths[i].locs[0] = pointLocation{
						pixel: px,
						trait: trait,
					}
					break
				}
			}
		}
		rs.paths = paths
	}

	// internodes
	// the first stage was already updated
	for i := 1; i < len(n.stages); i++ {
		ts := n.stages[i]
		age := t.rot.ClosestStageAge(ts.age)

		for j := range t.landProb {
			t.landProb[j].prepareStage(age)
		}

		prev := n.stages[i-1]
		last := len(prev.paths[0].locs) - 1

		// initialize the path
		paths := make([]Path, t.particles)
		steps := ts.steps + 1 // we add one for the first step
		for j := range paths {
			paths[j].locs = make([]pointLocation, steps)
			paths[j].locs[0] = prev.paths[j].locs[last]
			paths[j].cat = prev.paths[j].cat
			paths[j].traits = t.landProb[paths[j].cat].traits
		}

		prevAge := t.rot.ClosestStageAge(prev.age)
		if prevAge != age {
			// rotate if there is an state change
			rot := t.rot.OldToYoung(prevAge)
			for j := range paths {
				prior := t.landProb[paths[j].cat].stage(age, paths[j].locs[0].trait)
				paths[j].locs[0].pixel = rotPixel(rot.Rot, paths[j].locs[0].pixel, prior.prior)
			}
		}

		scaleLogProb(tmpEnd, ts.logLike)
		ts.simMap(t, tmpEnd, paths)
	}

	if !t.t.IsTerm(n.id) {
		split := n.stages[len(n.stages)-1]
		last := len(split.paths[0].locs) - 1

		// In a split node
		// copy the particles in the descendants.
		// Now copy the marginals in each descendant.
		for _, d := range t.t.Children(n.id) {
			c := t.nodes[d]
			cs := c.stages[0]
			scaleLogProb(tmpEnd, cs.logLike)

			paths := make([]Path, t.particles)

			// In the split there is a single particle
			steps := 1
			for j := range paths {
				paths[j].locs = make([]pointLocation, steps)
				paths[j].locs[0] = split.paths[j].locs[last]

				// to pick the category
				// we weight the conditional of the pixel
				// for each category
				var sum float64
				trait := paths[j].locs[0].trait
				px := paths[j].locs[0].pixel
				for cat := range tmpEnd {
					sum += tmpEnd[cat][trait][px]
				}
				for {
					cat := rand.IntN(len(tmpEnd))
					p := tmpEnd[cat][trait][px] / sum
					if rand.Float64() < p {
						paths[j].cat = cat
						break
					}
				}
				paths[j].traits = t.landProb[paths[j].cat].traits
			}
			cs.paths = paths
		}
	}
}

func (ts *timeStage) simMap(t *Tree, end [][][]float64, paths []Path) {
	age := t.tp.ClosestStageAge(ts.age)

	answer := make(chan pathChanAnswer)
	go func() {
		for i := range t.landProb {
			pathChan <- pathChanType{
				cond:      end[i],
				particles: paths,
				w:         t.landProb[i],
				age:       age,
				cat:       i,
				steps:     ts.steps,
				answer:    answer,
			}
		}
	}()

	for range t.landProb {
		<-answer
	}
	close(answer)

	ts.paths = paths
}
