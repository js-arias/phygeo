// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package like implements a command to perform
// an approximate biogeographic reconstruction with likelihood
// using random walks.
package like

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/phygeo/cats"
	"github.com/js-arias/phygeo/infer/walk"
	"github.com/js-arias/phygeo/project"
	"gonum.org/v1/gonum/stat/distuv"
)

var Command = &command.Command{
	Usage: `like [--stem <age>]
	[--steps <value>] [--min <number>] [--max <number>]
	[--walkers <value>]
	[--relaxed <value>] [--cats <number>]
	[-o|--output <file>]
	[--cpu <number>]
	<project-file>`,
	Short: "perform an approximate likelihood reconstruction",
	Long: `
Command like reads a PhyGeo project and performs an approximate likelihood
reconstruction for the trees in the project using a random walk.

The argument of the command is the name of the project file.

By default, the inference of the root will use the pixel settlement weights at
the root as pixel priors. Use the flag --stem, with a value in million of
years, to add a "root branch" with the indicated length. In that case the root
pixels will be closer to the expected equilibrium of the model, at the cost of
increasing computing time.

The flag --steps define the number of steps per million years in the random
walk. The default value is 10. It can be a non integer value. Flags --min and
--max define the minimum and maximum number of steps in a branch-category.
Defaults are 3 and 1000.

By default, there will be 100 walkers (particles) per each category. Use the
flag --walkers to set a different number.

By default, a relaxed random walk using a logNormal with mean 1 and sigma 1.5,
and ten categories. To change the number of categories use the parameter
--cats. To change the relaxed distribution, use the parameter --relaxed with
a distribution function. The format for the relaxed distribution function is

	"<distribution>=<param>[,<param>]"

Always use the quotations. The implemented distributions are:

	- Gamma: with a single parameter (both alpha and beta set as equal).
	- LogNormal: with a single parameter (sigma), the mean is 1.

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

var numSteps float64
var stemAge float64
var numCats int
var minSteps int
var maxSteps int
var walkers int
var numCPU int
var relaxed string
var output string

func setFlags(c *command.Command) {
	c.Flags().Float64Var(&stemAge, "stem", 0, "")
	c.Flags().Float64Var(&numSteps, "steps", 10, "")
	c.Flags().IntVar(&minSteps, "min", 3, "")
	c.Flags().IntVar(&maxSteps, "max", 1000, "")
	c.Flags().IntVar(&numCats, "cats", 10, "")
	c.Flags().IntVar(&walkers, "walkers", 100, "")
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

	net := earth.NewNetwork(landscape.Pixelation())

	var dd cats.Discrete
	if relaxed == "" {
		dd = cats.LogNormal{
			Param: distuv.LogNormal{
				Mu:    0,
				Sigma: 1.5,
			},
			NumCat: numCats,
		}
	} else {
		dd, err = cats.Parse(relaxed, numCats)
		if err != nil {
			return fmt.Errorf("flag --relaxed: %v", err)
		}
	}

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
		Steps:      numSteps,
		MinSteps:   minSteps,
		MaxSteps:   maxSteps,
		Walkers:    walkers,
		Discrete:   dd,
	}

	walk.Start(numCPU)
	for _, tn := range tc.Names() {
		t := tc.Tree(tn)
		param.Stem = int64(stemAge * 1_000_000)
		wt := walk.New(t, param)
		l := wt.DownPass()
		name := fmt.Sprintf("%s-down-%s.tab", output, t.Name())
		if err := writeTreeConditional(wt, name, p.Name()); err != nil {
			return err
		}
		fmt.Fprintf(c.Stdout(), "%s\t%.6f\n", tn, l)
	}
	walk.End()
	return nil
}

func writeTreeConditional(t *walk.Tree, name, p string) (err error) {
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
	fmt.Fprintf(w, "# base steps per million year: %.6f\n", t.Steps())
	fmt.Fprintf(w, "# walkers per rate category: %d\n", walkers)
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
		"steps",
		"relaxed",
		"cats",
		"trait",
		"equator",
		"pixel",
		"value",
	}
	if err := tsv.Write(header); err != nil {
		return err
	}
	steps := strconv.FormatFloat(t.Steps(), 'f', 6, 64)
	relaxed := t.Discrete().String()
	numberCats := strconv.Itoa(t.NumCats())
	eq := strconv.Itoa(t.Equator())

	nodes := t.Nodes()
	for _, n := range nodes {
		nID := strconv.Itoa(n)
		stages := t.Stages(n)
		for _, a := range stages {
			stageAge := strconv.FormatInt(a, 10)
			traits := t.Traits()
			for _, tr := range traits {
				c := t.Conditional(n, a, tr)
				for px := range t.Pixels() {
					lk, ok := c[px]
					if !ok {
						continue
					}
					row := []string{
						t.Name(),
						nID,
						stageAge,
						"log-like",
						steps,
						relaxed,
						numberCats,
						tr,
						eq,
						strconv.Itoa(px),
						strconv.FormatFloat(lk, 'f', 8, 64),
					}
					if err := tsv.Write(row); err != nil {
						return err
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
