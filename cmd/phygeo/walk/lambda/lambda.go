// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package lambda implements a command to approximate
// the number of steps in a random walk
// to the lambda value of the spherical normal.
package lambda

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/phygeo/infer/model"
	"github.com/js-arias/phygeo/infer/walker"
	"github.com/js-arias/phygeo/project"
)

var Command = &command.Command{
	Usage: `lambda --model <model-file>
	<project> [<value>]`,
	Short: "report the lambda equivalent value",
	Long: `
Command lambda writes the approximate lambda for an spherical normal in an
homogeneous landscape for a given roaming value.

The first argument of the command is the name of the project file.

The second argument is optional, it is the value of roaming value (i.e., the
probability to move out of the pixel per each step. If not defined, the value
will be taken from the model definition.

The flag --model is required, and is used to set the name of the model
definition. The model is used to define the parameters of the random walk.

The output indicates the lambda value, as well as the expected value (in
kilometers per million years), and the variance (km^2 per million years).
`,
	SetFlags: setFlags,
	Run:      run,
}

var modelFile string

func setFlags(c *command.Command) {
	c.Flags().StringVar(&modelFile, "model", "", "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}
	if modelFile == "" {
		return c.UsageError("--model flag should be defined")
	}

	p, err := project.Read(args[0])
	if err != nil {
		return err
	}

	landscape, err := p.Landscape(nil)
	if err != nil {
		return err
	}

	tr, err := p.Traits()
	if err != nil {
		return err
	}

	pix := landscape.Pixelation()
	net := earth.NewNetwork(pix)

	mp, err := openModel(modelFile)
	if err != nil {
		return err
	}

	fmt.Fprintf(c.Stdout(), "state\tsteps\tlambda\tE(x)\tvar(x)\troaming\n")
	if len(args) >= 2 {
		for _, a := range args[1:] {
			roaming, err := strconv.ParseFloat(a, 64)
			if err != nil {
				return fmt.Errorf("roaming value: %v", err)
			}
			printValues(c.Stdout(), pix, net, roaming, mp.Steps(), "user")
		}
		return nil
	}

	for _, s := range tr.States() {
		roaming := mp.Roaming(s)
		printValues(c.Stdout(), pix, net, roaming, mp.Steps(), s)
	}
	return nil
}

func openModel(name string) (*model.Model, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	mp, err := model.Read(f)
	if err != nil {
		return nil, fmt.Errorf("on file %q: %v", name, err)
	}
	return mp, nil
}

func printValues(w io.Writer, pix *earth.Pixelation, net earth.Network, roaming float64, steps int, state string) {
	lambda := walker.Lambda(pix, net, roaming, steps)
	E, V := walker.Expected(pix, net, roaming, steps)
	E *= earth.Radius / 1000
	V *= earth.Radius / 1000
	fmt.Fprintf(w, "%s\t%d\t%.3f\t%.3f\t%.3f\t%.6f\n", state, steps, lambda, E, V, roaming)
}
