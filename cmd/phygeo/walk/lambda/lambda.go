// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package lambda implements a command to approximate
// the number of steps in a random walk
// to the lambda value of the spherical normal.
package lambda

import (
	"fmt"
	"strconv"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/phygeo/cats"
	"github.com/js-arias/phygeo/infer/catwalk"
	"github.com/js-arias/phygeo/project"
	"gonum.org/v1/gonum/stat/distuv"
)

var Command = &command.Command{
	Usage: `lambda [--steps <number>]
	[--relaxed <value>] [--cats <number>]
	<project> <value>`,
	Short: "report settlement probabilities from lambda values",
	Long: `
Command lambda writes the settlement probability that approximates the
indicated lambda parameter of an spherical normal using the pixelation defined
in a PhyGeo project.

The first argument of the command is the name of the project file.

The second argument of the command is the value of lambda of the diffusion
process over a million years using 1/radian^2 units.

The flag --steps define the number of steps per million years in the random
walk. The default value is the number of pixels at the equator.

By default, a relaxed random walk using a logNormal with mean 1 and sigma 1.0,
and nine categories. To change the number of categories use the parameter
--cats. To change the relaxed distribution, use the parameter --relaxed with
a distribution function. The format for the relaxed distribution function is

	"<distribution>=<param>[,<param>]"

Always use the quotations. The implemented distributions are:

	- Gamma: with a single parameter (both alpha and beta set as equal).
	- LogNormal: with a single parameter (sigma), the mean is 1.
`,
	SetFlags: setFlags,
	Run:      run,
}

var numCats int
var numSteps int
var relaxed string

func setFlags(c *command.Command) {
	c.Flags().IntVar(&numSteps, "steps", 0, "")
	c.Flags().IntVar(&numCats, "cats", 9, "")
	c.Flags().StringVar(&relaxed, "relaxed", "", "")
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

	pix := landscape.Pixelation()
	net := earth.NewNetwork(pix)

	if len(args) < 2 {
		return c.UsageError("expecting lambda value (numerical)")
	}
	lambda, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		return fmt.Errorf("expecting lambda value: %v", err)
	}

	var dd cats.Discrete
	if relaxed == "" {
		dd = cats.LogNormal{
			Param: distuv.LogNormal{
				Mu:    0,
				Sigma: 1.0,
			},
			NumCat: numCats,
		}
	} else {
		dd, err = cats.Parse(relaxed, numCats)
		if err != nil {
			return fmt.Errorf("flag --relaxed: %v", err)
		}
	}
	if numSteps == 0 {
		numSteps = landscape.Pixelation().Equator()
	}
	cats := dd.Cats()
	settCats := catwalk.Cats(landscape.Pixelation(), net, lambda, numSteps, dd)

	fmt.Printf("steps\tlambda\tcat\tscalar\tscaled\tsettlement\n")
	for i, s := range settCats {
		cv := cats[i]
		fmt.Fprintf(c.Stdout(), "%d\t%.6f\t%d\t%.6f\t%.6f\t%.6f\n", numSteps, lambda, i+1, cv, lambda*cv, s)
	}

	return nil
}
