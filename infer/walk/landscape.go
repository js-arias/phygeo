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
		prior := w.buildPrior(age, t)
		prob := w.buildPixProb(age, t)
		trStage[i] = stageProb{
			prior: prior,
			move:  prob,
		}
	}
	w.stages[age] = trStage
}

type stageProb struct {
	prior []float64
	move  [][]pixProb
}

func (w *walkModel) buildPrior(age int64, t string) []float64 {
	landscape := w.tp.Stage(age)

	prior := make([]float64, w.tp.Pixelation().Len())
	for px := range prior {
		s := landscape[px]
		prior[px] = w.settlement.Weight(t, w.key.Label(s))
	}
	return prior
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
		var max float64
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
			if p > max {
				max = p
			}
		}

		// if all destinations are prohibited
		// do not move
		if max == 0 {
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
			prob[i].prob = p.prob / max
		}
		pp[px] = prob
	}
	return pp
}
