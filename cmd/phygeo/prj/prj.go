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
	"github.com/js-arias/earth/pixkey"
	"github.com/js-arias/earth/stat/pixweight"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/phygeo/timestage"
	"github.com/js-arias/phygeo/trait"
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

	stages := timestage.New()

	rotF := p.Path(project.GeoMotion)
	if rotF != "" {
		pix, err = readRotation(c.Stdout(), rotF, stages)
		if err != nil {
			return err
		}
	}

	lsF := p.Path(project.Landscape)
	if lsF != "" {
		pix, err = readLandscape(c.Stdout(), lsF, pix, stages)
		if err != nil {
			return err
		}
	}

	stF := p.Path(project.Stages)
	if err := readTimeStages(c.Stdout(), stF, stages); err != nil {
		return err
	}

	var keys *pixkey.PixKey
	if p.Path(project.Keys) != "" {
		keys, err = readPixKeys(c.Stdout(), p)
		if err != nil {
			return err
		}
	}

	pwF := p.Path(project.PixWeight)
	if pwF != "" {
		if err := readPixWeights(c.Stdout(), pwF); err != nil {
			return err
		}
	}

	ptR := p.Path(project.Ranges)
	if ptR != "" {
		if err := readRanges(c.Stdout(), ptR, pix, project.Ranges); err != nil {
			return err
		}
	}

	var traits *trait.Data
	if p.Path(project.Traits) != "" {
		traits, err = readTraitData(c.Stdout(), p)
		if err != nil {
			return err
		}
	}

	if p.Path(project.Movement) != "" {
		if err := readMovementMatrix(c.Stdout(), p, traits, keys); err != nil {
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

func readRotation(w io.Writer, name string, st timestage.Stages) (*earth.Pixelation, error) {
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

	st.Add(rot)
	stages := rot.Stages()
	min := float64(stages[0]) / timestage.MillionYears
	max := float64(stages[len(stages)-1]) / timestage.MillionYears
	fmt.Fprintf(w, "\tstages: %d [%.3f-%.3f Ma]\n", len(stages), min, max)
	fmt.Fprintf(w, "\n")

	return pix, nil
}

func readLandscape(w io.Writer, name string, pix *earth.Pixelation, st timestage.Stages) (*earth.Pixelation, error) {
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

	fmt.Fprintf(w, "Landscape model:\n")
	fmt.Fprintf(w, "\tfile: %s\n", name)
	fmt.Fprintf(w, "\tpixelation: e%d\n", pix.Equator())

	stages := tp.Stages()
	vs := make(map[int]bool)
	vs[0] = true
	for _, age := range stages {
		s := tp.Stage(age)
		for _, v := range s {
			vs[v] = true
		}
	}
	fmt.Fprintf(w, "\tdefined pixel types: %d\n", len(vs))

	st.Add(tp)
	min := float64(stages[0]) / timestage.MillionYears
	max := float64(stages[len(stages)-1]) / timestage.MillionYears
	fmt.Fprintf(w, "\tstages: %d [%.3f-%.3f Ma]\n", len(stages), min, max)
	fmt.Fprintf(w, "\n")

	return pix, nil
}

func readTimeStages(w io.Writer, name string, stages timestage.Stages) error {
	fmt.Fprintf(w, "Time stages:\n")

	if name != "" {
		fmt.Fprintf(w, "\tfile: %s\n", name)

		f, err := os.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()

		st, err := timestage.Read(f)
		if err != nil {
			return fmt.Errorf("on file %q: %v", name, err)
		}
		stages.Add(st)
	}

	st := stages.Stages()
	min := float64(st[0]) / timestage.MillionYears
	max := float64(st[len(st)-1]) / timestage.MillionYears
	fmt.Fprintf(w, "\tstages: %d [%.3f-%.3f Ma]\n", len(stages), min, max)
	fmt.Fprintf(w, "\n")

	return nil
}

func readMovementMatrix(w io.Writer, p *project.Project, traits *trait.Data, keys *pixkey.PixKey) error {
	m, err := p.Movement(traits, keys)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "Movement matrix:\n")
	fmt.Fprintf(w, "\tfile: %s\n", p.Path(project.Movement))
	fmt.Fprintf(w, "\tdefined trait states: %d\n", len(m.Traits()))
	fmt.Fprintf(w, "\tdefined landscape features: %d\n", len(m.Landscape()))
	fmt.Fprintf(w, "\n")

	return nil
}

func readPixKeys(w io.Writer, p *project.Project) (keys *pixkey.PixKey, err error) {
	keys, err = p.Keys()
	if err != nil {
		return nil, fmt.Errorf("when reading %q: %v", p.Path(project.Keys), err)
	}

	c := make(map[int]bool)
	g := make(map[int]bool)
	l := make(map[int]bool)
	for _, v := range keys.Keys() {
		if _, ok := keys.Color(v); ok {
			c[v] = true
		}
		if _, ok := keys.Gray(v); ok {
			g[v] = true
		}
		if tx := keys.Label(v); tx != "" {
			l[v] = true
		}
	}

	fmt.Fprintf(w, "Pixel keys:\n")
	fmt.Fprintf(w, "\tfile: %s\n", p.Path(project.Keys))
	fmt.Fprintf(w, "\tdefined pixel values-types: %d\n", len(keys.Keys()))
	fmt.Fprintf(w, "\tpixel values with color: %d\n", len(c))
	fmt.Fprintf(w, "\tpixel values with gray color: %d\n", len(g))
	fmt.Fprintf(w, "\tpixel values with label: %d\n", len(l))
	fmt.Fprintf(w, "\n")

	return keys, nil
}

func readPixWeights(w io.Writer, name string) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()

	pw, err := pixweight.ReadTSV(f)
	if err != nil {
		return fmt.Errorf("when reading %q: %v", name, err)
	}

	fmt.Fprintf(w, "Pixel weights:\n")
	fmt.Fprintf(w, "\tfile: %s\n", name)
	fmt.Fprintf(w, "\tdefined pixel types: %d\n", len(pw.Values()))
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

func readTraitData(w io.Writer, p *project.Project) (d *trait.Data, err error) {
	d, err = p.Traits()
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(w, "Traits\n")
	fmt.Fprintf(w, "\tfile: %s\n", p.Path(project.Traits))
	fmt.Fprintf(w, "\tdefined taxa: %d\n", len(d.Taxa()))
	fmt.Fprintf(w, "\tdefined trait states: %d\n", len(d.States()))
	fmt.Fprintf(w, "\n")

	return d, nil
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
		ra := float64(t.Age(t.Root())) / timestage.MillionYears
		if ra > max {
			max = ra
		}

		for _, tax := range t.Terms() {
			terms[tax] = true
			id, ok := t.TaxNode(tax)
			if !ok {
				continue
			}
			ta := float64(t.Age(id)) / timestage.MillionYears
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
