// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package model

import (
	"errors"
	"fmt"

	"github.com/js-arias/earth"
	"github.com/js-arias/earth/pixkey"
	"github.com/js-arias/phygeo/trait"
)

// Default creates a new model with the default values.
func Default(pix *earth.Pixelation, t *trait.Data, keys *pixkey.PixKey) *Model {
	mp := New()

	// Default value of lambda is 100.
	// Lambda is by default a parameter.
	mp.Add("lambda", Walk, 1, 100)

	// Default number of steps is the number of pixels in the equator
	mp.Add("steps", Walk, 0, float64(pix.Equator()))

	// Default relaxed function is a log-normal
	// with sigma 1.0
	// and 9 categories.
	// Sigma is by default a parameter.
	mp.Add("lognormal", Rate, 2, 1)
	mp.Add("cats", Rate, 0, 9)

	// By default all movement weights are set equal
	// (with 1.0)
	// as the same parameter.
	for _, n := range t.States() {
		for _, k := range keys.Keys() {
			kn := keys.Label(k)
			pn := n + ":" + kn
			mp.Add(pn, Mov, 3, 1)
			mp.SetMax(pn, Mov, 1)
		}
	}

	// By default all settlement weights are set equal
	// (with 1.0)
	// and fixed.
	for _, n := range t.States() {
		for _, k := range keys.Keys() {
			kn := keys.Label(k)
			pn := n + ":" + kn
			mp.Add(pn, Mov, 0, 1)
		}
	}

	return mp
}

// Validate checks if the different parameters
// are defined in the underlying data.
// It stop at the first error
func (mp *Model) Validate(t *trait.Data, keys *pixkey.PixKey) error {
	// check if there are more than one discrete rate function
	fn := make(map[string]bool)
	for _, p := range mp.vars {
		if p.tp != Rate {
			continue
		}
		if p.name == "cats" {
			continue
		}
		fn[p.name] = true
	}
	if len(fn) > 2 {
		return errors.New("too many rate function definitions")
	}

	// trait-landscape combinations
	tl := make(map[string]bool)
	for _, n := range t.States() {
		for _, k := range keys.Keys() {
			kn := keys.Label(k)
			pn := n + ":" + kn
			tl[pn] = true
		}
	}
	for _, p := range mp.vars {
		if p.tp != Mov && p.tp != Sett {
			continue
		}
		if !p.isParam {
			continue
		}
		if !tl[p.name] {
			return fmt.Errorf("parameter %q [ID:%d] not found in traits and landscapes", p.name, p.id)
		}
	}

	return nil
}
