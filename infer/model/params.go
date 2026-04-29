// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package model

import (
	"strings"

	"github.com/js-arias/earth/pixkey"
	"github.com/js-arias/phygeo/timestage"
	"github.com/js-arias/phygeo/trait"
)

// Roaming returns the roam parameter
// for a given trait state.
func (mp *Model) Roaming(state string) float64 {
	state = strings.ToLower(strings.Join(strings.Fields(state), " "))
	pn := state + ":roaming"
	return mp.Val(pn, Walk)
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

// StemAge returns the age of the stem root branch
// in years.
func (mp *Model) StemAge() int64 {
	age := mp.Val("stemage", Walk)
	if age == 0 {
		return 0
	}
	return int64(age * timestage.MillionYears)
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
