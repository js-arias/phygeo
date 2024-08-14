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
	"strconv"
	"strings"

	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/phygeo/timestage"
	"github.com/js-arias/timetree"
)

var Command = &command.Command{
	Usage: `draw [--tree <tree>]
	[--scale <value>]
	[--step <value>] [--time <number>] [--tick <tick-value>]
	[--nonodes]
	[-o|--output <out-prefix>]
	<project-file>`,
	Short: "draw project trees as SVG files",
	Long: `
Command draw reads a PhyGeo project and draws the trees into a SVG-encoded
file.

The argument of the command is the name of the project file.

By default, the time scale is set in million years. To change the scale, use
the flag --scale with the value in years of the scale.

If the --time flag is defied, then a gray box of the indicated size, in
the scale units, will be printed as background.

By default, 10 pixel units will be used per scale unit; use the flag --step to
define a different value (it can have decimal points).

By default, all trees in the project will be drawn. If the flag --tree is set,
only the indicated tree will be printed.

By default, node IDs will be drawn. If the flag --nonodes is given, then it
will draw the tree without node IDs.

By default, a timescale with ticks every scale unit will be added at the
bottom of the drawing. Use the flag --tick to define the tick lines, using the
following format: "<min-tick>,<max-tick>,<label-tick>", in which min-tick
indicates minor ticks, max-tick indicates major ticks, and label-tick the
ticks that will be labeled; for example, the default is "1,5,5" which means
that small ticks will be added each scale unit, major ticks will be added
every 5 scale units, and labels will be added every 5 scale units.

By default, the names of the trees will be used as the output file names. Use
the flag -o, or --output, to define a prefix for the resulting files.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var noNodes bool
var stepX float64
var timeBox float64
var scale float64
var treeName string
var tickFlag string
var outPrefix string

func setFlags(c *command.Command) {
	c.Flags().BoolVar(&noNodes, "nonodes", false, "")
	c.Flags().Float64Var(&stepX, "step", 10, "")
	c.Flags().Float64Var(&timeBox, "time", 0, "")
	c.Flags().Float64Var(&scale, "scale", timestage.MillionYears, "")
	c.Flags().StringVar(&outPrefix, "output", "", "")
	c.Flags().StringVar(&outPrefix, "o", "", "")
	c.Flags().StringVar(&treeName, "tree", "", "")
	c.Flags().StringVar(&tickFlag, "tick", "", "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}
	tv, err := parseTick()
	if err != nil {
		return err
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
		if err := writeSVG(tn, copyTree(t, stepX, tv.min, tv.max, tv.label)); err != nil {
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

type tickValues struct {
	min   int
	max   int
	label int
}

func parseTick() (tickValues, error) {
	if tickFlag == "" {
		return tickValues{
			min:   1,
			max:   5,
			label: 5,
		}, nil
	}

	vals := strings.Split(tickFlag, ",")
	if len(vals) != 3 {
		return tickValues{}, fmt.Errorf("invalid tick values: %q", tickFlag)
	}

	min, err := strconv.Atoi(vals[0])
	if err != nil {
		return tickValues{}, fmt.Errorf("invalid minor tick value: %q: %v", tickFlag, err)
	}

	max, err := strconv.Atoi(vals[1])
	if err != nil {
		return tickValues{}, fmt.Errorf("invalid major tick value: %q: %v", tickFlag, err)
	}

	label, err := strconv.Atoi(vals[2])
	if err != nil {
		return tickValues{}, fmt.Errorf("invalid label tick value: %q: %v", tickFlag, err)
	}

	return tickValues{
		min:   min,
		max:   max,
		label: label,
	}, nil
}
