// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package ml implements a command to search
// for the maximum likelihood estimation
// of a biogeographic reconstruction.
package ml

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"strconv"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/stat/pixprob"
	"github.com/js-arias/phygeo/infer/diffusion"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/ranges"
	"github.com/js-arias/timetree"
	"golang.org/x/exp/slices"
)

var Command = &command.Command{
	Usage: `ml [--ranges] [--stem <age>]
	[--lambda <value>] [--step <value>] [--stop <value>]
	[-p|--particles <number>]
	[-o|--output <file>]
	[--cpu <number>] [--nomat] <project-file>`,
	Short: "search the maximum likelihood estimate",
	Long: `
Command ml reads a PhyGeo project, and search for the maximum likelihood
estimation of the lambda parameter.

The algorithm is a simple hill climbing search. By default it starts at a
lambda value of zero. The flag --lambda changes this starting point. By
default, the initial step has a value of 100, use the flag --step to change
the value. At each cycle the step value is reduced a 50%, and stop when step
has a size of 1. Use flag --stop to set a different stop value.

By default, it will use geographic distributions stored as points (presence-
absence maps). If no there are no point distribution, or the flags --ranges is
defined, the continuous range maps will be used.

By default, an stem branch will be added to each tree using the 10% of the root
age. To set a different stem age use the flag --stem, the value should be in
million years.

By default, only performs the search for the lambda value. Use the flag
--particles, or -p, to set the number of particles using a stochastic mapping
to retrieve also the estimation of the ancestral pixels. In this case the
results will be writing on a TSV file using the project name, the lambda
value, and the number of particles. If the flag -o, or --output is defined,
the indicated string will be used as a prefix for the file.

By default, all available CPUs will be used in the processing. Set --cpu flag
to use a different number of CPUs.

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
var stepFlag float64
var stopFlag float64
var numCPU int
var particles int
var output string
var useRanges bool
var noDMatrix bool

func setFlags(c *command.Command) {
	c.Flags().Float64Var(&lambdaFlag, "lambda", 0, "")
	c.Flags().Float64Var(&stopFlag, "stop", 1, "")
	c.Flags().Float64Var(&stepFlag, "step", 100, "")
	c.Flags().Float64Var(&stemAge, "stem", 0, "")
	c.Flags().IntVar(&numCPU, "cpu", runtime.NumCPU(), "")
	c.Flags().IntVar(&particles, "particles", 0, "")
	c.Flags().IntVar(&particles, "p", 0, "")
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

	// Set the number of parallel processors
	diffusion.SetCPU(numCPU)

	var dm *earth.DistMat
	if !noDMatrix {
		dm, _ = earth.NewDistMat(landscape.Pixelation())
	}

	param := diffusion.Param{
		Landscape: landscape,
		Rot:       rot,
		DM:        dm,
		PP:        pp,
		Ranges:    rc,
	}

	fmt.Fprintf(c.Stdout(), "tree\tlambda\tlogLike\tstep\n")
	for _, tn := range tc.Names() {
		t := tc.Tree(tn)
		stem := int64(stemAge * 1_000_000)
		if stem == 0 {
			stem = t.Age(t.Root()) / 10
		}
		param.Stem = stem

		b := &bestRec{
			lambda:  lambdaFlag,
			logLike: -math.MaxFloat64,
		}
		if lambdaFlag > 0 {
			param.Lambda = lambdaFlag
			df := diffusion.New(t, param)
			like := df.LogLike()
			b.logLike = like
			b.t = df
		}
		b.first(c.Stdout(), t, param, stepFlag)
		for step := stepFlag / 2; step > stopFlag; step = step / 2 {
			b.search(c.Stdout(), t, param, step)
		}
		fmt.Fprintf(c.Stdout(), "# %s\t%.6f\t%.6f\t<--- best value\n", tn, b.lambda, b.logLike)

		if particles < 1 {
			continue
		}
		name := fmt.Sprintf("%s-%s-%.6fx%d.tab", args[0], t.Name(), lambdaFlag, particles)
		if output != "" {
			name = output + "-" + name
		}
		if err := b.upPass(name, args[0]); err != nil {
			return err
		}
	}

	return nil
}

// BestRec stores the best reconstruction
type bestRec struct {
	lambda  float64
	logLike float64
	t       *diffusion.Tree
}

func (b *bestRec) first(w io.Writer, t *timetree.Tree, p diffusion.Param, step float64) {
	name := t.Name()

	// go up
	upOK := false
	for l := b.lambda + step; ; l += step {
		p.Lambda = l
		df := diffusion.New(t, p)
		like := df.LogLike()
		fmt.Fprintf(w, "%s\t%.6f\t%.6f\t%.6f\n", name, l, like, step)

		if like < b.logLike {
			break
		}
		b.lambda = l
		b.logLike = like
		b.t = df
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
		like := df.LogLike()
		fmt.Fprintf(w, "%s\t%.6f\t%.6f\t%.6f\n", name, l, like, step)

		if like < b.logLike {
			return
		}
		b.lambda = l
		b.logLike = like
		b.t = df
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
	like := df.LogLike()
	fmt.Fprintf(w, "%s\t%.6f\t%.6f\t%.6f\n", name, p.Lambda, like, step)
	if like > b.logLike {
		// we found an improvement
		b.lambda = p.Lambda
		b.logLike = like
		b.t = df
		return
	}

	// go down
	if b.lambda < step {
		return
	}
	p.Lambda = b.lambda - step
	df = diffusion.New(t, p)
	like = df.LogLike()
	fmt.Fprintf(w, "%s\t%.6f\t%.6f\t%.6f\n", name, p.Lambda, like, step)
	if like > b.logLike {
		// we found an improvement
		b.lambda = p.Lambda
		b.logLike = like
		b.t = df
		return
	}
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

func (b *bestRec) upPass(name, p string) (err error) {
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

	tsv, err := outHeader(w, b.t.Name(), p, b.lambda, b.logLike)
	if err != nil {
		return fmt.Errorf("while writing header on %q: %v", name, err)
	}

	for i := 0; i < particles; i++ {
		m := b.t.Simulate()
		if err := writeUpPass(tsv, i, m); err != nil {
			return fmt.Errorf("while writing data on %q: %v", name, err)
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

func outHeader(w io.Writer, t, p string, lambda, logLike float64) (*csv.Writer, error) {
	fmt.Fprintf(w, "# diff.like on tree %q of project %q\n", t, p)
	fmt.Fprintf(w, "# lambda: %.6f * 1/radian^2\n", lambda)
	fmt.Fprintf(w, "# logLikelihood: %.6f\n", logLike)
	fmt.Fprintf(w, "# up-pass particles: %d\n", particles)

	tsv := csv.NewWriter(w)
	tsv.Comma = '\t'
	tsv.UseCRLF = true
	if err := tsv.Write([]string{"tree", "particle", "node", "age", "from", "to"}); err != nil {
		return nil, err
	}

	return tsv, nil
}

func writeUpPass(tsv *csv.Writer, p int, m *diffusion.Mapping) error {
	nodes := m.Nodes()

	for _, id := range nodes {
		n := m.Node(id)
		stages := make([]int64, 0, len(n.Stages))
		for a := range n.Stages {
			stages = append(stages, a)
		}
		slices.Sort(stages)

		for _, a := range stages {
			st := n.Stages[a]
			row := []string{
				m.Name,
				strconv.Itoa(p),
				strconv.Itoa(n.ID),
				strconv.FormatInt(a, 10),
				strconv.Itoa(st.From),
				strconv.Itoa(st.To),
			}
			if err := tsv.Write(row); err != nil {
				return err
			}
		}
	}
	return nil
}
