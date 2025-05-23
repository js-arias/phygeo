// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package ml implements a command to search
// for the maximum likelihood estimation
// of a biogeographic reconstruction.
package ml

import (
	"fmt"
	"io"
	"math"
	"runtime"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/stat/dist"
	"github.com/js-arias/phygeo/infer/diffusion"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/timetree"
)

var Command = &command.Command{
	Usage: `ml [--stem <age>]
	[--lambda <value>ep <value>] [--stop <value>]
	[--cpu <number>] <project-file>`,
	Short: "search the maximum likelihood estimate",
	Long: `
Command ml reads a PhyGeo project, and search for the maximum likelihood
estimation of the lambda parameter.

The algorithm is a simple hill climbing search. By default it starts at a
lambda value of zero. The flag --lambda changes this starting point. By
default, the initial step has a value of 100, use the flag --step to change
the value. At each cycle the step value is reduced a 50%, and stop when step
has a size of 1. Use flag --stop to set a different stop value.

By default, the inference of the root will use the pixel weights at the root
time stage as pixel priors. Use the flag --stem, with a value in million
years, to add a "root branch" with the indicated length. In that case the root
pixels will be closer to the expected equilibrium of the model, at the cost of
increasing computing time.

By default, all available CPUs will be used in the processing. Set --cpu flag
to use a different number of CPUs.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var lambdaFlag float64
var stemAge float64
var stepFlag float64
var stopFlag float64
var numCPU int

func setFlags(c *command.Command) {
	c.Flags().Float64Var(&lambdaFlag, "lambda", 0, "")
	c.Flags().Float64Var(&stopFlag, "stop", 1, "")
	c.Flags().Float64Var(&stepFlag, "step", 100, "")
	c.Flags().Float64Var(&stemAge, "stem", 0, "")
	c.Flags().IntVar(&numCPU, "cpu", runtime.NumCPU(), "")
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

	landscape, err := p.Landscape(nil)
	if err != nil {
		return err
	}

	rot, err := p.StageRotation(landscape.Pixelation())
	if err != nil {
		return err
	}

	stages, err := p.Stages(rot, landscape)
	if err != nil {
		return err
	}

	pw, err := p.PixWeight()
	if err != nil {
		return err
	}

	rc, err := p.Ranges(nil)
	if err != nil {
		return err
	}
	// check if all terminals have defined ranges
	for _, tn := range tc.Names() {
		t := tc.Tree(tn)
		for _, term := range t.Terms() {
			if !rc.HasTaxon(term) {
				return fmt.Errorf("taxon %q of tree %q has no defined range", term, tn)
			}
		}
	}

	// Set the number of parallel processors
	diffusion.SetCPU(numCPU)

	dm, _ := earth.NewDistMatRingScale(landscape.Pixelation())

	param := diffusion.Param{
		Landscape: landscape,
		Rot:       rot,
		DM:        dm,
		PW:        pw,
		Ranges:    rc,
		Stages:    stages.Stages(),
	}

	fmt.Fprintf(c.Stdout(), "tree\tlambda\tstdDev\tlogLike\tstep\n")
	for _, tn := range tc.Names() {
		t := tc.Tree(tn)
		param.Stem = int64(stemAge * 1_000_000)

		b := &bestRec{
			lambda:  lambdaFlag,
			logLike: -math.MaxFloat64,
		}
		if lambdaFlag > 0 {
			param.Lambda = lambdaFlag
			df := diffusion.New(t, param)
			like := df.DownPass()
			b.logLike = like
			standard := calcStandardDeviation(param.Landscape.Pixelation(), lambdaFlag)

			fmt.Fprintf(c.Stderr(), "%s\t%.6f\t%.6f\t%.6f\t%.6f\n", tn, lambdaFlag, standard, like, stepFlag)
		}
		b.first(c.Stdout(), t, param, stepFlag)
		for step := stepFlag / 2; ; step = step / 2 {
			b.search(c.Stdout(), t, param, step)
			if step < stopFlag {
				break
			}
		}
		fmt.Fprintf(c.Stdout(), "# %s\t%.6f\t%.6f\t<--- best value\n", tn, b.lambda, b.logLike)
	}

	return nil
}

// BestRec stores the best reconstruction
type bestRec struct {
	lambda  float64
	logLike float64
}

func (b *bestRec) first(w io.Writer, t *timetree.Tree, p diffusion.Param, step float64) {
	name := t.Name()

	// go up
	upOK := false
	for l := b.lambda + step; ; l += step {
		p.Lambda = l
		df := diffusion.New(t, p)
		like := df.DownPass()
		standard := calcStandardDeviation(p.Landscape.Pixelation(), l)

		fmt.Fprintf(w, "%s\t%.6f\t%.6f\t%.6f\t%.6f\n", name, l, standard, like, stepFlag)

		if like < b.logLike {
			break
		}
		b.lambda = l
		b.logLike = like
		upOK = true
	}
	// we found an improvement
	if upOK {
		return
	}

	// go down
	for l := b.lambda - step; l > 0; l -= step {
		p.Lambda = l
		df := diffusion.New(t, p)
		like := df.DownPass()
		standard := calcStandardDeviation(p.Landscape.Pixelation(), l)

		fmt.Fprintf(w, "%s\t%.6f\t%.6f\t%.6f\t%.6f\n", name, l, standard, like, stepFlag)

		if like < b.logLike {
			return
		}
		b.lambda = l
		b.logLike = like
	}
}

// Search go one step up and one step down
// to see if the likelihood improves.
// we known that the best is in the bounds of a 2-step size
// but we know the likelihood of the bounds,
// so we only search for an step in front,
// or a step in the back.
func (b *bestRec) search(w io.Writer, t *timetree.Tree, p diffusion.Param, step float64) {
	name := t.Name()

	// go up
	p.Lambda = b.lambda + step
	df := diffusion.New(t, p)
	like := df.DownPass()
	standard := calcStandardDeviation(p.Landscape.Pixelation(), p.Lambda)

	fmt.Fprintf(w, "%s\t%.6f\t%.6f\t%.6f\t%.6f\n", name, p.Lambda, standard, like, step)
	if like > b.logLike {
		// we found an improvement
		b.lambda = p.Lambda
		b.logLike = like
		return
	}

	// go down
	if b.lambda <= step {
		return
	}
	p.Lambda = b.lambda - step
	df = diffusion.New(t, p)
	like = df.DownPass()
	standard = calcStandardDeviation(p.Landscape.Pixelation(), p.Lambda)

	fmt.Fprintf(w, "%s\t%.6f\t%.6f\t%.6f\t%.6f\n", name, p.Lambda, standard, like, step)
	if like > b.logLike {
		// we found an improvement
		b.lambda = p.Lambda
		b.logLike = like
		return
	}
}

// CalcStandardDeviation returns the standard deviation
// (i.e. the square root of variance)
// in km per million year.
func calcStandardDeviation(pix *earth.Pixelation, lambda float64) float64 {
	n := dist.NewNormal(lambda, pix)
	v := n.Variance()
	return math.Sqrt(v) * earth.Radius / 1000
}
