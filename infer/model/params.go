// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package model

import (
	"strings"

	"github.com/js-arias/earth/pixkey"
	"github.com/js-arias/phygeo/cats"
	"github.com/js-arias/phygeo/trait"
	"gonum.org/v1/gonum/stat/distuv"
)

// Lambda returns the lambda parameter for the random walk.
func (mp *Model) Lambda() float64 {
	return mp.Val("lambda", Walk)
}

// Steps returns the steps per million year for the random walk.
func (mp *Model) Steps() int {
	steps := int(mp.Val("steps", Walk))
	if steps == 0 {
		// default
		return 360
	}
	return steps
}

// Relaxed returns the relaxed function for the random walk.
func (mp *Model) Relaxed() cats.Discrete {
	numCats := int(mp.Val("cats", Rate))
	if numCats == 0 {
		numCats = 9 // default number of categories
	}

	fn := "lognormal"
	val := 1.0
	for _, p := range mp.vars {
		if p.tp != Rate {
			continue
		}
		if p.name == "gamma" {
			fn = "gamma"
			val = p.val
			break
		}
		if p.name == "lognormal" {
			val = p.val
			break
		}
	}

	if fn == "gamma" {
		return cats.Gamma{
			Param: distuv.Gamma{
				Alpha: val,
				Beta:  val,
			},
			NumCat: numCats,
		}
	}

	// default is logNormal
	return cats.LogNormal{
		Param: distuv.LogNormal{
			Mu:    0,
			Sigma: val,
		},
		NumCat: numCats,
	}
}

// Movement returns the movement matrix
func (mp *Model) Movement(t *trait.Data, keys *pixkey.PixKey) *trait.Matrix {
	return mp.movSettMat(t, keys, Mov)
}

// Settlement returns the settlement matrix
func (mp *Model) Settlement(t *trait.Data, keys *pixkey.PixKey) *trait.Matrix {
	return mp.movSettMat(t, keys, Sett)
}

// IsScaled returns true if at least one of the fixed values
// for movement is set to 1.0
func (mp *Model) IsScaled() bool {
	for _, p := range mp.vars {
		if p.tp != Mov {
			continue
		}
		if p.id != 0 {
			continue
		}
		if p.val == 1 {
			return true
		}
	}
	return false
}

func (mp *Model) movSettMat(t *trait.Data, keys *pixkey.PixKey, tp Type) *trait.Matrix {
	m := trait.NewMatrix(t, keys)
	for _, p := range mp.vars {
		if p.tp != tp {
			continue
		}
		s := strings.Split(p.name, ":")
		if len(s) != 2 {
			continue
		}
		m.Add(s[0], s[1], p.val)
	}
	return m
}
