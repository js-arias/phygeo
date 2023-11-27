// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package draw implements a command to draw
// trees in a phygeo project as SVG files.
package draw

import (
	"bufio"
	"fmt"
	"os"

	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/timetree"
)

var Command = &command.Command{
	Usage: `draw [--tree <tree>] [--step <value>] [--time <number>]
	[-o|--output <out-prefix>]
	<project-file>`,
	Short: "draw project trees as SVG files",
	Long: `
Command draw reads a PhyGeo project and draws the trees into a SVG-encoded
file.

The argument of the command is the name of the project file.

If the --time flag is defied, then a gray box of the indicated size, in
million years, will be printed as background.

By default, 10 pixel units will be used per million years; use the flag --step
to define a different value (it can have decimal points).

By default, all trees in the project will be drawn. If the flag --tree is set,
only the indicated tree will be printed.

By default, the names of the trees will be used as the output file names. Use
the flag -o, or --output, to define a prefix for the resulting files.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var stepX float64
var timeBox float64
var treeName string
var outPrefix string

func setFlags(c *command.Command) {
	c.Flags().Float64Var(&stepX, "step", 10, "")
	c.Flags().Float64Var(&timeBox, "time", 0, "")
	c.Flags().StringVar(&outPrefix, "output", "", "")
	c.Flags().StringVar(&outPrefix, "o", "", "")
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

	tc, err := readTreeFile(tf)
	if err != nil {
		return err
	}

	ls := tc.Names()
	for _, tn := range ls {
		t := tc.Tree(tn)
		if err := writeSVG(tn, copyTree(t, stepX)); err != nil {
			return err
		}
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

func writeSVG(name string, t svgTree) (err error) {
	if outPrefix != "" {
		name = fmt.Sprintf("%s-%s.svg", outPrefix, name)
	} else {
		name += ".svg"
	}

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

	bw := bufio.NewWriter(f)
	if err := t.draw(bw); err != nil {
		return fmt.Errorf("while writing file %q: %v", name, err)
	}
	if err := bw.Flush(); err != nil {
		return fmt.Errorf("while writing file %q: %v", name, err)
	}
	return nil
}
