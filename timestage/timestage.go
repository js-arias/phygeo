// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package timestage implements an slice with time stages
// in million years.
package timestage

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"
)

// MillionYears is the base unit for most analysis.
const MillionYears = 1_000_000

// A Stager is an interface for types
// that return a list of time stages.
type Stager interface {
	Stages() []int64
}

// Stages is a set of time stages
type Stages map[int64]bool

// New returns an empty set of time stages.
func New() Stages {
	return Stages(make(map[int64]bool))
}

// Read reads one or more time stages from a TSV file.
//
// The TSV must be without header
// and the first column should indicate the age
// (in years)
// of each stage.
// Any other columns will be ignored.
//
// Here is an example file
//
//	# time stages
//	0
//	5000000
//	10000000
//	100000000
//	200000000
//	300000000
func Read(r io.Reader) (Stages, error) {
	tsv := csv.NewReader(r)
	tsv.Comma = '\t'
	tsv.Comment = '#'

	st := New()
	for {
		row, err := tsv.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		ln, _ := tsv.FieldPos(0)
		if err != nil {
			return nil, fmt.Errorf("on line %d: %v", ln, err)
		}

		as := strings.TrimSpace(row[0])
		if as == "" {
			continue
		}
		a, err := strconv.ParseInt(as, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("on line %d: read %q: %v", ln, as, err)
		}
		st.AddStage(a)
	}

	return st, nil
}

// Add adds time stages from a stager.
func (s Stages) Add(ts Stager) {
	for _, a := range ts.Stages() {
		s[a] = true
	}
}

// AddStage adds a time stage.
func (s Stages) AddStage(a int64) {
	s[a] = true
}

// ClosestStageAge returns the closest stage age
// for a time
// (i.e., the age of the oldest state
// younger than the indicated age).
func (s Stages) ClosestStageAge(age int64) int64 {
	st := s.Stages()
	if i, ok := slices.BinarySearch(st, age); !ok {
		return st[i-1]
	}
	return age
}

// Stages returns a sorted slice
// of the defined time stages
func (s Stages) Stages() []int64 {
	st := make([]int64, 0, len(s))
	for a := range s {
		st = append(st, a)
	}
	slices.Sort(st)

	return st
}

// Write writes time stages into a tab-delimited file.
func (s Stages) Write(w io.Writer) (err error) {
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "# time stages\n")
	fmt.Fprintf(bw, "# data save on: %s\n", time.Now().Format(time.RFC3339))

	tsv := csv.NewWriter(bw)
	tsv.Comma = '\t'
	tsv.UseCRLF = true

	st := s.Stages()
	for _, a := range st {
		row := []string{
			strconv.FormatInt(a, 10),
		}
		if err := tsv.Write(row); err != nil {
			return err
		}
	}
	tsv.Flush()
	if err := tsv.Error(); err != nil {
		return fmt.Errorf("while writing data: %v", err)
	}
	if err := bw.Flush(); err != nil {
		return fmt.Errorf("while writing data: %v", err)
	}
	return nil
}
