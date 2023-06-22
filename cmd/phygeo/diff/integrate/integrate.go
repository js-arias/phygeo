// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package integrate implements a numerical integration
// of the likelihood curve for a diffusion model.
package integrate

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/stat/pixprob"
	"github.com/js-arias/phygeo/infer/diffusion"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/ranges"
	"github.com/js-arias/timetree"
	"golang.org/x/exp/rand"
)

var Command = &command.Command{
	Usage: `integrate [--ranges] [--stem <age>]
	[--min <float>] [--max <float>] [--mc <number>] [--parts <number>]
	[--cpu <number>] [--nomat] <project-file>`,
	Short: "integrate numerically the likelihood curve",
	Long: `
Command integrate reads a PhyGeo project, and makes a numerical integration of
the likelihood function, using a diffusion model over a sphere, by reporting
the log likelihood values of different values of lambda.

By default, it will use geographic distributions stored as points (presence-
absence maps). If no there are no point distribution, or the flags --ranges is
defined, the continuous range maps will be used.

By default, an stem branch will be added to each tree using the 10% of the root
age. To set a different stem age use the flag --stem, the value should be in
million years.

The flags --min and --max defines the bounds for the values of the lambda
(concentration) parameter of the spherical normal (equivalent to the kappa
parameter of von Mises-Fisher distribution). The units of the lambda parameter
are in 1/radians^2. The default values are 0 and 1000.

By default the command performs an stepwise integration, the flag --parts
indicates the number of segments using for the integration. The default value
is 1000. If the flag --mc is defined, it will perform a Monte Carlo
integration using the indicated number of samples.

Results will be written in the standard output, as a TSV table with the
following columns:

	- tree, for the tree used in the sample
	- lambda, for the value of lambda used in the sample
	- logLike, the log likelihood for the reconstruction

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

var minFlag float64
var maxFlag float64
var mcParts int
var parts int
var numCPU int
var stemAge float64
var useRanges bool
var noDMatrix bool

func setFlags(c *command.Command) {
	c.Flags().Float64Var(&minFlag, "min", 0, "")
	c.Flags().Float64Var(&maxFlag, "max", 1000, "")
	c.Flags().Float64Var(&stemAge, "stem", 0, "")
	c.Flags().IntVar(&numCPU, "cpu", runtime.GOMAXPROCS(0), "")
	c.Flags().IntVar(&mcParts, "mc", 0, "")
	c.Flags().IntVar(&parts, "parts", 1000, "")
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

	// Start random number generator
	rand.Seed(uint64(time.Now().UnixNano()))

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

	fmt.Fprintf(c.Stdout(), "tree\tlambda\tlogLike\n")
	fnInt := integrate
	if mcParts > 0 {
		fnInt = monteCarlo
	}
	for _, tn := range tc.Names() {
		t := tc.Tree(tn)
		stem := int64(stemAge * 1_000_000)
		if stem == 0 {
			stem = t.Age(t.Root()) / 10
		}
		param.Stem = stem
		fnInt(c.Stdout(), t, param)
	}

	return nil
}

func integrate(w io.Writer, t *timetree.Tree, p diffusion.Param) {
	name := t.Name()
	step := (maxFlag - minFlag) / float64(parts)
	for i := minFlag + step/2; i < maxFlag; i += step {
		p.Lambda = i
		df := diffusion.New(t, p)
		like := df.LogLike()

		fmt.Fprintf(w, "%s\t%.6f\t%.6f\n", name, i, like)
	}
}

func monteCarlo(w io.Writer, t *timetree.Tree, p diffusion.Param) {
	name := t.Name()
	size := maxFlag - minFlag
	for i := 0; i < mcParts; i++ {
		p.Lambda = rand.Float64()*size + minFlag
		df := diffusion.New(t, p)
		like := df.LogLike()

		fmt.Fprintf(w, "%s\t%.6f\t%.6f\n", name, p.Lambda, like)
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
