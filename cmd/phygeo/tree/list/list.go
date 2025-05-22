// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package list implements a command to print
// the list of trees in a phygeo project.
package list

import (
	"fmt"

	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/project"
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

	tc, err := p.Trees()
	if err != nil {
		return err
	}

	ls := tc.Names()
	for _, t := range ls {
		fmt.Fprintf(c.Stdout(), "%s\n", t)
	}
	return nil
}
