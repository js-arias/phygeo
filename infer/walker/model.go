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
}

// New creates a new landscape model
// using the default PhyGeo model.
func New(landscape *model.TimePix, net earth.Network, movement, settlement *trait.Matrix, trans *trait.Trans, wanderlust float64, state string, stateID int, keys *pixkey.PixKey) Model {
	return &walkModel{
		stages:     make(map[int64]StageProb),
		tp:         landscape,
		net:        net,
		movement:   movement,
		settlement: settlement,
		trans:      trans,
		wanderlust: wanderlust,
		state:      state,
		id:         stateID,
		key:        keys,
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

	stageProb := w.stageProb(age)
	w.stages[age] = stageProb
	return stageProb
}

func (w *walkModel) stageProb(age int64) StageProb {
	landscape := w.tp.Stage(age)
	moveProb := w.wanderlust

	prior := make([]float64, w.tp.Pixelation().Len())
	logPrior := make([]float64, w.tp.Pixelation().Len())
	settlement := make([]float64, w.tp.Pixelation().Len())
	var sumPrior float64

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
		p := w.settlement.Weight(w.state, w.key.Label(s))
		settProb *= p
		tr := w.trans.Transition(w.state, w.key.Label(s))
		for i := range tr {
			tr[i] *= settProb
		}
		trans[px] = tr

		settlement[px] = p
		sumPrior += p
	}

	for px := range prior {
		prior[px] = settlement[px] / sumPrior
		logPrior[px] = math.Log(prior[px])
	}

	return StageProb{
		Move:       pp,
		Trans:      trans,
		Prior:      prior,
		LogPrior:   logPrior,
		Settlement: settlement,
	}
}
