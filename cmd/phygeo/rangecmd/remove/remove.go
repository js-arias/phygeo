// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package remove implements a command
// to remove range distribution records
// not present on a tree.
package remove

import (
	"fmt"
	"os"

	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/ranges"
)

var Command = &command.Command{
	Usage: "remove <project-file>",
	Short: "remove distribution ranges absent in tree",
	Long: `
Package remove reads the geographic ranges from a PhyGeo project and removes
all ranges that are not defined as terminals of the phylogenetic trees of the
project.

The name of the removed distribution ranges will be printed on the screen.

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

	coll, err := p.Ranges(nil)
	if err != nil {
		return err
	}
	if coll == nil {
		return nil
	}

	ls, err := makeTermList(p)
	if err != nil {
		return nil
	}

	changed := false
	for _, tax := range coll.Taxa() {
		if _, ok := ls[tax]; ok {
			continue
		}
		coll.Delete(tax)
		fmt.Fprintf(os.Stdin, "%s\n", tax)
		changed = true
	}

	if !changed {
		return nil
	}

	if err := writeCollection(p.Path(project.Ranges), coll); err != nil {
		return err
	}
	return nil
}

func makeTermList(p *project.Project) (map[string]bool, error) {
	c, err := p.Trees()
	if err != nil {
		return nil, err
	}

	ls := c.Names()
	terms := make(map[string]bool)

	for _, tn := range ls {
		t := c.Tree(tn)
		if t == nil {
			continue
		}
		for _, tax := range t.Terms() {
			terms[tax] = true
		}
	}
	return terms, nil
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
