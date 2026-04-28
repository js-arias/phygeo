// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package lambda implements a command to approximate
// the number of steps in a random walk
// to the lambda value of the spherical normal.
package lambda

import (
	"fmt"
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
	Short: "report settlement probabilities from lambda values",
	Long: `
Command lambda writes the settlement probability that approximates the
indicated lambda parameter of an spherical normal using the pixelation defined
in a PhyGeo project.

The first argument of the command is the name of the project file.

The second argument is optional, it is the value of lambda of the diffusion
process over a million years using 1/radian^2 units. If not defined, the
lambda value will be taken from the model definition.

The flag --model is required, and is used to set the name of the model
definition. The model is used to define the parameters of the random walk.

The output indicates the settlement value, as well as the expected value
(in kilometers per million years), and the variance (km^2 per million years).
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

	pix := landscape.Pixelation()
	net := earth.NewNetwork(pix)

	mp, err := openModel(modelFile)
	if err != nil {
		return err
	}
	lambda := mp.Lambda()

	if len(args) >= 2 {
		lambda, err = strconv.ParseFloat(args[1], 64)
		if err != nil {
			return fmt.Errorf("lambda value: %v", err)
		}
	}
	sett := walker.Settlement(pix, net, lambda, mp.Steps())
	E, V := walker.Expected(pix, net, sett, mp.Steps())
	E *= earth.Radius / 1000
	V *= earth.Radius / 1000

	fmt.Printf("steps\tlambda\tE(x)\tvar(x)\tsettlement\n")
	fmt.Printf("%d\t%.6f\t%.3f\t%.3f\t%.6f\n", int(mp.Steps()), lambda, E, V, sett)
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
