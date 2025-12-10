// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package walkparam implements reading and writing
// of the PhyGeo parameters for a random walk.
package walkparam

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/js-arias/earth"
	"github.com/js-arias/phygeo/cats"
	"gonum.org/v1/gonum/stat/distuv"
)

// Param is a keyword to identify
// the type of parameter is a walkParam file.
type Param string

// Valid parameters
const (
	// Cats is the number of categories
	// used in a relaxed random walk.
	Cats Param = "cats"

	// MinSteps is the minimum number of steps
	// in a terminal branch.
	MinSteps Param = "minsteps"

	// Relaxed is the function used for the scalar values
	// in the relaxed random walk.
	Relaxed Param = "relaxed"

	// Steps is the number of steps per million year.
	Steps Param = "steps"
)

// WP represents a collection of a random walk parameters.
type WP struct {
	name string // file name

	// relaxed random walk
	r string // the type of relaxed random walk
	c int    // categories

	// steps
	min   int
	steps int
}

// New creates a new parameter collection.
func New(name string, pix *earth.Pixelation) *WP {
	return &WP{
		name:  name,
		r:     "lognormal",
		c:     9,
		steps: pix.Equator(),
	}
}

var header = []string{
	"parameter",
	"value",
}

// Read reads a walkParam file from a TSV file.
//
// The TSV must contains the following fields:
//
//   - parameter, the name of the parameter
//   - value, the value of the parameter
//
// Here is an example file:
//
//	# phygeo random walk parameters
//	parameter	value
//	relaxed	LogNormal
//	cats	9
//	steps	360
func Read(name string, pix *earth.Pixelation) (*WP, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tsv := csv.NewReader(f)
	tsv.Comma = '\t'
	tsv.Comment = '#'

	head, err := tsv.Read()
	if err != nil {
		return nil, fmt.Errorf("on file %q: header: %v", name, err)
	}
	fields := make(map[string]int, len(head))
	for i, h := range head {
		h = strings.ToLower(h)
		fields[h] = i
	}
	for _, h := range header {
		if _, ok := fields[h]; !ok {
			return nil, fmt.Errorf("on file %q: expecting field %q", name, h)
		}
	}

	wp := New(name, pix)
	for {
		row, err := tsv.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		ln, _ := tsv.FieldPos(0)
		if err != nil {
			return nil, fmt.Errorf("on file %q: on row %d: %v", name, ln, err)
		}

		f := "parameter"
		p := Param(strings.ToLower(row[fields[f]]))

		f = "value"
		switch p {
		case Cats:
			c, err := strconv.Atoi(row[fields[f]])
			if err != nil {
				return nil, fmt.Errorf("on file %q: on row %d, field %q: %v", name, ln, f, err)
			}
			wp.c = c
		case MinSteps:
			m, err := strconv.Atoi(row[fields[f]])
			if err != nil {
				return nil, fmt.Errorf("on file %q: on row %d, field %q: %v", name, ln, f, err)
			}
			wp.min = m
		case Relaxed:
			r := strings.ToLower(row[fields[f]])
			switch r {
			case "gamma":
			case "lognormal":
			default:
				return nil, fmt.Errorf("on file %q: on row %d, field %q: unknown function %q", name, ln, f, r)
			}
			wp.r = r
		case Steps:
			s, err := strconv.Atoi(row[fields[f]])
			if err != nil {
				return nil, fmt.Errorf("on file %q: on row %d, field %q: %v", name, ln, f, err)
			}
			wp.steps = s
		}
	}
	return wp, nil
}

// Cats returns the number of categories
// for the relaxed random walk.
func (wp *WP) Cats() int {
	return wp.c
}

// Function returns the distribution function
// used for the scalars of the relaxed random walk.
func (wp *WP) Function() string {
	return wp.r
}

// Name returns the name used for a set of parameters
// for a random walk.
func (wp *WP) Name() string {
	return wp.name
}

// MinSteps returns the minimum number of steps
// in a terminal branch.
func (wp *WP) MinSteps() int {
	return wp.min
}

// Relaxed returns a scalar function
// for relaxed random walks using the indicated parameters.
func (wp *WP) Relaxed(param []float64) cats.Discrete {
	switch wp.r {
	case "gamma":
		if len(param) < 1 {
			param = []float64{1.0}
		}
		return cats.Gamma{
			Param: distuv.Gamma{
				Alpha: param[0],
				Beta:  param[0],
			},
			NumCat: wp.c,
		}
	case "lognormal":
		if len(param) < 1 {
			param = []float64{1.0}
		}
		return cats.LogNormal{
			Param: distuv.LogNormal{
				Mu:    0,
				Sigma: param[0],
			},
			NumCat: wp.c,
		}
	}
	panic("invalid relaxed function")
}

// SetCats sets the number of categories
// for a relaxed random walk.
func (wp *WP) SetCats(c int) error {
	if c < 1 {
		return fmt.Errorf("invalid number of categories: %d", c)
	}
	wp.c = c
	return nil
}

// SetName sets the name of a parameter collection.
func (wp *WP) SetName(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	wp.name = name
}

// SetMinSteps sets the number of minimum steps
// in a terminal branch.
func (wp *WP) SetMinSteps(m int) error {
	if m < 0 {
		return fmt.Errorf("invalid steps value: %d", m)
	}
	wp.min = m
	return nil
}

// SetRelaxed sets the function used for the scalar
// of the relaxed random walk.
func (wp *WP) SetRelaxed(r string) error {
	r = strings.ToLower(strings.TrimSpace(r))
	switch r {
	case "gamma":
	case "lognormal":
	default:
		return fmt.Errorf("unknown function %q", r)
	}
	wp.r = r
	return nil
}

// SetSteps sets the number of steps per million years.
func (wp *WP) SetSteps(s int) error {
	if s < 1 {
		return fmt.Errorf("invalid steps value: %d", s)
	}
	wp.steps = s
	return nil
}

// Steps returns the number of steps per million years.
func (wp *WP) Steps() int {
	return wp.steps
}

// Write writes a parameter collection into a file.
func (wp *WP) Write() (err error) {
	f, err := os.Create(wp.name)
	if err != nil {
		return err
	}
	defer func() {
		e := f.Close()
		if e != nil && err == nil {
			err = e
		}
	}()

	bw := bufio.NewWriter(f)
	fmt.Fprintf(bw, "# phygeo random walk parameters\n")
	fmt.Fprintf(bw, "# data save on: %s\n", time.Now().Format(time.RFC3339))
	tsv := csv.NewWriter(bw)
	tsv.Comma = '\t'
	tsv.UseCRLF = true

	if err := tsv.Write(header); err != nil {
		return fmt.Errorf("on file %q: while writing header: %v", wp.name, err)
	}

	row := []string{
		string(Cats),
		strconv.Itoa(wp.c),
	}
	if err := tsv.Write(row); err != nil {
		return fmt.Errorf("on file %q: %v", wp.name, err)
	}

	row = []string{
		string(Relaxed),
		wp.r,
	}
	if err := tsv.Write(row); err != nil {
		return fmt.Errorf("on file %q: %v", wp.name, err)
	}

	row = []string{
		string(Steps),
		strconv.Itoa(wp.steps),
	}
	if err := tsv.Write(row); err != nil {
		return fmt.Errorf("on file %q: %v", wp.name, err)
	}

	row = []string{
		string(MinSteps),
		strconv.Itoa(wp.min),
	}
	if err := tsv.Write(row); err != nil {
		return fmt.Errorf("on file %q: %v", wp.name, err)
	}

	tsv.Flush()
	if err := tsv.Error(); err != nil {
		return fmt.Errorf("on file %q: while writing data: %v", wp.name, err)
	}
	if err := bw.Flush(); err != nil {
		return fmt.Errorf("on file %q: while writing data: %v", wp.name, err)
	}
	return nil
}
