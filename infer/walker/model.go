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
	stages map[int64][]StageProb

	tp  *model.TimePix
	net earth.Network

	movement   *trait.Matrix
	settlement *trait.Matrix

	settProb float64

	traits []string
	key    *pixkey.PixKey

	buildPixProb func(w *walkModel, age int64, t string) [][]PixProb
}

// New creates a new landscape model
// using the default PhyGeo model
// (A model in which probability vanished
// when moving over unsuitable landscapes).
func New(landscape *model.TimePix, net earth.Network, movement, settlement *trait.Matrix, settProb float64, traits []string, keys *pixkey.PixKey) Model {
	return &walkModel{
		stages:       make(map[int64][]StageProb),
		tp:           landscape,
		net:          net,
		movement:     movement,
		settlement:   settlement,
		settProb:     settProb,
		traits:       traits,
		key:          keys,
		buildPixProb: defPixProb,
	}
}

// Bouckaert creates a new landscape model
// using a generalized definition of the model from
// Bouckaert et al. (2012) Science 337:957-960.
func Bouckaert(landscape *model.TimePix, net earth.Network, movement, settlement *trait.Matrix, settProb float64, traits []string, keys *pixkey.PixKey) Model {
	return &walkModel{
		stages:       make(map[int64][]StageProb),
		tp:           landscape,
		net:          net,
		movement:     movement,
		settlement:   settlement,
		settProb:     settProb,
		traits:       traits,
		key:          keys,
		buildPixProb: buildBouckaert,
	}
}

func (w *walkModel) StageProb(age int64, t int) StageProb {
	w.lock.Lock()
	defer w.lock.Unlock()
	if s, ok := w.stages[age]; ok {
		return s[t]
	}
	s := w.prepare(age)
	return s[t]
}

func (w *walkModel) Traits() []string {
	return w.traits
}

func (w *walkModel) prepare(age int64) []StageProb {
	age = w.tp.ClosestStageAge(age)
	if s, ok := w.stages[age]; ok {
		return s
	}

	trStage := make([]StageProb, len(w.traits))
	for i, t := range w.traits {
		prob := w.buildPixProb(w, age, t)
		prior := w.buildPrior(age, t)
		trStage[i] = StageProb{
			Move:  prob,
			Prior: prior,
		}
	}
	w.stages[age] = trStage
	return trStage
}

func defPixProb(w *walkModel, age int64, t string) [][]PixProb {
	landscape := w.tp.Stage(age)
	moveProb := 1 - w.settProb
	pp := make([][]PixProb, w.tp.Pixelation().Len())
	for px := range pp {
		n := w.net[px]
		prob := make([]PixProb, len(n))
		mv := moveProb / float64(len(n)-1)
		s := landscape[px]
		for i, x := range n {
			v := landscape[x]
			p := mv * w.movement.Weight(t, w.key.Label(v))
			if x == px {
				p = w.settlement.Weight(t, w.key.Label(s)) * w.settProb
			}
			prob[i] = PixProb{
				ID:   x,
				Prob: p,
			}
		}
		pp[px] = prob
	}
	return pp
}

// Build the Bouckaert et al. (2012) model.
func buildBouckaert(w *walkModel, age int64, t string) [][]PixProb {
	landscape := w.tp.Stage(age)

	pp := make([][]PixProb, w.tp.Pixelation().Len())
	for px := range pp {
		n := w.net[px]
		prob := make([]PixProb, len(n))
		var sumWeight float64
		for _, x := range n {
			if x == px {
				continue
			}
			v := landscape[x]
			sumWeight += w.movement.Weight(t, w.key.Label(v))
		}
		s := landscape[px]
		moveProb := 1 - w.settlement.Weight(t, w.key.Label(s))*w.settProb

		var sumProb float64
		for i, x := range n {
			v := landscape[x]
			p := moveProb * w.movement.Weight(t, w.key.Label(v)) / sumWeight
			if x == px {
				p = 1 - moveProb
			}
			prob[i] = PixProb{
				ID:   x,
				Prob: p,
			}
			sumProb += p
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
