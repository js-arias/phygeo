// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package difflike implements a command to perform
// a biogeographic reconstruction using likelihood.
package difflike

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
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
	Usage: `diff.like [--ranges] [--stem <age>] [--lambda <value>]
	[-p|--particles <number>]
	[-o|--output <file>]
	[--cpu <number>] <project-file>`,
	Short: "perform a likelihood reconstruction",
	Long: `
Command like reads a PhyGeo project, perform a likelihood reconstruction for
the trees in the project, using a diffusion model over a sphere, and write the
results of an stochastic mapping.

By default, it will use geographic distributions stored as points (presence-
absence maps). If no there are no point distribution, or the flags --ranges is
defined, the continuous range maps will be used.

By default, an stem branch will be added to each tree using the 10% of the root
age. To set a different stem age use the flag --stem, the value should be in
million years.

The flag --lambda defines the concentration parameter of the spherical normal
(equivalent to kappa parameter of the von Mises-Fisher distribution) for a
diffusion process on a million year using 1/radian^2 units. If no value is
defined, it will use 50. As the kappa parameter, lager values indicate low
diffusivity while smaller values indicate high diffusivity.

By default, 1000 particles will be simulated for the stochastic mapping. The
number of particles can be changed with the flag -p, or --particles.

The results will be writing on a TSV file using the project name, the tree
name, the lambda value, and the number of particles. If the flag -o, or
--output is defined, the indicated string will be used as a prefix for the
file. For example, in the project 'rhododendron.tab', and the tree 'vireya'
using default parameters will result in a file called:
'rhododendron.tab-vireya-1.000000x1000.tab'. If the flag -o is set to 'out' the
resulting file will be: 'out-rhododendron.tab-vireya-1.000000x1000.tab'.

By default, all available CPUs will be used in the processing. Set --cpu flag
to use a different number of CPUs.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var lambdaFlag float64
var stemAge float64
var numCPU int
var particles int
var output string
var useRanges bool

func setFlags(c *command.Command) {
	c.Flags().Float64Var(&lambdaFlag, "lambda", 50, "")
	c.Flags().Float64Var(&stemAge, "stem", 0, "")
	c.Flags().IntVar(&numCPU, "cpu", runtime.GOMAXPROCS(0), "")
	c.Flags().IntVar(&particles, "p", 1000, "")
	c.Flags().IntVar(&particles, "particles", 1000, "")
	c.Flags().StringVar(&output, "output", "", "")
	c.Flags().StringVar(&output, "o", "", "")
	c.Flags().BoolVar(&useRanges, "ranges", false, "")
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

	tpf := p.Path(project.TimePix)
	if tpf == "" {
		msg := fmt.Sprintf("time pixelation not defined in project %q", args[0])
		return c.UsageError(msg)
	}
	tp, err := readTimePix(tpf)
	if err != nil {
		return err
	}

	rotF := p.Path(project.GeoMod)
	if rotF == "" {
		msg := fmt.Sprintf("paleogeographic model not defined in project %q", args[0])
		return c.UsageError(msg)
	}
	rot, err := readRotation(rotF, tp.Pixelation())
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

	param := diffusion.Param{
		TP:     tp,
		Rot:    rot,
		PP:     pp,
		Ranges: rc,
		Lambda: lambdaFlag,
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
		dt := diffusion.New(t, param)

		name := fmt.Sprintf("%s-%s-%.6fx%d.tab", args[0], t.Name(), lambdaFlag, particles)
		if output != "" {
			name = output + "-" + name
		}
		if err := upPass(dt, name, args[0], lambdaFlag, particles); err != nil {
			return err
		}
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

func readTimePix(name string) (*model.TimePix, error) {
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

func upPass(t *diffusion.Tree, name, p string, lambda float64, particles int) (err error) {
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

	tsv, err := outHeader(w, t.Name(), p, lambda, t.LogLike())
	if err != nil {
		return fmt.Errorf("while writing header on %q: %v", name, err)
	}

	for i := 0; i < particles; i++ {
		m := t.Simulate()
		if err := writeUpPass(tsv, m); err != nil {
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
	fmt.Fprintf(w, "# lambda: %.6f 1/radian^2\n", lambda)
	fmt.Fprintf(w, "# logLikelihood: %.6f\n", logLike)
	fmt.Fprintf(w, "# up-pass particles: %d\n", particles)

	tsv := csv.NewWriter(w)
	tsv.Comma = '\t'
	tsv.UseCRLF = true
	if err := tsv.Write([]string{"tree", "node", "age", "from", "to"}); err != nil {
		return nil, err
	}

	return tsv, nil
}

func writeUpPass(tsv *csv.Writer, m *diffusion.Mapping) error {
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
