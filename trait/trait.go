// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package trait provides a list of a trait observations
// for a taxon list.
package trait

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Data is a collection of trait states
// observed in a set of taxa.
type Data struct {
	taxon map[string]map[string]bool
}

// New creates a new empty data set.
func New() *Data {
	return &Data{
		taxon: make(map[string]map[string]bool),
	}
}

// Add adds a new observation
// (i.e., a trait state)
// for a given taxon.
func (d *Data) Add(taxon, state string) {
	taxon = canon(taxon)
	if taxon == "" {
		return
	}
	state = strings.Join(strings.Fields(strings.ToLower(state)), " ")
	if state == "" {
		return
	}

	obs, ok := d.taxon[taxon]
	if !ok {
		obs = make(map[string]bool)
		d.taxon[taxon] = obs
	}
	obs[state] = true
}

// HasTrait returns true if the given trait state
// is among the defined trait states
// in the dataset.
func (d *Data) HasTrait(state string) bool {
	state = strings.Join(strings.Fields(strings.ToLower(state)), " ")
	for _, obs := range d.taxon {
		if obs[state] {
			return true
		}
	}
	return false
}

// Obs returns the observed states
// for a taxon in a data set.
func (d *Data) Obs(taxon string) []string {
	taxon = canon(taxon)
	if taxon == "" {
		return nil
	}
	tx, ok := d.taxon[taxon]
	if !ok {
		return nil
	}
	obs := make([]string, 0, len(tx))
	for s := range tx {
		obs = append(obs, s)
	}
	slices.Sort(obs)
	return obs
}

// States returns the defined states
// in a data set.
func (d *Data) States() []string {
	st := make(map[string]bool)
	for _, obs := range d.taxon {
		for s := range obs {
			st[s] = true
		}
	}

	states := make([]string, 0, len(st))
	for s := range st {
		states = append(states, s)
	}
	slices.Sort(states)
	return states
}

// Taxa returns the taxa with observed states
// in a data set.
func (d *Data) Taxa() []string {
	taxa := make([]string, 0, len(d.taxon))
	for tx := range d.taxon {
		taxa = append(taxa, tx)
	}
	slices.Sort(taxa)
	return taxa
}

// Canon returns a taxon name
// in its canonical form.
func canon(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return ""
	}
	name = strings.ToLower(name)
	r, n := utf8.DecodeRuneInString(name)
	return string(unicode.ToUpper(r)) + name[n:]
}
