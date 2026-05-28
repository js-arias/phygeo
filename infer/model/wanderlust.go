// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package model

import (
	"slices"
	"strings"
)

// Wanderlust indicates the base movement fraction
// for a given trait.
type Wanderlust struct {
	wander map[string]float64
}

// Traits returns the trait states
// with defined wanderlust values.
func (r *Wanderlust) Landscape() []string {
	traits := make([]string, 0, len(r.wander))
	for s := range r.wander {
		traits = append(traits, s)
	}
	slices.Sort(traits)
	return traits
}

// Wander returns the wanderlust parameter
// for a given trait state.
func (r *Wanderlust) Wander(trait string) float64 {
	trait = strings.Join(strings.Fields(strings.ToLower(trait)), " ")
	if trait == "" {
		return 0
	}
	wander, ok := r.wander[trait]
	if !ok {
		return 0
	}
	return wander
}
