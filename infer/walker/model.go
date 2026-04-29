// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walker

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
	stages map[int64]StageProb

	tp  *model.TimePix
	net earth.Network

	movement   *trait.Matrix
	settlement *trait.Matrix

	roaming float64

	state string
	id    int //state ID
	key   *pixkey.PixKey

	buildPixProb func(w *walkModel, age int64) [][]PixProb
}

// New creates a new landscape model
// using the default PhyGeo model.
func New(landscape *model.TimePix, net earth.Network, movement, settlement *trait.Matrix, roaming float64, state string, stateID int, keys *pixkey.PixKey) Model {
	return &walkModel{
		stages:       make(map[int64]StageProb),
		tp:           landscape,
		net:          net,
		movement:     movement,
		settlement:   settlement,
		roaming:      roaming,
		state:        state,
		id:           stateID,
		key:          keys,
		buildPixProb: defPixProb,
	}
}

// Bouckaert creates a new landscape model
// using a generalized definition of the model from
// Bouckaert et al. (2012) Science 337:957-960.
func Bouckaert(landscape *model.TimePix, net earth.Network, movement, settlement *trait.Matrix, roaming float64, state string, stateID int, keys *pixkey.PixKey) Model {
	return &walkModel{
		stages:       make(map[int64]StageProb),
		tp:           landscape,
		net:          net,
		movement:     movement,
		settlement:   settlement,
		roaming:      roaming,
		state:        state,
		id:           stateID,
		key:          keys,
		buildPixProb: buildBouckaert,
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

	prob := w.buildPixProb(w, age)
	prior, sett := w.buildPrior(age)
	stageProb := StageProb{
		Move:       prob,
		Prior:      prior,
		Settlement: sett,
	}
	w.stages[age] = stageProb
	return stageProb
}

func defPixProb(w *walkModel, age int64) [][]PixProb {
	landscape := w.tp.Stage(age)
	moveProb := w.roaming

	pp := make([][]PixProb, w.tp.Pixelation().Len())
	for px := range pp {
		n := w.net[px]
		prob := make([]PixProb, len(n))
		settProb := 1.0
		mv := moveProb / float64(len(n)-1)
		sp := 0
		for i, x := range n {
			if x == px {
				sp = i
			}
			v := landscape[x]
			p := mv * w.movement.Weight(w.state, w.key.Label(v))
			prob[i] = PixProb{
				ID:   x,
				Prob: p,
			}
			settProb -= p
		}
		s := landscape[px]
		settProb *= w.settlement.Weight(w.state, w.key.Label(s))
		prob[sp] = PixProb{
			ID:   px,
			Prob: settProb,
		}
		pp[px] = prob
	}
	return pp
}

// Build the Bouckaert et al. (2012) model.
func buildBouckaert(w *walkModel, age int64) [][]PixProb {
	landscape := w.tp.Stage(age)
	moveProb := w.roaming
	pp := make([][]PixProb, w.tp.Pixelation().Len())
	for px := range pp {
		n := w.net[px]
		prob := make([]PixProb, len(n))
		mv := moveProb / float64(len(n)-1)
		s := landscape[px]

		// If we cannot settle we have to move all the probability out
		if w.settlement.Weight(w.state, w.key.Label(s)) == 0 {
			mv = 1.0 / float64(len(n)-1)
			for i, x := range n {
				p := mv
				if x == px {
					p = 0
				}
				prob[i] = PixProb{
					ID:   x,
					Prob: p,
				}
			}
			continue
		}

		var sumMov float64
		for i, x := range n {
			if x == px {
				continue
			}
			v := landscape[x]
			p := mv * w.movement.Weight(w.state, w.key.Label(v))
			prob[i] = PixProb{
				ID:   x,
				Prob: p,
			}
			sumMov += p
		}
		for i, x := range n {
			if x != px {
				continue
			}
			prob[i] = PixProb{
				ID:   x,
				Prob: 1 - sumMov,
			}
		}
		pp[px] = prob
	}
	return pp
}

func (w *walkModel) buildPrior(age int64) (prior, settlement []float64) {
	landscape := w.tp.Stage(age)

	prior = make([]float64, w.tp.Pixelation().Len())
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
	}
	return prior, settlement
}
