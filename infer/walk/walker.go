// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walk

import (
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/pixkey"
	"github.com/js-arias/phygeo/trait"
)

// WalkModel contains the landscape model for the random walk
type walkModel struct {
	stages map[int64][]stageProb

	tp  *model.TimePix
	net earth.Network

	movement   *trait.Matrix
	settlement *trait.Matrix

	traits []string
	key    *pixkey.PixKey
}

func (w *walkModel) stage(age int64, t int) stageProb {
	return w.stages[age][t]
}

func (w *walkModel) prepareStage(age int64) {
	age = w.tp.ClosestStageAge(age)
	if _, ok := w.stages[age]; ok {
		return
	}

	trStage := make([]stageProb, len(w.traits))
	for i, t := range w.traits {
		prob := w.buildPixProb(age, t)
		trStage[i] = stageProb{
			move: prob,
		}
	}
	w.stages[age] = trStage
}

type stageProb struct {
	move [][]pixProb
}

type pixProb struct {
	id   int
	prob float64
}

func (w *walkModel) buildPixProb(age int64, t string) [][]pixProb {
	landscape := w.tp.Stage(age)

	pp := make([][]pixProb, w.tp.Pixelation().Len())
	for px := range pp {
		n := w.net[px]
		prob := make([]pixProb, len(n))
		s := landscape[px]
		var sum float64
		for i, x := range n {
			v := landscape[x]
			p := w.movement.Weight(t, w.key.Label(v))
			if x == px {
				p = w.settlement.Weight(t, w.key.Label(s))
			}
			prob[i] = pixProb{
				id:   x,
				prob: p,
			}
			sum += p
		}

		// if all destinations are prohibited
		// do not move
		if sum == 0 {
			for i, x := range n {
				if x == px {
					prob[i].prob = 1
					break
				}
			}
			pp[px] = prob
			continue
		}

		// normalize probabilities
		for i, p := range prob {
			prob[i].prob = p.prob / sum
		}
		pp[px] = prob
	}
	return pp
}
