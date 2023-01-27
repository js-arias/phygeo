// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package list implements a command to print
// the list of trees in a phygeo project.
package list

import (
	"fmt"
	"os"

	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/timetree"
)

var Command = &command.Command{
	Usage: "list <project-file>",
	Short: "print a list of the trees in a project",
	Long: `
Command list reads the trees from a PhyGeo project and print the tree names in
the standard output.

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

	tf := p.Path(project.Trees)
	if tf == "" {
		return nil
	}

	tc, err := readTreeFile(tf)
	if err != nil {
		return err
	}

	ls := tc.Names()
	for _, t := range ls {
		fmt.Fprintf(c.Stdout(), "%s\n", t)
	}
	return nil
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
