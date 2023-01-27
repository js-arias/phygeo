// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package terms implements a command to print
// the list of taxa in a PhyGeo project
// with defined distribution ranges.
package taxa

import (
	"fmt"
	"os"

	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/ranges"
)

var Command = &command.Command{
	Usage: "taxa [--count] [--ranges] <project-file>",
	Short: "print a list of taxa with distribution ranges",
	Long: `
Command taxa reads the geographic ranges from a PhyGeo project and print the
name of the taxa in the standard output.

The argument of the command is the name of the project file.

By default the taxa of the points (presence-absence pixels) dataset will be
printed. If the flag --ranges is set, it will print the taxa of the continuous
distribution range file.

If the flag --count is defined, the number of pixels in the range will be
printed in front of each taxon name.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var countFlag bool
var rangeFlag bool

func setFlags(c *command.Command) {
	c.Flags().BoolVar(&countFlag, "count", false, "")
	c.Flags().BoolVar(&rangeFlag, "ranges", false, "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}

	p, err := project.Read(args[0])
	if err != nil {
		return err
	}

	rf := p.Path(project.Points)
	if rangeFlag {
		rf = p.Path(project.Ranges)
	}
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

	ls := coll.Taxa()
	for _, tax := range ls {
		if countFlag {
			rng := coll.Range(tax)
			fmt.Fprintf(c.Stdout(), "%s\t%d\n", tax, len(rng))
			continue
		}
		fmt.Fprintf(c.Stdout(), "%s\n", tax)
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
