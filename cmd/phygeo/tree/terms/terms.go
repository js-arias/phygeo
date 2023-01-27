// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package terms implements a command to print
// the list of the terminals in the trees of a PhyGeo project.
package terms

import (
	"fmt"
	"os"

	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/timetree"
	"golang.org/x/exp/slices"
)

var Command = &command.Command{
	Usage: "terms [--tree <tree-name>] <project-file>",
	Short: "print a list of tree terminals",
	Long: `
Command terms reads the trees from a PhyGeo project and print the name of the
terminals in the standard output.

The argument of the command is the name of the project file.

By default all terminals will be printed. If the flag --tree is set, only the
terminals of the indicated tree will be printed.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var treeName string

func setFlags(c *command.Command) {
	c.Flags().StringVar(&treeName, "tree", "", "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}

	p, err := project.Read(args[0])
	if err != nil {
		return err
	}

	tf := p.Path(project.Trees)
	if tf == "" {
		return nil
	}

	ls, err := makeTermList(tf)
	if err != nil {
		return nil
	}
	for _, term := range ls {
		fmt.Fprintf(c.Stdout(), "%s\n", term)
	}

	return nil
}

func makeTermList(name string) ([]string, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	c, err := timetree.ReadTSV(f)
	if err != nil {
		return nil, fmt.Errorf("while reading file %q: %v", name, err)
	}

	var ls []string
	if treeName != "" {
		ls = append(ls, treeName)
	} else {
		ls = c.Names()
	}

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

	termList := make([]string, 0, len(terms))
	for tax := range terms {
		termList = append(termList, tax)
	}
	slices.Sort(termList)

	return termList, nil
}
