// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package rotate implements a command to rotate
// the point records of a phygeo project.
package rotate

import (
	"fmt"
	"os"

	"github.com/js-arias/command"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/ranges"
	"github.com/js-arias/timetree"
)

var Command = &command.Command{
	Usage: "rotate <project-file>",
	Short: "rotate point records",
	Long: `
Command rotate reads the point locations from a PhyGeo project, as well as the
trees, and uses the rotation model defined in the project to set the location
of fossil taxa in the project to their past location.

The argument of the command is the name of the project file.

The command requires that the project have a defined tree file, and the age of
the terminals will be used to define the time stage for the rotation.
	`,
	Run: run,
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}
	pFile := args[0]
	p, err := project.Read(pFile)
	if err != nil {
		return err
	}

	tf := p.Path(project.Trees)
	if tf == "" {
		msg := fmt.Sprintf("tree file not defined in project %q", args[0])
		return c.UsageError(msg)
	}
	ages, err := readTermAges(tf)
	if err != nil {
		return err
	}

	rotF := p.Path(project.GeoMotion)
	if rotF == "" {
		msg := fmt.Sprintf("plate motion model not defined in project %q", args[0])
		return c.UsageError(msg)
	}
	tot, err := readRotation(rotF)
	if err != nil {
		return err
	}

	pf := p.Path(project.Points)
	pts, err := readRanges(pf)
	if err != nil {
		return err
	}

	for _, tax := range pts.Taxa() {
		a := ages[tax]
		if a == 0 {
			continue
		}

		// ignore taxa already rotated
		if pts.Age(tax) > 0 {
			continue
		}

		rng := pts.Range(tax)

		rot := tot.Rotation(a)
		n := make(map[int]float64, len(rng))
		for px := range rng {
			dst := rot[px]
			for _, np := range dst {
				n[np] = 1.0
			}
		}
		if len(n) == 0 {
			fmt.Fprintf(c.Stderr(), "WARNING: taxon %q: undefined pixels at age %.6f\n", tax, float64(a)/millionYears)
		}
		pts.SetPixels(tax, a, n)
	}

	if err := writeCollection(pf, pts); err != nil {
		return err
	}
	p.Add(project.Points, pf)

	if err := p.Write(pFile); err != nil {
		return err
	}
	return nil
}

const millionYears = 1_000_000

func readTermAges(name string) (map[string]int64, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	c, err := timetree.ReadTSV(f)
	if err != nil {
		return nil, fmt.Errorf("while reading file %q: %v", name, err)
	}

	ages := make(map[string]int64)
	for _, tn := range c.Names() {
		t := c.Tree(tn)
		if t == nil {
			continue
		}
		for _, tax := range t.Terms() {
			id, ok := t.TaxNode(tax)
			if !ok {
				continue
			}
			a := t.Age(id)
			if ages[tax] > a {
				continue
			}
			ages[tax] = a
		}
	}

	return ages, nil
}

func readRotation(name string) (*model.Total, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rot, err := model.ReadTotal(f, nil, false)
	if err != nil {
		return nil, fmt.Errorf("on file %q: %v", name, err)
	}

	return rot, nil
}

func readRanges(name string) (*ranges.Collection, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	coll, err := ranges.ReadTSV(f, nil)
	if err != nil {
		return nil, fmt.Errorf("when reading %q: %v", name, err)
	}

	return coll, nil
}

func writeCollection(name string, coll *ranges.Collection) (err error) {
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

	if err := coll.TSV(f); err != nil {
		return fmt.Errorf("while writing to %q: %v", name, err)
	}
	return nil
}
