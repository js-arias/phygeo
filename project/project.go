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
	"strings"
	"time"

	"golang.org/x/exp/slices"
)

// Dataset is a keyword to identify
// the type of a dataset file in a project.
type Dataset string

// Valid dataset types.
const (
	// File for the rotation model
	// of the paleogeographic reconstruction.
	GeoMod Dataset = "geomod"

	// File for pixel prior probability values.
	PixPrior Dataset ="pixprior"

	// File for presence-absence points
	// of the taxa in the project.
	Points Dataset = "points"

	// File for geographic distribution ranges
	// of the taxa in the project.
	Ranges Dataset = "ranges"

	// File for the pixel values
	// at different time stages.
	TimePix Dataset = "timepix"

	// File for phylogenetic trees.
	Trees Dataset = "trees"
)

// A Project represents a collection of paths
// for particular datasets.
type Project struct {
	paths map[Dataset]string
}

// New creates a new empty project.
func New() *Project {
	return &Project{
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
//	geomod	geo-model.tab
//	pixprior	pix-prior.tab
//	ranges	ranges.tab
//	timepix	pix-time.tab
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

// Write writes a project into a file with the indicated name.
func (p *Project) Write(name string) (err error) {
	f, err := os.Create(name)
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
		return fmt.Errorf("on file %q: while writing header: %v", name, err)
	}

	sets := p.Sets()
	for _, s := range sets {
		row := []string{
			string(s),
			p.paths[s],
		}
		if err := tsv.Write(row); err != nil {
			return fmt.Errorf("on file %q: %v", name, err)
		}
	}

	tsv.Flush()
	if err := tsv.Error(); err != nil {
		return fmt.Errorf("on file %q: while writing data: %v", name, err)
	}
	if err := bw.Flush(); err != nil {
		return fmt.Errorf("on file %q: while writing data: %v", name, err)
	}
	return nil
}
