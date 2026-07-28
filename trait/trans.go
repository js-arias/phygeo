// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package trait

import (
	"fmt"
	"slices"
	"strings"

	"github.com/js-arias/earth/pixkey"
)

// Trans is a definition of a numerical value
// of a given trait->trait transition
// under a given trait feature.
type Trans struct {
	states map[string]int
	key    *pixkey.PixKey
	labels map[string]int
	m      map[int][][]float64
}

// NewTrans creates a new transition matrix
// for trait transformations from a trait dataset
// and landscape keys.
func NewTrans(traits *Data, keys *pixkey.PixKey) *Trans {
	k := keys.Keys()

	labels := make(map[string]int)
	for i, id := range k {
		labels[keys.Label(id)] = i
	}

	t := traits.States()
	states := make(map[string]int, len(t))
	for i, s := range t {
		states[s] = i
	}

	m := make(map[int][][]float64, len(labels))
	for _, id := range labels {
		m[id] = make([][]float64, len(t))
		for j := range t {
			m[id][j] = make([]float64, len(t))
		}
	}

	return &Trans{
		states: states,
		key:    keys,
		labels: labels,
		m:      m,
	}
}

// Add adds a transition probability between two traits
// for a given landscape feature.
// The probability should be between 0 and 1.
func (t *Trans) Add(from, to, key string, prob float64) error {
	key = strings.Join(strings.Fields(strings.ToLower(key)), " ")
	if key == "" {
		return nil
	}
	k, ok := t.labels[key]
	if !ok {
		return nil
	}
	m, ok := t.m[k]
	if !ok {
		return nil
	}

	from = strings.Join(strings.Fields(strings.ToLower(from)), " ")
	if from == "" {
		return nil
	}
	f, ok := t.states[from]
	if !ok {
		return nil
	}
	to = strings.Join(strings.Fields(strings.ToLower(to)), " ")
	if to == "" {
		return nil
	}
	tt, ok := t.states[to]
	if !ok {
		return nil
	}

	if prob < 0 || prob > 1 {
		return fmt.Errorf("trait: invalid probability value: %.6f", prob)
	}
	var sum float64
	for i, p := range m[f] {
		if i == f {
			continue
		}
		sum += p
	}
	if prob+sum > 1.0 {
		return fmt.Errorf("trait: transition probabilities should sum up to 1.0")
	}

	t.m[k][f][tt] = prob
	return nil
}

// Landscape returns the landscape feature labels
// defined in a transition matrix.
func (t *Trans) Landscape() []string {
	landscape := make([]string, 0, len(t.labels))
	for l := range t.labels {
		landscape = append(landscape, l)
	}
	slices.Sort(landscape)
	return landscape
}

// Traits return the name of the states
// defined in a transition matrix.
func (t *Trans) Traits() []string {
	traits := make([]string, 0, len(t.states))
	for s := range t.states {
		traits = append(traits, s)
	}
	slices.Sort(traits)
	return traits
}

// Transition returns the transitions from a trait
// for a given landscape feature.
func (t *Trans) Transition(trait, key string) []float64 {
	key = strings.Join(strings.Fields(strings.ToLower(key)), " ")
	if key == "" {
		return nil
	}
	k, ok := t.labels[key]
	if !ok {
		return nil
	}
	m, ok := t.m[k]
	if !ok {
		return nil
	}

	trait = strings.Join(strings.Fields(strings.ToLower(trait)), " ")
	if trait == "" {
		return nil
	}
	f, ok := t.states[trait]
	if !ok {
		return nil
	}
	mr := m[f]

	tt := make([]float64, len(t.states))
	var sum float64
	for i := range tt {
		if i == f {
			continue
		}
		tt[i] = mr[i]
		sum += mr[i]
	}
	tt[f] = 1 - sum

	return tt
}
