// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package like implements a command to perform
// a biogeographic reconstruction with likelihood
// using random walks.
package like

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/phygeo/cats"
	"github.com/js-arias/phygeo/infer/catwalk"
	"github.com/js-arias/phygeo/infer/walk"
	"github.com/js-arias/phygeo/project"
)

var Command = &command.Command{
	Usage: `like [--stem <age>]
	[--lambda <value>]
	[--relaxed <value>]
	[-o|--output <file>]
	[--cpu <number>]
	<project-file>`,
	Short: "perform a likelihood reconstruction",
	Long: `
Command like reads a PhyGeo project and performs a likelihood reconstruction
for the trees in the project using a random walk.

The argument of the command is the name of the project file.

By default, the inference of the root will use the pixel settlement weights at
the root as pixel priors. Use the flag --stem, with a value in million of
years, to add a "root branch" with the indicated length. In that case the root
pixels will be closer to the expected equilibrium of the model, at the cost of
increasing computing time.

The flag --lambda defines the concentration parameter of the spherical normal
(equivalent to the kappa parameter of the von Mises-Fisher distribution)
resulting from running the random walk over a million years. It uses
1/radian^2 units. If no value is defined, it will use 100. As the kappa
parameter, larger values indicate low diffusivity, while smaller values
indicate high diffusivity.

By default, if a relaxed random walk is used, it will use the function defined
in the random walk parameters file, with the default parameters for the function.
To set the parameter of that distribution, use the flag --relaxed with the
parameter(s) of the function. The format is

	"<param>[,<param>]"

Always use the quotations if more than one parameter is defined.

The output file is a pixel probability file with the conditional likelihoods
(i.e., down-pass results) for each pixel at each node. The prefix of the
output file name is the name of the project file. To set a different prefix,
use the flag --output, or -o. The output file name will have the output
prefix, the word 'down', and the tree name. The extension will be '.tab'.

By default, all available CPU will be used in the calculations. Set the flag
--cpu to use a different number of CPUs.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var lambdaFlag float64
var stemAge float64
var numCPU int
var relaxed string
var output string

func setFlags(c *command.Command) {
	c.Flags().Float64Var(&lambdaFlag, "lambda", 100, "")
	c.Flags().Float64Var(&stemAge, "stem", 0, "")
	c.Flags().IntVar(&numCPU, "cpu", 0, "")
	c.Flags().StringVar(&relaxed, "relaxed", "", "")
	c.Flags().StringVar(&output, "output", "", "")
	c.Flags().StringVar(&output, "o", "", "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		c.UsageError("expecting project file")
	}

	p, err := project.Read(args[0])
	if err != nil {
		return err
	}
	if output == "" {
		output = p.NameRoot()
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

	rc, err := p.Ranges(landscape.Pixelation())
	if err != nil {
		return err
	}

	tr, err := p.Traits()
	if err != nil {
		return err
	}

	// check if all terminals have defined ranges
	// and traits
	for _, tn := range tc.Names() {
		t := tc.Tree(tn)
		for _, term := range t.Terms() {
			if !rc.HasTaxon(term) {
				return fmt.Errorf("taxon %q of tree %q has no defined range", term, tn)
			}
			if len(tr.Obs(term)) == 0 {
				return fmt.Errorf("taxon %q of tree %q has no defined trait", term, tn)
			}
		}
	}

	keys, err := p.Keys()
	if err != nil {
		return err
	}

	mv, err := p.Movement(tr, keys)
	if err != nil {
		return err
	}

	st, err := p.Settlement(tr, keys)
	if err != nil {
		return err
	}

	wp, err := p.WalkParam(landscape.Pixelation())
	if err != nil {
		return err
	}
	params, err := parseParams()
	if err != nil {
		return err
	}

	net := earth.NewNetwork(landscape.Pixelation())

	dd := wp.Relaxed(params)
	settCats := catwalk.Cats(landscape.Pixelation(), net, lambdaFlag, wp.Steps(), dd)

	param := walk.Param{
		Landscape:  landscape,
		Rot:        rot,
		Stages:     stages.Stages(),
		Net:        net,
		Ranges:     rc,
		Traits:     tr,
		Keys:       keys,
		Movement:   mv,
		Settlement: st,
		Lambda:     lambdaFlag,
		Steps:      wp.Steps(),
		MinSteps:   wp.MinSteps(),
		Discrete:   settCats,
	}

	walk.StartDown(numCPU, landscape.Pixelation(), len(tr.States()))
	for _, tn := range tc.Names() {
		t := tc.Tree(tn)
		param.Stem = int64(stemAge * 1_000_000)
		wt := walk.New(t, param)
		l := wt.DownPass()
		fmt.Fprintf(c.Stdout(), "%s\t%.6f\n", tn, l)
		if math.IsInf(l, -1) {
			continue
		}

		name := fmt.Sprintf("%s-down-%s.tab", output, t.Name())
		if err := writeTreeConditional(wt, name, p.Name(), dd); err != nil {
			return err
		}
	}
	walk.EndDown()
	return nil
}

func writeTreeConditional(t *walk.Tree, name, p string, dd cats.Discrete) (err error) {
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
	fmt.Fprintf(w, "# conditional likelihoods of tree %q of project %q\n", t.Name(), p)
	fmt.Fprintf(w, "# lambda: %.6f * 1/radian^2\n", lambdaFlag)
	fmt.Fprintf(w, "# relaxed diffusion function: %s with %d categories\n", dd, len(dd.Cats()))
	fmt.Fprintf(w, "# steps per million year: %d\n", t.Steps())
	fmt.Fprintf(w, "# logLikelihood: %.6f\n", t.LogLike())
	fmt.Fprintf(w, "# date: %s\n", time.Now().Format(time.RFC3339))

	tsv := csv.NewWriter(w)
	tsv.Comma = '\t'
	tsv.UseCRLF = true
	header := []string{
		"tree",
		"node",
		"age",
		"type",
		"lambda",
		"steps",
		"relaxed",
		"cats",
		"cat",
		"scaled",
		"trait",
		"equator",
		"pixel",
		"value",
	}
	if err := tsv.Write(header); err != nil {
		return err
	}

	cats := t.Cats()
	numberCats := strconv.Itoa(len(cats))
	eq := strconv.Itoa(t.Equator())
	lambdaVal := strconv.FormatFloat(lambdaFlag, 'f', 6, 64)

	nodes := t.Nodes()
	for _, n := range nodes {
		nID := strconv.Itoa(n)
		stages := t.Stages(n)
		for _, a := range stages {
			stageAge := strconv.FormatInt(a, 10)
			steps := strconv.Itoa(t.StageSteps(n, a))
			for i, c := range cats {
				traits := t.Traits()
				currCat := strconv.Itoa(i + 1)
				scaled := strconv.FormatFloat(lambdaFlag*c, 'f', 6, 64)
				for _, tr := range traits {
					cond := t.Conditional(n, a, i, tr)
					for px := range t.Pixels() {
						lk, ok := cond[px]
						if !ok {
							continue
						}
						row := []string{
							t.Name(),
							nID,
							stageAge,
							"log-like",
							lambdaVal,
							steps,
							dd.String(),
							numberCats,
							currCat,
							scaled,
							tr,
							eq,
							strconv.Itoa(px),
							strconv.FormatFloat(lk, 'f', 16, 64),
						}
						if err := tsv.Write(row); err != nil {
							return err
						}
					}
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

func parseParams() ([]float64, error) {
	if relaxed == "" {
		return nil, nil
	}
	pv := strings.Split(relaxed, ",")
	p := make([]float64, 0, len(pv))
	for _, v := range pv {
		x, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("flag --relaxed: %v", err)
		}
		p = append(p, x)
	}
	return p, nil
}
