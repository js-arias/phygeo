// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walker

import (
	"math"
	"sync"

	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/pixkey"
	"github.com/js-arias/phygeo/trait"
)

// WalkModel contains the landscape model for the random walk
type walkModel struct {
	lock   sync.Mutex
	stages map[int64]StageProb

	tp  *model.TimePix
	net earth.Network

	movement   *trait.Matrix
	settlement *trait.Matrix
	trans      *trait.Trans

	wanderlust float64

	state string
	id    int //state ID
	key   *pixkey.PixKey

	buildPixProb func(w *walkModel, age int64) ([][]PixProb, [][]float64)
}

// New creates a new landscape model
// using the default PhyGeo model.
func New(landscape *model.TimePix, net earth.Network, movement, settlement *trait.Matrix, trans *trait.Trans, wanderlust float64, state string, stateID int, keys *pixkey.PixKey) Model {
	return &walkModel{
		stages:       make(map[int64]StageProb),
		tp:           landscape,
		net:          net,
		movement:     movement,
		settlement:   settlement,
		trans:        trans,
		wanderlust:   wanderlust,
		state:        state,
		id:           stateID,
		key:          keys,
		buildPixProb: defPixProb,
	}
}

func (w *walkModel) StageProb(age int64) StageProb {
	w.lock.Lock()
	defer w.lock.Unlock()
	if s, ok := w.stages[age]; ok {
		return s
	}
	s := w.prepare(age)
	return s
}

// State returns the trait state
// assigned to the model
func (w *walkModel) State() string {
	return w.state
}

func (w *walkModel) prepare(age int64) StageProb {
	age = w.tp.ClosestStageAge(age)
	if s, ok := w.stages[age]; ok {
		return s
	}

	prob, trans := w.buildPixProb(w, age)
	prior, logPrior, sett := w.buildPrior(age)
	stageProb := StageProb{
		Move:       prob,
		Trans:      trans,
		Prior:      prior,
		LogPrior:   logPrior,
		Settlement: sett,
	}
	w.stages[age] = stageProb
	return stageProb
}

func defPixProb(w *walkModel, age int64) ([][]PixProb, [][]float64) {
	landscape := w.tp.Stage(age)
	moveProb := w.wanderlust

	pp := make([][]PixProb, w.tp.Pixelation().Len())
	trans := make([][]float64, w.tp.Pixelation().Len())
	for px := range pp {
		n := w.net[px]
		prob := make([]PixProb, 0, len(n)-1)
		settProb := 1 - moveProb
		var moveWeight float64
		for _, x := range n {
			if x == px {
				continue
			}
			v := landscape[x]
			moveWeight += w.movement.Weight(w.state, w.key.Label(v))
		}
		for _, x := range n {
			if x == px {
				continue
			}
			v := landscape[x]
			p := moveProb * w.movement.Weight(w.state, w.key.Label(v)) / moveWeight
			prob = append(prob, PixProb{
				ID:   x,
				Prob: p,
			})
		}
		pp[px] = prob

		// settlement
		s := landscape[px]
		settProb *= w.settlement.Weight(w.state, w.key.Label(s))
		tr := w.trans.Transition(w.state, w.key.Label(s))
		for i := range tr {
			tr[i] *= settProb
		}
		trans[px] = tr
	}
	return pp, trans
}

func (w *walkModel) buildPrior(age int64) (prior, logPrior, settlement []float64) {
	landscape := w.tp.Stage(age)

	prior = make([]float64, w.tp.Pixelation().Len())
	logPrior = make([]float64, w.tp.Pixelation().Len())
	settlement = make([]float64, w.tp.Pixelation().Len())
	var sum float64
	for px := range prior {
		s := landscape[px]
		p := w.settlement.Weight(w.state, w.key.Label(s))
		prior[px] = p
		settlement[px] = p
		sum += p
	}
	for px, p := range prior {
		prior[px] = p / sum
		logPrior[px] = math.Log(prior[px])
	}
	return prior, logPrior, settlement
}
