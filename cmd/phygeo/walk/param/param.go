// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package param implements a command to manege
// somo common parameters for random walks.
package param

import (
	"fmt"
	"io"

	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/phygeo/walkparam"
)

var Command = &command.Command{
	Usage: `param [--add <param-file>] [--file <file-name>]
	[--cats <value>] [--func <value>]
	[--steps <value>] [--min <value>]
	<project-file>`,
	Short: "manage random walk parameters",
	Long: `
Command param manages some of the basic parameters of the random walk defined
for a PhyGeo project. These parameters define the main form of the relaxed
random walk, as well as the number of steps used in the random walk.

The argument of the command is the name of the project file.

By default, the command will print the currently defined parameters.

If the flag --add is defined, it will use the indicated file for the random
walk parameters.

By default, any change on the parameters will be stored in the current
parameters file. Use the flag --file to define a new parameters file.

To set the number of categories in the relaxed random walk use the flag
--cats. The value set as default for a project is 9. To set the function used
to retrieve the scalars of used for the relaxed random walk use the flag
--func. Valid values are (lognormal is the default for a project):

	- Gamma: with a single parameter (both alpha and beta set as equal).
	- LogNormal: with a single parameter (sigma), the mean is 1.

The number of steps is set by default to the number of pixels in the equator.
To set a different number use the flag --steps. Some terminal branches might
be to small, so a minimum number of steps for that branches can be defined
with the flag --min. By default, the minimum number is not bounded.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var addFile string
var paramFile string
var funcName string
var cats int
var steps int
var minSteps int

func setFlags(c *command.Command) {
	c.Flags().StringVar(&addFile, "add", "", "")
	c.Flags().StringVar(&paramFile, "file", "", "")
	c.Flags().StringVar(&funcName, "func", "", "")
	c.Flags().IntVar(&cats, "cats", 0, "")
	c.Flags().IntVar(&steps, "steps", 0, "")
	c.Flags().IntVar(&minSteps, "min", 0, "")
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

	wp, err := p.WalkParam(landscape.Pixelation())
	if err != nil {
		return err
	}

	if addFile != "" {
		if _, err := walkparam.Read(addFile, landscape.Pixelation()); err != nil {
			return err
		}
		p.Add(project.WalkParam, addFile)
		if err := p.Write(); err != nil {
			return err
		}
		return nil
	}

	if paramFile != "" {
		wp.SetName(paramFile)
	}

	ed := false
	if cats > 0 {
		if err := wp.SetCats(cats); err != nil {
			return err
		}
		ed = true
	}
	if funcName != "" {
		if err := wp.SetRelaxed(funcName); err != nil {
			return err
		}
		ed = true
	}

	if steps > 0 {
		if err := wp.SetSteps(steps); err != nil {
			return err
		}
		ed = true
	}
	if minSteps != wp.MinSteps() {
		if err := wp.SetMinSteps(minSteps); err != nil {
			return err
		}
		ed = true
	}
	if p.Path(project.WalkParam) != wp.Name() {
		if err := wp.Write(); err != nil {
			return err
		}
		p.Add(project.WalkParam, wp.Name())
		if err := p.Write(); err != nil {
			return err
		}
		return nil
	}
	if ed {
		if err := wp.Write(); err != nil {
			return err
		}
		return nil
	}

	printParams(c.Stdout(), wp)
	return nil
}

func printParams(w io.Writer, wp *walkparam.WP) {
	fmt.Fprintf(w, "file:         %s\n", wp.Name())
	fmt.Fprintf(w, "steps per my: %d\n", wp.Steps())
	if m := wp.MinSteps(); m > 0 {
		fmt.Fprintf(w, "min steps:   %d\n", m)
	}
	if c := wp.Cats(); c > 1 {
		fmt.Fprintf(w, "relaxed:     %s\n", wp.Function())
		fmt.Fprintf(w, "categories:  %d\n", c)
	}
}
