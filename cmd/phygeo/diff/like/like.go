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
	"io"
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
	[-p|--particles <number>]
	[-o|--output <file>]
	[--cpu <number>] [--nomat] <project-file>`,
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
defined, it will use 100. As the kappa parameter, lager values indicate low
diffusivity while smaller values indicate high diffusivity.

By default, 1000 particles will be simulated for the stochastic mapping. The
number of particles can be changed with the flag -p, or --particles.

There are two different kinds of results. The first is a TSV file with the
conditional likelihoods (i.e., down-pass results) for each pixel at each node.
The second is a TSV file with the results of the stochastic mapping. These
output files are prefixed with the name of the project file; to set a
different prefix, use the flag --output or -o. After the prefix, the file will
be identified by the tree name and the lambda value. For the down-pass file,
the suffix 'down' will be added, and for the stochastic mapping, it will add
the number of particles. For example, in the tree 'vireya' in project
'rhododendron.tab', using default parameters will result in the files
'rhododendron.tab-vireya-100.000000-down.tab' (conditional likelihood
down-pass) and 'rhododendron.tab-vireya-100.000000x1000.tab' (stochastic
mapping up-pass).

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
var numCPU int
var particles int
var output string
var useRanges bool
var noDMatrix bool

func setFlags(c *command.Command) {
	c.Flags().Float64Var(&lambdaFlag, "lambda", 100, "")
	c.Flags().Float64Var(&stemAge, "stem", 0, "")
	c.Flags().IntVar(&numCPU, "cpu", runtime.GOMAXPROCS(0), "")
	c.Flags().IntVar(&particles, "p", 1000, "")
	c.Flags().IntVar(&particles, "particles", 1000, "")
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
		dt := diffusion.New(t, param)
		name := fmt.Sprintf("%s-%s-%.6f-down.tab", args[0], t.Name(), lambdaFlag)
		if output != "" {
			name = output + "-" + name
		}
		if err := writeTreeConditional(dt, name, args[0], lambdaFlag, standard, landscape.Pixelation().Len()); err != nil {
			return err
		}

		name = fmt.Sprintf("%s-%s-%.6fx%d.tab", args[0], t.Name(), lambdaFlag, particles)
		if output != "" {
			name = output + "-" + name
		}

		if err := upPass(dt, name, args[0], lambdaFlag, standard, particles); err != nil {
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

func upPass(t *diffusion.Tree, name, p string, lambda, standard float64, particles int) (err error) {
	t.Simulate(particles)

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
	tsv, err := outHeader(w, t.Name(), p, lambda, standard, t.LogLike())
	if err != nil {
		return fmt.Errorf("while writing header on %q: %v", name, err)
	}

	for i := 0; i < particles; i++ {
		if err := writeUpPass(tsv, i, t); err != nil {
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

func outHeader(w io.Writer, t, p string, lambda, standard, logLike float64) (*csv.Writer, error) {
	fmt.Fprintf(w, "# diff.like on tree %q of project %q\n", t, p)
	fmt.Fprintf(w, "# lambda: %.6f * 1/radian^2\n", lambda)
	fmt.Fprintf(w, "# standard deviation: %.6f * Km/My\n", standard)
	fmt.Fprintf(w, "# logLikelihood: %.6f\n", logLike)
	fmt.Fprintf(w, "# up-pass particles: %d\n", particles)
	fmt.Fprintf(w, "# date: %s\n", time.Now().Format(time.RFC3339))

	tsv := csv.NewWriter(w)
	tsv.Comma = '\t'
	tsv.UseCRLF = true
	if err := tsv.Write([]string{"tree", "particle", "node", "age", "from", "to"}); err != nil {
		return nil, err
	}

	return tsv, nil
}

func writeUpPass(tsv *csv.Writer, p int, t *diffusion.Tree) error {
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
	if err := tsv.Write([]string{"tree", "node", "age", "type", "pixel", "value"}); err != nil {
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
