// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package like implements a command to perform
// a biogeographic reconstruction using likelihood.
package like

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/stat/dist"
	"github.com/js-arias/phygeo/infer/diffusion"
	"github.com/js-arias/phygeo/project"
)

var Command = &command.Command{
	Usage: `like [--stem <age>] [--lambda <value>]
	[-o|--output <file>]
	[--cpu <number>] <project-file>`,
	Short: "perform a likelihood reconstruction",
	Long: `
Command like reads a PhyGeo project and performs a likelihood reconstruction
for the trees in the project.

The argument of the command is the name of the project file.

By default, the inference of the root will use the pixel weights at the root
time stage as pixel priors. Use the flag --stem, with a value in million
years, to add a "root branch" with the indicated length. In that case the root
pixels will be closer to the expected equilibrium of the model, at the cost of
increasing computing time.

The flag --lambda defines the concentration parameter of the spherical normal
(equivalent to the kappa parameter of the von Mises-Fisher distribution) for a
diffusion process over a million years using 1/radias^2 units. If no value is
defined, it will use 100. As the kappa parameter, larger values indicate low
diffusivity, while smaller values indicate high diffusivity.

The output file is a pixel probability file with the conditional likelihoods
(i.e., down-pass results) for each pixel at each node. The prefix of the
output file name is the name of the project file. To set a different prefix,
use the flag --output, or -o. The output file name will be named by the tree
name, the lambda value, and the suffix 'down'.

By default, all available CPUs will be used in the calculations. Set the flag
--cpu to use a different number of CPUs.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var lambdaFlag float64
var stemAge float64
var numCPU int
var output string

func setFlags(c *command.Command) {
	c.Flags().Float64Var(&lambdaFlag, "lambda", 100, "")
	c.Flags().Float64Var(&stemAge, "stem", 0, "")
	c.Flags().IntVar(&numCPU, "cpu", runtime.GOMAXPROCS(0), "")
	c.Flags().StringVar(&output, "output", "", "")
	c.Flags().StringVar(&output, "o", "", "")
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

	rc, err := p.Ranges(landscape.Pixelation())
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

	dm, _ := earth.NewDistMatRingScale(landscape.Pixelation())

	standard := calcStandardDeviation(landscape.Pixelation(), lambdaFlag)

	param := diffusion.Param{
		Landscape: landscape,
		Rot:       rot,
		DM:        dm,
		PW:        pw,
		Ranges:    rc,
		Lambda:    lambdaFlag,
		Stages:    stages.Stages(),
	}

	// Set the number of parallel processors
	diffusion.SetCPU(numCPU)

	for _, tn := range tc.Names() {
		t := tc.Tree(tn)
		param.Stem = int64(stemAge * 1_000_000)
		name := fmt.Sprintf("%s-%s-%.6f-down.tab", args[0], t.Name(), lambdaFlag)
		if output != "" {
			name = output + "-" + name
		}

		dt := diffusion.New(t, param)
		dt.DownPass()
		if err := writeTreeConditional(dt, name, args[0], lambdaFlag, standard, landscape.Pixelation().Len(), landscape.Pixelation().Equator()); err != nil {
			return err
		}
		fmt.Fprintf(c.Stdout(), "%s\t%.6f\n", tn, dt.LogLike())
	}
	return nil
}

// CalcStandardDeviation returns the standard deviation
// (i.e. the square root of variance)
// in km per million year.
func calcStandardDeviation(pix *earth.Pixelation, lambda float64) float64 {
	n := dist.NewNormal(lambda, pix)
	v := n.Variance()
	return math.Sqrt(v) * earth.Radius / 1000
}

func writeTreeConditional(t *diffusion.Tree, name, p string, lambda, standard float64, numPix, eq int) (err error) {
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	defer func() {
		e := f.Close()
		if err == nil && e != nil {
			err = e
		}
	}()

	w := bufio.NewWriter(f)
	fmt.Fprintf(w, "# diff.like on tree %q of project %q\n", t.Name(), p)
	fmt.Fprintf(w, "# lambda: %.6f * 1/radian^2\n", lambda)
	fmt.Fprintf(w, "# standard deviation: %.6f * Km/My\n", standard)
	fmt.Fprintf(w, "# logLikelihood: %.6f\n", t.LogLike())
	fmt.Fprintf(w, "# date: %s\n", time.Now().Format(time.RFC3339))

	tsv := csv.NewWriter(w)
	tsv.Comma = '\t'
	tsv.UseCRLF = true
	if err := tsv.Write([]string{"tree", "node", "age", "type", "lambda", "equator", "pixel", "value"}); err != nil {
		return err
	}

	nodes := t.Nodes()
	for _, n := range nodes {
		stages := t.Stages(n)
		for _, a := range stages {
			c := t.Conditional(n, a)
			for px := 0; px < numPix; px++ {
				lk, ok := c[px]
				if !ok {
					continue
				}
				row := []string{
					t.Name(),
					strconv.Itoa(n),
					strconv.FormatInt(a, 10),
					"log-like",
					strconv.FormatFloat(lambda, 'f', 6, 64),
					strconv.Itoa(eq),
					strconv.Itoa(px),
					strconv.FormatFloat(lk, 'f', 8, 64),
				}
				if err := tsv.Write(row); err != nil {
					return err
				}
			}
		}
	}

	tsv.Flush()
	if err := tsv.Error(); err != nil {
		return fmt.Errorf("while writing data on %q: %v", name, err)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("while writing data on %q: %v", name, err)
	}
	return nil
}
