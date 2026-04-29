// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package model

import (
	"fmt"
	"slices"

	"github.com/js-arias/earth"
	"github.com/js-arias/earth/pixkey"
	"github.com/js-arias/phygeo/trait"
)

// Default creates a new model with the default values.
func Default(pix *earth.Pixelation, t *trait.Data, keys *pixkey.PixKey) *Model {
	if t == nil {
		// traits should be defined for a model
		panic("undefined traits")
	}

	mp := New()

	// Default roaming value for each trait is 0.05
	// (roughly equivalent to lambda 100).
	// It is by default a parameter equal for all traits.
	for _, n := range t.States() {
		pn := n + ":roaming"
		mp.Add(pn, Walk, 1, 0.05)
		mp.SetMax(pn, Walk, 1)
	}
	// Default number of steps is the number of pixels in the equator
	mp.Add("steps", Walk, 0, float64(pix.Equator()))

	// trait-landscape combinations
	tl := make(map[string]bool)
	for _, n := range t.States() {
		for _, k := range keys.Keys() {
			kn := keys.Label(k)
			pn := n + ":" + kn
			tl[pn] = true
		}
	}
	names := make([]string, 0, len(tl))
	for n := range tl {
		names = append(names, n)
	}
	slices.Sort(names)

	// By default all movement weights are set equal
	// (with 1.0)
	// but as different parameters,
	kv := 10
	for i, n := range names {
		mp.Add(n, Mov, kv+i, 1)
		mp.SetMax(n, Mov, 1)
	}

	// By default all settlement weights are set equal
	// (with 1.0)
	// and fixed.
	for _, n := range names {
		mp.Add(n, Sett, 0, 1)
	}

	return mp
}

// Validate checks if the different parameters
// are defined in the underlying data.
// It stop at the first error
func (mp *Model) Validate(t *trait.Data, keys *pixkey.PixKey) error {
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
		if p.id == 0 {
			continue
		}
		if !tl[p.name] {
			return fmt.Errorf("parameter %q [ID:%d] not found in traits and landscapes", p.name, p.id)
		}
	}

	return nil
}
