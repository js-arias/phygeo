// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walk

import (
	"sync"

	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/pixkey"
	"github.com/js-arias/phygeo/trait"
)

// WalkModel contains the landscape model for the random walk
type walkModel struct {
	lock   sync.Mutex
	stages map[int64][]stageProb

	tp  *model.TimePix
	net earth.Network

	movement   *trait.Matrix
	settlement *trait.Matrix

	settProb float64

	traits []string
	key    *pixkey.PixKey
}

func (w *walkModel) stage(age int64, t int) stageProb {
	w.lock.Lock()
	defer w.lock.Unlock()
	return w.stages[age][t]
}

func (w *walkModel) prepareStage(age int64) {
	w.lock.Lock()
	defer w.lock.Unlock()

	age = w.tp.ClosestStageAge(age)
	if _, ok := w.stages[age]; ok {
		return
	}

	trStage := make([]stageProb, len(w.traits))
	for i, t := range w.traits {
		prob := w.buildPixProb(age, t)
		prior := w.buildPrior(age, t)
		trStage[i] = stageProb{
			move:  prob,
			prior: prior,
		}
	}
	w.stages[age] = trStage
}

type stageProb struct {
	move  [][]pixProb
	prior []float64
}

type pixProb struct {
	id   int
	prob float64
}

func (w *walkModel) buildPixProb(age int64, t string) [][]pixProb {
	landscape := w.tp.Stage(age)

	moveProb := 1 - w.settProb
	pp := make([][]pixProb, w.tp.Pixelation().Len())
	for px := range pp {
		n := w.net[px]
		prob := make([]pixProb, len(n))
		mv := moveProb / float64(len(n)-1)
		s := landscape[px]
		for i, x := range n {
			v := landscape[x]
			p := mv * w.movement.Weight(t, w.key.Label(v))
			if x == px {
				p = w.settlement.Weight(t, w.key.Label(s)) * w.settProb
			}
			prob[i] = pixProb{
				id:   x,
				prob: p,
			}
		}
		pp[px] = prob
	}
	return pp
}

func (w *walkModel) buildPrior(age int64, t string) []float64 {
	landscape := w.tp.Stage(age)

	prior := make([]float64, w.tp.Pixelation().Len())
	var sum float64
	for px := range prior {
		s := landscape[px]
		p := w.settlement.Weight(t, w.key.Label(s))
		prior[px] = p
		sum += p
	}
	for px, p := range prior {
		prior[px] = p / sum
	}
	return prior
}
