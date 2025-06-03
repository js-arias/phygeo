// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package lambda implements a command to approximate
// the number of steps in a random walk
// to the lambda value of the spherical normal.
package lambda

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/stat/dist"
	"github.com/js-arias/phygeo/project"
)

var Command = &command.Command{
	Usage: `lambda [--particles <int>] [--steps]
	<project> <value>`,
	Short: "approximate the value of lambda",
	Long: `
Command lambda uses a given number of steps to estimate an approximate value
of the lambda parameter of an spherical normal using the pixelation defined in
a PhyGeo project.

The first argument of the command is the name of the project file.

The second argument of the command is the number of steps. If the flag --steps
is defined, it reads the second argument as a lambda value, and the command
will print the number of steps in which the maximum likelihood lambda is
closer to the indicated value.

By default, the simulation will use 1000 particles. Use the flag --particles
to define a different number.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var stepsFlag bool
var particles int

func setFlags(c *command.Command) {
	c.Flags().BoolVar(&stepsFlag, "steps", false, "")
	c.Flags().IntVar(&particles, "particles", 1000, "")
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
		return c.UsageError("expecting numerical value")
	}

	if stepsFlag {
		lambda, err := strconv.ParseFloat(args[1], 64)
		if err != nil {
			return fmt.Errorf("invalid value argument %q: %v", args[1], err)
		}
		if lambda < 0 {
			return fmt.Errorf("invalid value argument %q: value must be greater than 0", args[1])
		}
		steps := findStep(pix, net, lambda)
		fmt.Fprintf(c.Stdout(), "steps ≈ %d\n", steps)
		return nil
	}
	steps, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("invalid value argument %q: %v", args[1], err)
	}
	if steps < 0 {
		return fmt.Errorf("invalid value argument %q: value must be greater than 0", args[1])
	}

	d := walk(pix, net, steps)
	best, bestLike := findMax(pix, d)
	fmt.Fprintf(c.Stdout(), "lambda ≈ %.6f (likelihood ≈ %.6f)\n", best, bestLike)

	return nil
}

func walk(pix *earth.Pixelation, net earth.Network, steps int) []int {
	d := make([]int, particles)
	for i := range d {
		px := 0
		for range steps {
			n := net[px]
			nx := rand.IntN(len(n))
			px = n[nx]
		}
		d[i] = pix.ID(px).Ring()
	}
	return d
}

func findMax(pix *earth.Pixelation, d []int) (best, bestLike float64) {
	// min boundary
	min := 0.001
	minLike := likelihood(pix, min, d)
	best = min
	bestLike = minLike

	// max boundary
	max := 100.0
	maxLike := likelihood(pix, max, d)
	if maxLike > bestLike {
		best = max
		bestLike = maxLike
	}

	// go up
	for {
		l := max * math.Phi
		like := likelihood(pix, l, d)
		max = l
		maxLike = like
		if maxLike < bestLike {
			break
		}
		best = max
		bestLike = maxLike
	}

	// golden ratio search
	ratio := 1 / math.Phi
	for {
		if (max - min) < 0.01 {
			break
		}
		d1 := best - min
		d2 := max - best
		if d1 > d2 {
			l := min + d1*(1-ratio)
			like := likelihood(pix, l, d)
			if like > bestLike {
				max = best
				maxLike = bestLike
				bestLike = like
				best = l
				continue
			}
			minLike = like
			min = l
			continue
		}
		l := best + d2*ratio
		like := likelihood(pix, l, d)
		if like > bestLike {
			min = best
			minLike = bestLike
			bestLike = like
			best = l
			continue
		}
		maxLike = like
		max = l
	}

	return best, bestLike
}

func likelihood(pix *earth.Pixelation, l float64, d []int) float64 {
	n := dist.NewNormal(l, pix)
	var sum float64
	for _, r := range d {
		sum += n.LogProbRingDist(r)
	}
	return sum
}

func findStep(pix *earth.Pixelation, net earth.Network, lambda float64) int {
	best := 3
	diff := math.MaxFloat64
	for s := 3; s <= 1000; s++ {
		d := walk(pix, net, s)
		l, _ := findMax(pix, d)
		x := math.Abs(l - lambda)
		if x < diff {
			best = s
			diff = x
		}
	}
	return best
}
