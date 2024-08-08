// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package remove implements a command
// to remove tree terminals from a PhyGeo project
// without defined distribution ranges.
package remove

import (
	"fmt"
	"os"

	"github.com/js-arias/command"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/stat/pixprob"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/ranges"
	"github.com/js-arias/timetree"
)

var Command = &command.Command{
	Usage: "remove <project-file>",
	Short: "remove terminals without data",
	Long: `
Command remove reads the trees and geographic ranges from a PhyGeo project and
removes all tree terminals without a valid records.

To be valid, a terminal must have at least a single record defined on a pixel
in which the landscape value has a prior greater than zero.

The name of the removed terminal will be printed on the screen.
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

	rf := p.Path(project.Ranges)
	if rf == "" {
		return nil
	}
	coll, err := readRanges(rf)
	if err != nil {
		return err
	}
	if coll == nil {
		return nil
	}

	lsf := p.Path(project.Landscape)
	if lsf == "" {
		msg := fmt.Sprintf("paleolandscape not defined in project %q", args[0])
		return c.UsageError(msg)
	}
	landscape, err := readLandscape(lsf)
	if err != nil {
		return err
	}

	ppF := p.Path(project.PixPrior)
	if ppF == "" {
		msg := fmt.Sprintf("pixel priors not defined in project %q", args[0])
		return c.UsageError(msg)
	}
	pp, err := readPriors(ppF)
	if err != nil {
		return err
	}

	tf := p.Path(project.Trees)
	if tf == "" {
		msg := fmt.Sprintf("tree file not defined in project %q", args[0])
		return c.UsageError(msg)
	}
	tc, err := readTreeFile(tf)
	if err != nil {
		return err
	}

	changes := false
	for _, tn := range tc.Names() {
		t := tc.Tree(tn)
		if t == nil {
			continue
		}

		for _, tax := range t.Terms() {
			if coll.HasTaxon(tax) {
				rng := coll.Range(tax)
				age := landscape.ClosestStageAge(coll.Age(tax))
				lsc := landscape.Stage(age)
				valid := false
				for px := range rng {
					v := lsc[px]
					prior := pp.Prior(v)
					if prior > 0 {
						valid = true
						break
					}
				}
				if valid {
					continue
				}
			}
			id, ok := t.TaxNode(tax)
			if !ok {
				continue
			}

			if err := t.Delete(id); err != nil {
				return fmt.Errorf("unable to removed terminal %q [%d] of tree %s", tax, id, tn)
			}
			fmt.Fprintf(c.Stdout(), "tree %q: %s\n", tn, tax)
			changes = true
		}
	}

	if !changes {
		return nil
	}

	if err := writeTrees(tc, tf); err != nil {
		return err
	}
	return nil
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

func readLandscape(name string) (*model.TimePix, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tp, err := model.ReadTimePix(f, nil)
	if err != nil {
		return nil, fmt.Errorf("on file %q: %v", name, err)
	}

	return tp, nil
}

func readPriors(name string) (pixprob.Pixel, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	pp, err := pixprob.ReadTSV(f)
	if err != nil {
		return nil, fmt.Errorf("when reading %q: %v", name, err)
	}

	return pp, nil
}

func readTreeFile(name string) (*timetree.Collection, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	c, err := timetree.ReadTSV(f)
	if err != nil {
		return nil, fmt.Errorf("while reading file %q: %v", name, err)
	}
	return c, nil
}

func writeTrees(tc *timetree.Collection, treeFile string) (err error) {
	f, err := os.Create(treeFile)
	if err != nil {
		return err
	}
	defer func() {
		e := f.Close()
		if e != nil && err == nil {
			err = e
		}
	}()

	if err := tc.TSV(f); err != nil {
		return fmt.Errorf("while writing to %q: %v", treeFile, err)
	}
	return nil
}
