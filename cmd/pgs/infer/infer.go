// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package infer implements a command to reads simulated data
// and infer the parameters using maximum likelihood.
package infer

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/stat/pixprob"
	"github.com/js-arias/phygeo/infer/diffusion"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/ranges"
	"github.com/js-arias/timetree"
)

var Command = &command.Command{
	Usage: `infer -i|--input <prefix> [-o|--output <prefix>]
	[-p|--particles <number>]
	<project-file>`,
	Short: "infer parameters from simulated data",
	Long: `
Command infer reads the results of a data simulation and performs a maximum
likelihood inference of the parameters used to generate the data.

The argument of the command is the name of a project file that contains the
paleogeographic models (plate motion model, landscape model, and the pixel
priors).

The flag --input, or -i, is required and defines the prefix of the files that
contain the results of the simulation (the tree '<prefix>-trees.tab', the
particles '<prefix>-particles.tab', and the value of lambda
'<prefix>-lambda.tab').

The flag --output, or -o, defines the prefix of the files produced in the
inference. The file '<prefix>-infer-particles.tab' produces the inferred
stochastic mapping for the lambda value estimated with maximum likelihood. The
lambda values are stored in '<prefix>-infer-lambda.tab'. If no prefix is
defined, the command will use the prefix used for the input.

By default, 1000 particles will be simulated for the stochastic mapping. The
number of particles can be changed with the flag --particles, or -p.

	`,
	SetFlags: setFlags,
	Run:      run,
}

var input string
var output string
var numParticles int

func setFlags(c *command.Command) {
	c.Flags().StringVar(&input, "input", "", "")
	c.Flags().StringVar(&input, "i", "", "")
	c.Flags().StringVar(&output, "output", "", "")
	c.Flags().StringVar(&output, "o", "", "")
	c.Flags().IntVar(&numParticles, "p", 1000, "")
	c.Flags().IntVar(&numParticles, "particles", 1000, "")
}

func run(c *command.Command, args []string) (err error) {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}
	if input == "" {
		return c.UsageError("flag --input must be defined")
	}
	if output == "" {
		output = input
	}

	p, err := project.Read(args[0])
	if err != nil {
		return err
	}

	tc, err := readTreeFile()
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

	dm, _ := earth.NewDistMatRingScale(landscape.Pixelation())

	res, err := readSimLambda(tc)
	if err != nil {
		return err
	}
	if err := readParticles(res, landscape); err != nil {
		return err
	}

	param := diffusion.Param{
		Landscape: landscape,
		Rot:       rot,
		DM:        dm,
		PP:        pp,
	}

	outName := fmt.Sprintf("%s-infer-lambda.tab", output)
	f, err := os.Create(outName)
	if err != nil {
		return err
	}
	defer func() {
		e := f.Close()
		if e != nil && err == nil {
			err = e
		}
	}()

	date := time.Now().Format(time.RFC3339)
	fmt.Fprintf(f, "# results from simulated data from project %q\n", args[0])
	fmt.Fprintf(f, "# date: %s\n", date)
	fmt.Fprintf(f, "tree\tterms\trootAge\tlambda\tml-lambda\n")

	pName := fmt.Sprintf("%s-infer-particles.tab", output)
	ff, err := os.Create(pName)
	if err != nil {
		return err
	}
	defer func() {
		e := ff.Close()
		if e != nil && err == nil {
			err = e
		}
	}()
	tsv, err := outHeader(ff, args[0], date)
	if err != nil {
		return err
	}

	for _, r := range res {
		stem := r.tree.Age(r.tree.Root()) / 10
		param.Stem = stem
		param.Ranges = r.rng

		param.Lambda = 100.0
		r.df = diffusion.New(r.tree, param)
		r.mlLambda = param.Lambda
		r.logLike = r.df.DownPass()
		r.goUp(param, 100.0)

		for step := 50.0; ; step = step / 2 {
			r.search(param, step)
			if step < 0.5 {
				break
			}
		}

		fmt.Fprintf(f, "%s\t%d\t%.3f\t%.6f\t%.6f\n", r.tree.Name(), len(r.tree.Terms()), float64(r.tree.Age(r.tree.Root()))/1_000_000, r.lambda, r.mlLambda)
		r.df.Simulate(numParticles)
		for i := 0; i < numParticles; i++ {
			if err := writeParticles(tsv, i, r.df, landscape.Pixelation().Equator()); err != nil {
				return fmt.Errorf("while writing data on %q: %v", pName, err)
			}
		}
	}
	tsv.Flush()
	if err := tsv.Error(); err != nil {
		return fmt.Errorf("while writing data on %q: %v", pName, err)
	}

	return nil
}

type simResults struct {
	tree     *timetree.Tree
	lambda   float64
	mlLambda float64
	logLike  float64
	rng      *ranges.Collection
	df       *diffusion.Tree
}

func (sr *simResults) goUp(p diffusion.Param, step float64) {
	for {
		p.Lambda = sr.mlLambda + step
		df := diffusion.New(sr.tree, p)
		like := df.DownPass()
		if like < sr.logLike {
			// we fail to improve
			return
		}

		sr.mlLambda = p.Lambda
		sr.logLike = like
		sr.df = df
	}
}

// Search go one step up and one step down
// to see if the likelihood improves.
// we known that the best is in the bounds of a 2-step size
// but we know the likelihood of the bounds,
// so we only search for an step in front,
// or a step in the back.
func (sr *simResults) search(p diffusion.Param, step float64) {
	// go up
	p.Lambda = sr.mlLambda + step
	df := diffusion.New(sr.tree, p)
	like := df.DownPass()
	if like > sr.logLike {
		// we found an improvement
		sr.mlLambda = p.Lambda
		sr.logLike = like
		sr.df = df
		return
	}

	// go down
	if sr.mlLambda <= step {
		return
	}
	p.Lambda = sr.mlLambda - step
	df = diffusion.New(sr.tree, p)
	like = df.DownPass()
	if like > sr.logLike {
		// we found an improvement
		sr.mlLambda = p.Lambda
		sr.logLike = like
		sr.df = df
		return
	}
}

func readTreeFile() (*timetree.Collection, error) {
	name := fmt.Sprintf("%s-trees.tab", input)
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

func readSimLambda(coll *timetree.Collection) (map[string]*simResults, error) {
	name := fmt.Sprintf("%s-lambda.tab", input)
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tsv := csv.NewReader(f)
	tsv.Comma = '\t'
	tsv.Comment = '#'

	header, err := tsv.Read()
	if err != nil {
		return nil, fmt.Errorf("while reading %q: %v", name, err)
	}

	cols := []string{
		"tree",
		"lambda",
	}
	fields := make(map[string]int)
	for i, h := range header {
		h = strings.ToLower(h)
		fields[h] = i
	}
	for _, c := range cols {
		if _, ok := fields[c]; !ok {
			return nil, fmt.Errorf("while reading %q: expecting column %q", name, c)
		}
	}

	r := make(map[string]*simResults)
	for {
		row, err := tsv.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		ln, _ := tsv.FieldPos(0)
		if err != nil {
			return nil, fmt.Errorf("while reading %q: line %d: %v", name, ln, err)
		}

		tn := row[fields["tree"]]
		if tn == "" {
			continue
		}
		t := coll.Tree(tn)
		if t == nil {
			continue
		}

		f := "lambda"
		l, err := strconv.ParseFloat(row[fields[f]], 64)
		if err != nil {
			return nil, fmt.Errorf("while reading %q: line %d: field %q: %v", name, ln, f, err)
		}

		r[tn] = &simResults{
			tree:     t,
			lambda:   l,
			mlLambda: 100,
		}
	}

	return r, nil
}

func readParticles(res map[string]*simResults, landscape *model.TimePix) error {
	name := fmt.Sprintf("%s-particles.tab", input)
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()

	tsv := csv.NewReader(f)
	tsv.Comma = '\t'
	tsv.Comment = '#'

	header, err := tsv.Read()
	if err != nil {
		return fmt.Errorf("while reading %q: %v", name, err)
	}

	cols := []string{
		"tree",
		"node",
		"age",
		"equator",
		"to",
	}
	fields := make(map[string]int)
	for i, h := range header {
		h = strings.ToLower(h)
		fields[h] = i
	}
	for _, c := range cols {
		if _, ok := fields[c]; !ok {
			return fmt.Errorf("while reading %q: expecting column %q", name, c)
		}
	}

	for {
		row, err := tsv.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		ln, _ := tsv.FieldPos(0)
		if err != nil {
			return fmt.Errorf("while reading %q: on row %d: %v", name, ln, err)
		}

		f := "tree"
		tn := strings.Join(strings.Fields(row[fields[f]]), " ")
		if tn == "" {
			continue
		}

		tn = strings.ToLower(tn)
		r, ok := res[tn]
		if !ok {
			continue
		}
		if r.rng == nil {
			r.rng = ranges.New(landscape.Pixelation())
		}

		f = "node"
		id, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return fmt.Errorf("while reading %q: on row %d, field %q: %v", name, ln, f, err)
		}
		if !r.tree.IsTerm(id) {
			continue
		}
		term := r.tree.Taxon(id)

		f = "age"
		age, err := strconv.ParseInt(row[fields[f]], 10, 64)
		if err != nil {
			return fmt.Errorf("while reading %q: on row %d, field %q: %v", name, ln, f, err)
		}
		if r.tree.Age(id) != age {
			continue
		}

		f = "equator"
		eq, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return fmt.Errorf("while reading %q: on row %d, field %q: %v", name, ln, f, err)
		}
		if landscape.Pixelation().Equator() != eq {
			return fmt.Errorf("while reading %q: on row %d, field %q: invalid resolution %d", name, ln, f, eq)
		}

		f = "to"
		px, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return fmt.Errorf("while reading %q: on row %d, field %q: %v", name, ln, f, err)
		}
		r.rng.AddPixel(term, age, px)
	}

	return nil
}

func outHeader(w io.Writer, p, date string) (*csv.Writer, error) {
	fmt.Fprintf(w, "# stochastic mapping on simulated data from project %q\n", p)
	fmt.Fprintf(w, "# up-pass particles: %d\n", numParticles)
	fmt.Fprintf(w, "# date: %s\n", date)

	tsv := csv.NewWriter(w)
	tsv.Comma = '\t'
	tsv.UseCRLF = true
	if err := tsv.Write([]string{"tree", "particle", "node", "age", "equator", "from", "to"}); err != nil {
		return nil, err
	}

	return tsv, nil
}

func writeParticles(tsv *csv.Writer, p int, t *diffusion.Tree, eq int) error {
	nodes := t.Nodes()

	for _, n := range nodes {
		stages := t.Stages(n)
		// skip the first stage
		// (i.e. the post-split stage)
		for i := 1; i < len(stages); i++ {
			a := stages[i]
			st := t.SrcDest(n, p, a)
			if st.From == -1 {
				continue
			}
			row := []string{
				t.Name(),
				strconv.Itoa(p),
				strconv.Itoa(n),
				strconv.FormatInt(a, 10),
				strconv.Itoa(eq),
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
