// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package project implements reading and writing
// of PhyGeo project files.
//
// A PhyGeo project is a tab-delimited file (TSV)
// used to store the different data files
// required by PhyGeo commands.
package project

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"
)

// Dataset is a keyword to identify
// the type of a dataset file in a project.
type Dataset string

// Valid dataset types.
const (
	// File for the plate motion model
	// of the paleogeographic reconstruction.
	GeoMotion Dataset = "geomotion"

	// File for pixel key values
	// (color and pixel labels).
	Keys Dataset = "keys"

	// File for the movement matrix used in random walks.
	Movement = "movement"

	// File for pixel normalized weights
	// (a form of pixel prior).
	PixWeight Dataset = "pixweight"

	// File for geographic distribution ranges
	// of the taxa in the project.
	Ranges Dataset = "ranges"

	// File for the landscape pixel values
	// at different time stages.
	Landscape Dataset = "landscape"

	// File for trait observations.
	Traits Dataset = "traits"

	// File for phylogenetic trees.
	Trees Dataset = "trees"

	// File for the settlement matrix used in random walks.
	Settlement = "settlement"

	// File for the time stages.
	Stages Dataset = "stages"
)

// A Project represents a collection of paths
// for particular datasets.
type Project struct {
	name  string
	paths map[Dataset]string
}

// New creates a new empty project.
func New() *Project {
	return &Project{
		name:  "",
		paths: make(map[Dataset]string),
	}
}

var header = []string{
	"dataset",
	"path",
}

// Read reads a project file from a TSV file.
//
// The TSV must contain the following fields:
//
//   - dataset, for the kind of file
//   - path, for the path of the file
//
// Here is an example file:
//
//	# phygeo project files
//	dataset	path
//	geomotion	geo-motion.tab
//	pixweight	pix-weights.tab
//	ranges	ranges.tab
//	landscape	landscape.tab
//	trees	trees.tab
func Read(name string) (*Project, error) {
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

	p := New()
	p.name = name
	for {
		row, err := tsv.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		ln, _ := tsv.FieldPos(0)
		if err != nil {
			return nil, fmt.Errorf("on file %q: on row %d: %v", name, ln, err)
		}

		f := "dataset"
		s := Dataset(row[fields[f]])

		f = "path"
		path := row[fields[f]]
		p.paths[s] = path
	}

	return p, nil
}

// Add adds a filepath of a dataset to a given project.
// It returns the previous value
// for the dataset.
func (p *Project) Add(set Dataset, path string) string {
	prev := p.paths[set]
	if path == "" {
		delete(p.paths, set)
		return prev
	}

	p.paths[set] = path
	return prev
}

// Path returns the path of the given dataset.
func (p *Project) Path(set Dataset) string {
	return p.paths[set]
}

// Sets returns the datasets defined on a project.
func (p *Project) Sets() []Dataset {
	var sets []Dataset
	for s := range p.paths {
		sets = append(sets, s)
	}
	slices.Sort(sets)
	return sets
}

// SetName sets the project file name.
func (p *Project) SetName(name string) {
	p.name = name
}

// Write writes a project into a file.
func (p *Project) Write() (err error) {
	f, err := os.Create(p.name)
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
	fmt.Fprintf(bw, "# phygeo project files\n")
	fmt.Fprintf(bw, "# data save on: %s\n", time.Now().Format(time.RFC3339))
	tsv := csv.NewWriter(bw)
	tsv.Comma = '\t'
	tsv.UseCRLF = true

	if err := tsv.Write(header); err != nil {
		return fmt.Errorf("on file %q: while writing header: %v", p.name, err)
	}

	sets := p.Sets()
	for _, s := range sets {
		row := []string{
			string(s),
			p.paths[s],
		}
		if err := tsv.Write(row); err != nil {
			return fmt.Errorf("on file %q: %v", p.name, err)
		}
	}

	tsv.Flush()
	if err := tsv.Error(); err != nil {
		return fmt.Errorf("on file %q: while writing data: %v", p.name, err)
	}
	if err := bw.Flush(); err != nil {
		return fmt.Errorf("on file %q: while writing data: %v", p.name, err)
	}
	return nil
}
