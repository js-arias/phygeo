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
	"time"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/phygeo/infer/model"
	"github.com/js-arias/phygeo/infer/walk"
	"github.com/js-arias/phygeo/infer/walker"
	"github.com/js-arias/phygeo/project"
)

var Command = &command.Command{
	Usage: `like
	[-o|--output <file>]
	[--cpu <number>]
	--model <model-file>
	<project-file>`,
	Short: "perform a likelihood reconstruction",
	Long: `
Command like reads a PhyGeo project and a model definition and performs a
likelihood reconstruction for the trees in the project using a random walk.

The argument of the command is the name of the project file.

The flag --model is required, and it is used to read the model parameter
values. Any undefined value will be set as zero.

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

var numCPU int
var output string
var modelFile string

func setFlags(c *command.Command) {
	c.Flags().IntVar(&numCPU, "cpu", 0, "")
	c.Flags().StringVar(&modelFile, "model", "", "")
	c.Flags().StringVar(&output, "output", "", "")
	c.Flags().StringVar(&output, "o", "", "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		c.UsageError("expecting project file")
	}
	if modelFile == "" {
		return c.UsageError("--model flag should be defined")
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
	rot.SetUndefAsFix()

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

	mp, err := openModel(modelFile)
	if err != nil {
		return err
	}

	net := earth.NewNetwork(landscape.Pixelation())

	mv := mp.Movement(tr, keys)
	st := mp.Settlement(tr, keys)
	states := tr.States()
	landProb := make([]walker.Model, len(states))
	for i, c := range states {
		roaming := mp.Roaming(c)
		lp := walker.New(landscape, net, mv, st, roaming, c, i, keys)
		landProb[i] = lp
	}

	param := walk.Param{
		Landscape: landscape,
		Rot:       rot,
		Stages:    stages.Stages(),
		Ranges:    rc,
		Traits:    tr,
		Keys:      keys,
		Walker:    landProb,
		Stem:      mp.StemAge(),
		Steps:     mp.Steps(),
	}

	walk.StartDown(numCPU, landscape.Pixelation(), len(tr.States()))
	for _, tn := range tc.Names() {
		t := tc.Tree(tn)
		wt := walk.New(t, param)
		l := wt.DownPass()
		fmt.Fprintf(c.Stdout(), "%s\t%.6f\n", tn, l)
		if math.IsInf(l, -1) {
			continue
		}

		name := fmt.Sprintf("%s-down-%s.tab", output, t.Name())
		if err := writeTreeConditional(wt, name, p.Name(), mp); err != nil {
			return err
		}
	}
	walk.EndDown()
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

func writeTreeConditional(t *walk.Tree, name, p string, mp *model.Model) (err error) {
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
	mp.WriteAsComment(w)
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
		"state",
		"equator",
		"pixel",
		"value",
	}
	if err := tsv.Write(header); err != nil {
		return err
	}

	eq := strconv.Itoa(t.Equator())

	nodes := t.Nodes()
	for _, n := range nodes {
		nID := strconv.Itoa(n)
		stages := t.Stages(n)
		for _, a := range stages {
			stageAge := strconv.FormatInt(a, 10)
			states := t.States()
			for _, tr := range states {
				cond := t.Conditional(n, a, tr)
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

	tsv.Flush()
	if err := tsv.Error(); err != nil {
		return fmt.Errorf("while writing data on %q: %v", name, err)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("while writing data on %q: %v", name, err)
	}
	return nil
}
