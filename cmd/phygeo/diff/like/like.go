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
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/stat/dist"
	"github.com/js-arias/earth/stat/pixprob"
	"github.com/js-arias/phygeo/infer/diffusion"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/ranges"
	"github.com/js-arias/timetree"
)

var Command = &command.Command{
	Usage: `like [--ranges] [--stem <age>] [--lambda <value>]
	[-o|--output <file>]
	[--cpu <number>] [--nomat] <project-file>`,
	Short: "perform a likelihood reconstruction",
	Long: `
Command like reads a PhyGeo project and performs a likelihood reconstruction
for the trees in the project.

The argument of the command is the name of the project file.

By default, it will use geographic distributions stored as points
(presence-absence maps). If there are no point distributions or the flag
--ranges is defined, the continuous range maps will be used.

By default, a stem branch will be added to each tree using 10% of the root
age. To set a different stem age, use the flag --stem; the value should be in
million years.

The flag --lambda defines the concentration parameter of the spherical normal
(equivalent to the kappa parameter of the von Mises-Fisher distribution) for a
diffusion process over a million years using 1/radias^2 units. If no value is
defined, it will use 100. As the kappa parameter, larger values indicate low
diffusivity, while smaller values indicate high diffusivity.

The output file is a TSV file with the conditional likelihoods (i.e.,
down-pass results) for each pixel at each node. The prefix of the output file
name is the name of the project file. To set a different prefix, use the flag
--output, or -o. The output file name will be named by the tree name, the
lambda value, and the suffix 'down'.

By default, all available CPUs will be used in the calculations. Set the flag
--cpu to use a different number of CPUs.

By default, if the base pixelation is smaller than 500 pixels at the equator,
it will build a distance matrix to speed up the search. As this matrix
consumes a lot of memory, this procedure can be disabled using the flag
--nomat.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var lambdaFlag float64
var stemAge float64
var numCPU int
var output string
var useRanges bool
var noDMatrix bool

func setFlags(c *command.Command) {
	c.Flags().Float64Var(&lambdaFlag, "lambda", 100, "")
	c.Flags().Float64Var(&stemAge, "stem", 0, "")
	c.Flags().IntVar(&numCPU, "cpu", runtime.GOMAXPROCS(0), "")
	c.Flags().StringVar(&output, "output", "", "")
	c.Flags().StringVar(&output, "o", "", "")
	c.Flags().BoolVar(&useRanges, "ranges", false, "")
	c.Flags().BoolVar(&noDMatrix, "nomat", false, "")
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
		msg := fmt.Sprintf("tree file not defined in project %q", args[0])
		return c.UsageError(msg)
	}
	tc, err := readTreeFile(tf)
	if err != nil {
		return err
	}

	lsf := p.Path(project.Landscape)
	if lsf == "" {
		msg := fmt.Sprintf("paleolandscape not defined in project %q", args[0])
		return c.UsageError(msg)
	}
	landscape, err := readLandscape(lsf)
	if err != nil {
		return err
	}

	rotF := p.Path(project.GeoMotion)
	if rotF == "" {
		msg := fmt.Sprintf("plate motion model not defined in project %q", args[0])
		return c.UsageError(msg)
	}
	rot, err := readRotation(rotF, landscape.Pixelation())
	if err != nil {
		return err
	}

	ppF := p.Path(project.PixPrior)
	if ppF == "" {
		msg := fmt.Sprintf("pixel priors not defined in project %q", args[0])
		return c.UsageError(msg)
	}
	pp, err := readPriors(ppF)
	if err != nil {
		return err
	}

	rf := p.Path(project.Points)
	if useRanges || rf == "" {
		rf = p.Path(project.Ranges)
	}
	rc, err := readRanges(rf)
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

	var dm *earth.DistMat
	if !noDMatrix {
		dm, _ = earth.NewDistMat(landscape.Pixelation())
	}

	standard := calcStandardDeviation(landscape.Pixelation(), lambdaFlag)

	param := diffusion.Param{
		Landscape: landscape,
		Rot:       rot,
		DM:        dm,
		PP:        pp,
		Ranges:    rc,
		Lambda:    lambdaFlag,
	}

	// Set the number of parallel processors
	diffusion.SetCPU(numCPU)

	for _, tn := range tc.Names() {
		t := tc.Tree(tn)
		stem := int64(stemAge * 1_000_000)
		if stem == 0 {
			stem = t.Age(t.Root()) / 10
		}
		param.Stem = stem
		name := fmt.Sprintf("%s-%s-%.6f-down.tab", args[0], t.Name(), lambdaFlag)
		if output != "" {
			name = output + "-" + name
		}

		dt := diffusion.New(t, param)
		dt.DownPass()
		if err := writeTreeConditional(dt, name, args[0], lambdaFlag, standard, landscape.Pixelation().Len()); err != nil {
			return err
		}

		/*
			name = fmt.Sprintf("%s-%s-%.6fx%d.tab", args[0], t.Name(), lambdaFlag, particles)
			if output != "" {
				name = output + "-" + name
			}

			if err := upPass(dt, name, args[0], lambdaFlag, standard, particles); err != nil {
				return err
			}
		*/
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

func readLandscape(name string) (*model.TimePix, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tp, err := model.ReadTimePix(f, nil)
	if err != nil {
		return nil, fmt.Errorf("on file %q: %v", name, err)
	}

	return tp, nil
}

func readRotation(name string, pix *earth.Pixelation) (*model.StageRot, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rot, err := model.ReadStageRot(f, pix)
	if err != nil {
		return nil, fmt.Errorf("on file %q: %v", name, err)
	}

	return rot, nil
}

func readPriors(name string) (pixprob.Pixel, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	pp, err := pixprob.ReadTSV(f)
	if err != nil {
		return nil, fmt.Errorf("when reading %q: %v", name, err)
	}

	return pp, nil
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

// CalcStandardDeviation returns the standard deviation
// (i.e. the square root of variance)
// in km per million year.
func calcStandardDeviation(pix *earth.Pixelation, lambda float64) float64 {
	n := dist.NewNormal(lambda, pix)
	v := n.Variance()
	return math.Sqrt(v) * earth.Radius / 1000
}

func writeTreeConditional(t *diffusion.Tree, name, p string, lambda, standard float64, numPix int) (err error) {
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
	if err := tsv.Write([]string{"tree", "node", "age", "type", "lambda", "pixel", "value"}); err != nil {
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
