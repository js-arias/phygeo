// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package prj implements a command to print
// the basic information of a project.
package prj

import (
	"fmt"
	"io"
	"math"
	"os"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/stat/pixprob"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/ranges"
	"github.com/js-arias/timetree"
)

var Command = &command.Command{
	Usage: "prj <project-file>",
	Short: "print information about a project",
	Long: `
Command prj reads a PhyGeo project and prints the information of the different
project elements into the standard output.

The argument of the command is the name of the project file.
	`,
	Run: run,
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}

	p, err := project.Read(args[0])
	if err != nil {
		return err
	}

	var pix *earth.Pixelation

	rotF := p.Path(project.GeoMotion)
	if rotF != "" {
		pix, err = readRotation(c.Stdout(), rotF)
		if err != nil {
			return err
		}
	}

	lsF := p.Path(project.Landscape)
	if lsF != "" {
		pix, err = readLandscape(c.Stdout(), lsF, pix)
		if err != nil {
			return err
		}
	}

	ppF := p.Path(project.PixPrior)
	if ppF != "" {
		if err := readPriors(c.Stdout(), ppF); err != nil {
			return err
		}
	}

	ptF := p.Path(project.Points)
	if ptF != "" {
		if err := readRanges(c.Stdout(), ptF, pix, project.Points); err != nil {
			return err
		}
	}

	ptR := p.Path(project.Ranges)
	if ptR != "" {
		if err := readRanges(c.Stdout(), ptR, pix, project.Ranges); err != nil {
			return err
		}
	}

	tF := p.Path(project.Trees)
	if tF != "" {
		if err := readTrees(c.Stdout(), tF); err != nil {
			return err
		}
	}

	return nil
}

const million = 1_000_000

func readRotation(w io.Writer, name string) (*earth.Pixelation, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rot, err := model.ReadTotal(f, nil, false)
	if err != nil {
		return nil, fmt.Errorf("on file %q: %v", name, err)
	}
	pix := rot.Pixelation()

	fmt.Fprintf(w, "Plate motion model:\n")
	fmt.Fprintf(w, "\tfile: %s\n", name)
	fmt.Fprintf(w, "\tpixelation: e%d\n", pix.Equator())

	stages := rot.Stages()
	min := float64(stages[0]) / million
	max := float64(stages[len(stages)-1]) / million
	fmt.Fprintf(w, "\tstages: %d [%.3f-%.3f Ma]\n", len(stages), min, max)
	fmt.Fprintf(w, "\n")

	return pix, nil
}

func readLandscape(w io.Writer, name string, pix *earth.Pixelation) (*earth.Pixelation, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tp, err := model.ReadTimePix(f, pix)
	if err != nil {
		return nil, fmt.Errorf("on file %q: %v", name, err)
	}
	if pix == nil {
		pix = tp.Pixelation()
	}

	fmt.Fprintf(w, "Paleo-Landscape model:\n")
	fmt.Fprintf(w, "\tfile: %s\n", name)
	fmt.Fprintf(w, "\tpixelation: e%d\n", pix.Equator())

	stages := tp.Stages()
	min := float64(stages[0]) / million
	max := float64(stages[len(stages)-1]) / million
	fmt.Fprintf(w, "\tstages: %d [%.3f-%.3f Ma]\n", len(stages), min, max)
	fmt.Fprintf(w, "\n")

	return pix, nil
}

func readPriors(w io.Writer, name string) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()

	pp, err := pixprob.ReadTSV(f)
	if err != nil {
		return fmt.Errorf("when reading %q: %v", name, err)
	}

	fmt.Fprintf(w, "Pixel priors:\n")
	fmt.Fprintf(w, "\tfile: %s\n", name)
	fmt.Fprintf(w, "\tdefined pixel types: %d\n", len(pp.Values()))
	fmt.Fprintf(w, "\n")

	return nil
}

func readRanges(w io.Writer, name string, pix *earth.Pixelation, tp project.Dataset) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()

	coll, err := ranges.ReadTSV(f, pix)
	if err != nil {
		return fmt.Errorf("when reading %q: %v", name, err)
	}

	fmt.Fprintf(w, "Terminal %s:\n", tp)
	fmt.Fprintf(w, "\tfile: %s\n", name)
	fmt.Fprintf(w, "\tdefined taxa: %d\n", len(coll.Taxa()))
	fmt.Fprintf(w, "\n")

	return nil
}

func readTrees(w io.Writer, name string) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()

	c, err := timetree.ReadTSV(f)
	if err != nil {
		return fmt.Errorf("while reading file %q: %v", name, err)
	}

	fmt.Fprintf(w, "Trees:\n")
	fmt.Fprintf(w, "\tfile: %s\n", name)

	terms := make(map[string]bool)
	min := math.MaxFloat64
	var max float64
	for _, tn := range c.Names() {
		t := c.Tree(tn)
		if t == nil {
			continue
		}
		ra := float64(t.Age(t.Root())) / million
		if ra > max {
			max = ra
		}

		for _, tax := range t.Terms() {
			terms[tax] = true
			id, ok := t.TaxNode(tax)
			if !ok {
				continue
			}
			ta := float64(t.Age(id)) / million
			if ta < min {
				min = ta
			}
		}
	}
	fmt.Fprintf(w, "\ttrees: %d\n", len(c.Names()))
	fmt.Fprintf(w, "\tterminals: %d\n", len(terms))
	fmt.Fprintf(w, "\tage range: %.3f-%.3f Ma\n", min, max)
	fmt.Fprintf(w, "\n")

	return nil
}
