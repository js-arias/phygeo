// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package model implement the model parameters
// and values used for inference with PhyGeo.
package model

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Type is the type of parameter.
type Type string

// Valid types
const (
	// Walk is for parameters of the random walk
	Walk Type = "walk"

	// Rate is for parameters of the rate category
	Rate = "rate"

	// Mov is for the movement weight parameters
	Mov = "mov"

	// Sett is for the settlement weight parameters
	Sett = "sett"

	// Trait os for trait transition parameters
	Trait = "trait"
)

// Params stores the information of a model variable
type params struct {
	// Name is the name of the variable
	name string

	// The type variable
	tp Type

	// ID of the parameter
	id int

	// Value store the current value of the variable
	val float64

	// Max store the maximum possible value of the variable
	// (only relevant if it is a parameter)
	max float64
}

// Model store the values and parameters of a model
type Model struct {
	vars []*params
}

// New creates a new set of values.
func New() *Model {
	return &Model{}
}

// Copy creates a copy of a model.
func (mp *Model) Copy() *Model {
	cp := &Model{}
	for _, p := range mp.vars {
		pp := &params{
			name: p.name,
			tp:   p.tp,
			id:   p.id,
			val:  p.val,
			max:  p.max,
		}
		cp.vars = append(cp.vars, pp)
	}
	return cp
}

// Add adds a new variable.
// If the id is less or equal to 0
// it is assumed to be a fixed value
func (mp *Model) Add(name string, tp Type, id int, val float64) error {
	name = strings.Join(strings.Fields(strings.ToLower(name)), " ")
	tp = getType(string(tp))
	if tp == "" {
		return fmt.Errorf("undefined type")
	}
	if _, dup := mp.getParam(name, tp); dup {
		return fmt.Errorf("duplicated variable %q (type %q)", name, tp)
	}

	if id <= 0 {
		id = 0
	}
	p := &params{
		name: name,
		tp:   tp,
		id:   id,
		val:  val,
		max:  math.Inf(1),
	}
	mp.vars = append(mp.vars, p)
	return nil
}

// AsParam sets a variable as a parameter
// with the indicated ID and maximum value.
func (mp *Model) AsParam(name string, tp Type, id int, max float64) {
	name = strings.Join(strings.Fields(strings.ToLower(name)), " ")
	p, ok := mp.getParam(name, tp)
	if !ok {
		return
	}
	if p.id > 0 {
		if id == 0 {
			// Set the variable as fixed
			p.id = 0
			return
		}
		if max < 0 {
			max = p.max
		}
	}
	if max < 0 {
		max = p.max
	}
	if max == 0 {
		max = math.Inf(1)
	}
	if tp == Mov || tp == Sett {
		max = 1
	}
	if p.val > max {
		p.val = max
	}
	p.id = id
}

// ID returns the ID of a model parameter.
// It returns 0 if the variable is not a parameter
// (i.e., it is a fixed value).
func (mp *Model) ID(name string, tp Type) int {
	name = strings.Join(strings.Fields(strings.ToLower(name)), " ")
	p, ok := mp.getParam(name, tp)
	if !ok {
		return 0
	}
	return p.id
}

// IDs return the IDs of the parameters defined
// for the model.
func (mp *Model) IDs() []int {
	v := make(map[int]bool)
	for _, p := range mp.vars {
		if p.id == 0 {
			continue
		}
		v[p.id] = true
	}

	ids := make([]int, 0, len(v))
	for id := range v {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// Fix sets a value as a fixed value.
func (mp *Model) Fix(name string, tp Type) {
	mp.AsParam(name, tp, 0, 0)
}

// Max returns the maximum value for a variable.
// If the variable is fixed,
// current value will be returned.
func (mp *Model) Max(name string, tp Type) float64 {
	name = strings.Join(strings.Fields(strings.ToLower(name)), " ")
	p, ok := mp.getParam(name, tp)
	if !ok {
		return 0
	}
	if p.id == 0 {
		return p.val
	}
	return p.max
}

// Names returns a slice with the names of the defined variables
// for a type of parameters in the model.
func (mp *Model) Names(tp Type) []string {
	tp = getType(string(tp))
	if tp == "" {
		return nil
	}

	var nm []string
	for _, p := range mp.vars {
		if p.tp != tp {
			continue
		}
		nm = append(nm, p.name)
	}
	slices.Sort(nm)
	return nm
}

// Set sets the value of a variable.
// If it is a param,
// it should be greater than zero
// and less than the maximum value.
// For a more secure method see Update.
func (mp *Model) Set(name string, tp Type, val float64) {
	name = strings.Join(strings.Fields(strings.ToLower(name)), " ")
	p, ok := mp.getParam(name, tp)
	if !ok {
		return
	}
	mp.set(p, val)
}

// SetMax sets the maximum value for a variable.
func (mp *Model) SetMax(name string, tp Type, max float64) {
	name = strings.Join(strings.Fields(strings.ToLower(name)), " ")
	p, ok := mp.getParam(name, tp)
	if !ok {
		return
	}
	if p.tp == Mov || p.tp == Sett {
		if max > 1 {
			max = 1
		}
	}
	p.max = max
	if p.val > max {
		p.val = max
	}
}

// Types return the types defined for the model parameters.
func (mp *Model) Types() []Type {
	s := make(map[string]bool)
	for _, p := range mp.vars {
		s[string(p.tp)] = true
	}

	tps := make([]Type, 0, len(s))
	for v := range s {
		tps = append(tps, Type(v))
	}
	slices.Sort(tps)
	return tps
}

// Update sets the value of a parameter.
// If the variable is fixed it does nothing.
// The value should be between 0 and the maximum
// or the value will not be updated.
func (mp *Model) Update(name string, tp Type, val float64) {
	name = strings.Join(strings.Fields(strings.ToLower(name)), " ")
	p, ok := mp.getParam(name, tp)
	if !ok {
		return
	}
	if p.id == 0 {
		return
	}
	mp.set(p, val)
}

// Val returns the value of a model variable.
func (mp *Model) Val(name string, tp Type) float64 {
	name = strings.Join(strings.Fields(strings.ToLower(name)), " ")
	p, ok := mp.getParam(name, tp)
	if !ok {
		return 0
	}
	return p.val
}

func (mp *Model) getParam(name string, tp Type) (*params, bool) {
	for _, p := range mp.vars {
		if p.tp != tp {
			continue
		}
		if p.name == name {
			return p, true
		}
	}
	return nil, false
}

func (mp *Model) set(p *params, val float64) {
	if p.id > 0 {
		if val == 0 {
			return
		}
		if val > p.max {
			val = p.max
		}
	}

	p.val = val
}

var header = []string{
	"type",
	"name",
	"param",
	"value",
	"max",
}

// Read reads a model parameters-variables from a TSV file.
//
// The TSV must contains the following fields:
//   - type, the type of the variable.
//   - name, the name of the variable.
//     A rate parameter
//     (different from "cats") is in the form
//     "<distribution>"
//     Movement and settlement variables are in the form
//     "<trait>:<landscape-feature>".
//     Traits variables are in the form
//     "<trait1>:<trait2>"
//     which indicates a trait1 -> trait2 transition.
//   - param, the ID for a parameter.
//     Parameters with the same ID will have the save value.
//     If not a parameters is set as "fixed".
//   - value, the current value of the variable.
//   - max, the maximum value of the variable,
//     if not defined,
//     it is assumed that it is unbounded.
//
// Here is an example file:
//
//	# phygeo model parameters
//	type	name	param	value	max
//	mov	land:lands	fixed	1
//	mov	land:ocean	3	1	1
//	mov	land:oceanic plateaus	3	1	1
//	rate	cats	fixed	9
//	rate	lognormal:sigma	2	1	2
//	sett	land:lands	fixed	1
//	sett	land:ocean	fixed	0
//	sett	land:oceanic plateaus	fixed	0.0001
//	walk	lambda	1	100	+Inf
//	walk	steps	fixed	120
func Read(f io.Reader) (*Model, error) {
	tsv := csv.NewReader(f)
	tsv.Comma = '\t'
	tsv.Comment = '#'

	head, err := tsv.Read()
	if err != nil {
		return nil, fmt.Errorf("header: %v", err)
	}
	fields := make(map[string]int, len(head))
	for i, h := range head {
		h = strings.ToLower(h)
		fields[h] = i
	}
	for _, h := range header {
		if _, ok := fields[h]; !ok {
			return nil, fmt.Errorf("expecting field %q", h)
		}
	}

	mp := New()
	for {
		row, err := tsv.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		ln, _ := tsv.FieldPos(0)
		if err != nil {
			return nil, fmt.Errorf("on row %d: %v", ln, err)
		}

		c := "type"
		tp := getType(row[fields[c]])
		if tp == "" {
			continue
		}

		c = "name"
		name := strings.Join(strings.Fields(strings.ToLower(row[fields[c]])), " ")
		if name == "" {
			continue
		}

		if _, dup := mp.getParam(name, tp); dup {
			return nil, fmt.Errorf("on row %d: duplicated variable %q (type %q)", ln, name, tp)
		}

		id := 0
		c = "param"
		cc := strings.ToLower(row[fields[c]])
		if cc != "" && cc != "fixed" {
			id, err = strconv.Atoi(cc)
			if err != nil {
				return nil, fmt.Errorf("on row %d, field %q: %v", ln, c, err)
			}
			if id < 0 {
				continue
			}
		}

		c = "value"
		cv, err := strconv.ParseFloat(row[fields[c]], 64)
		if err != nil {
			return nil, fmt.Errorf("on row %d, field %q: %v", ln, c, err)
		}

		if err := mp.Add(name, tp, id, cv); err != nil {
			return nil, fmt.Errorf("on row %d: %v", ln, err)
		}

		c = "max"
		if id > 0 && row[fields[c]] != "" {
			mv, err := strconv.ParseFloat(row[fields[c]], 64)
			if err != nil {
				return nil, fmt.Errorf("on row %d, field %q: %v", ln, c, err)
			}
			mp.SetMax(name, tp, mv)
		}
	}
	if len(mp.vars) == 0 {
		return nil, io.EOF
	}
	return mp, nil
}

func getType(s string) Type {
	s = strings.Join(strings.Fields(strings.ToLower(s)), " ")
	switch s {
	case string(Walk):
		return Walk
	case string(Rate):
		return Rate
	case string(Mov):
		return Mov
	case string(Sett):
		return Sett
	}
	return ""
}

// TSV writes a model into a TSV encoded file.
func (mp *Model) TSV(w io.Writer) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "# phygeo model parameters\n")
	fmt.Fprintf(bw, "# data save on: %s\n", time.Now().Format(time.RFC3339))
	tsv := csv.NewWriter(bw)
	tsv.Comma = '\t'
	tsv.UseCRLF = true

	if err := tsv.Write(header); err != nil {
		return fmt.Errorf("while writing header: %v", err)
	}

	for _, tp := range mp.Types() {
		for _, n := range mp.Names(tp) {
			p, _ := mp.getParam(n, tp)
			row := []string{
				string(tp),
				n,
				"fixed",
				strconv.FormatFloat(p.val, 'g', 6, 64),
				"",
			}
			if p.id > 0 {
				row[2] = strconv.Itoa(p.id)
				row[4] = strconv.FormatFloat(p.max, 'g', 6, 64)
			}
			if err := tsv.Write(row); err != nil {
				return fmt.Errorf("while writing data: %v", err)
			}
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
