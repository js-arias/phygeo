// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package trait

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/js-arias/earth/pixkey"
)

// Matrix is a definition of a numerical value
// of a given trait
// under a given trait feature
// (for example movement or settlement).
type Matrix struct {
	states map[string]int
	key    *pixkey.PixKey
	labels map[string]int
	m      [][]float64
}

// NewMatrix creates a new matrix for weights
// from a trait dataset
// and landscape keys.
func NewMatrix(traits *Data, keys *pixkey.PixKey) *Matrix {
	k := keys.Keys()

	t := traits.States()
	states := make(map[string]int, len(t))
	m := make([][]float64, len(t))
	for i, s := range t {
		states[s] = i
		m[i] = make([]float64, len(k))
	}

	labels := make(map[string]int)
	for i, id := range k {
		labels[keys.Label(id)] = i
	}

	return &Matrix{
		states: states,
		key:    keys,
		labels: labels,
		m:      m,
	}
}

// Add adds a weight to a given trait
// for a given landscape feature.
func (m *Matrix) Add(trait, key string, weight float64) {
	trait = strings.Join(strings.Fields(strings.ToLower(trait)), " ")
	if trait == "" {
		return
	}
	t, ok := m.states[trait]
	if !ok {
		return
	}

	key = strings.Join(strings.Fields(strings.ToLower(key)), " ")
	if key == "" {
		return
	}
	k, ok := m.labels[key]
	if !ok {
		return
	}

	m.m[t][k] = weight
}

// ReadTSV reads a matrix from a TSV file.
//
// The TSV file must contain the following fields:
//
//   - trait, the name of an observed trait state
//   - landscape, the name of the landscape feature
//   - weight, the weight associated with the trait-feature pair
//
// Here is an example file:
//
//	trait	landscape	weight
//	temperate	glacial	0.010000
//	temperate	ocean	0.010000
//	temperate	temperate	1.000000
//	temperate	tropical	0.100000
//	temperate	tundra	0.100000
//	tropical	glacial	0.010000
//	tropical	ocean	0.010000
//	tropical	temperate	0.100000
//	tropical	tropical	1.000000
//	tropical	tundra	0.010000
func (m *Matrix) ReadTSV(r io.Reader) error {
	tab := csv.NewReader(r)
	tab.Comma = '\t'
	tab.Comment = '#'

	head, err := tab.Read()
	if err != nil {
		return fmt.Errorf("while reading header: %v", err)
	}
	fields := make(map[string]int, len(head))
	for i, h := range head {
		h = strings.ToLower(h)
		fields[h] = i
	}
	for _, h := range []string{"trait", "landscape", "weight"} {
		if _, ok := fields[h]; !ok {
			return fmt.Errorf("expecting field %q", h)
		}
	}

	for {
		row, err := tab.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		ln, _ := tab.FieldPos(0)
		if err != nil {
			return fmt.Errorf("on row %d: %v", ln, err)
		}

		f := "trait"
		trait := row[fields[f]]

		f = "landscape"
		key := row[fields[f]]

		f = "weight"
		w, err := strconv.ParseFloat(row[fields[f]], 64)
		if err != nil {
			return fmt.Errorf("on row %d: field %q: %q: %v", ln, f, row[fields[f]], err)
		}

		m.Add(trait, key, w)
	}
	return nil
}

// TSV writes a matrix to a TSV file.
func (m *Matrix) TSV(w io.Writer) error {
	tab := csv.NewWriter(w)
	tab.Comma = '\t'
	tab.UseCRLF = true

	// header
	header := []string{"trait", "landscape", "weight"}
	if err := tab.Write(header); err != nil {
		return fmt.Errorf("unable to write header: %v", err)
	}

	traits := make([]string, 0, len(m.states))
	for s := range m.states {
		traits = append(traits, s)
	}
	slices.Sort(traits)

	landscape := make([]string, 0, len(m.labels))
	for l := range m.labels {
		landscape = append(landscape, l)
	}
	slices.Sort(landscape)

	for _, t := range traits {
		for _, l := range landscape {
			w := m.Weight(t, l)
			row := []string{
				t,
				l,
				strconv.FormatFloat(w, 'f', 6, 64),
			}
			if err := tab.Write(row); err != nil {
				return fmt.Errorf("when writing data: %v", err)
			}
		}
	}

	tab.Flush()
	if err := tab.Error(); err != nil {
		return fmt.Errorf("when writing data: %v", err)
	}
	return nil
}

// Weight returns the weight of a feature
// under a given trait state.
func (m *Matrix) Weight(trait, key string) float64 {
	trait = strings.Join(strings.Fields(strings.ToLower(trait)), " ")
	if trait == "" {
		return 0
	}
	t, ok := m.states[trait]
	if !ok {
		return 0
	}

	key = strings.Join(strings.Fields(strings.ToLower(key)), " ")
	if key == "" {
		return 0
	}
	k, ok := m.labels[key]
	if !ok {
		return 0
	}

	return m.m[t][k]
}
