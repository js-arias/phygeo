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
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/timetree"
)

var Command = &command.Command{
	Usage: "remove <project-file>",
	Short: "remove terminals without data",
	Long: `
Command remove reads the trees and geographic ranges from a PhyGeo project and
removes all tree terminals without a valid records.

To be valid, a terminal must have at least a single record defined on a pixel
in which the landscape value has a weight greater than zero.

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

	landscape, err := p.Landscape(nil)
	if err != nil {
		return err
	}

	coll, err := p.Ranges(landscape.Pixelation())
	if err != nil {
		return err
	}
	if coll == nil {
		return nil
	}

	pw, err := p.PixWeight()
	if err != nil {
		return err
	}

	tc, err := p.Trees()
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
					weight := pw.Weight(v)
					if weight > 0 {
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

	if err := writeTrees(tc, p.Path(project.Trees)); err != nil {
		return err
	}
	return nil
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
