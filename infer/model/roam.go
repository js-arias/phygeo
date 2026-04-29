// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package model

import (
	"slices"
	"strings"
)

// Roaming indicates the base movement fraction
// for a given trait.
type Roaming struct {
	roam map[string]float64
}

// Traits returns the trait states
// with defined roaming values.
func (r *Roaming) Landscape() []string {
	traits := make([]string, 0, len(r.roam))
	for s := range r.roam {
		traits = append(traits, s)
	}
	slices.Sort(traits)
	return traits
}

// Roam returns the roaming parameter
// for a given trait state.
func (r *Roaming) Roam(trait string) float64 {
	trait = strings.Join(strings.Fields(strings.ToLower(trait)), " ")
	if trait == "" {
		return 0
	}
	roam, ok := r.roam[trait]
	if !ok {
		return 0
	}
	return roam
}
