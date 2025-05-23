// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package integrate implements a numerical integration
// of the likelihood curve for a diffusion model.
package integrate

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/stat/dist"
	"github.com/js-arias/phygeo/infer/diffusion"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/timetree"
	"gonum.org/v1/gonum/stat/distuv"
)

var Command = &command.Command{
	Usage: `integrate [--stem <age>]
	[--distribution <distribution>] [-p|--particles <number>]
	[--min <float>] [--max <float>] [--mc <number>] [--parts <number>]
	[--cpu <number>] <project-file>`,
	Short: "integrate numerically the likelihood curve",
	Long: `
Command integrate reads a PhyGeo project, and makes a numerical integration of
the likelihood function, using a diffusion model over a sphere, by reporting
the log likelihood values of different values of lambda.

By default, the inference of the root will use the pixel weights at the root
time stage as pixel priors. Use the flag --stem, with a value in million
years, to add a "root branch" with the indicated length. In that case the root
pixels will be closer to the expected equilibrium of the model, at the cost of
increasing computing time.

The flags --min and --max defines the bounds for the values of the lambda
(concentration) parameter of the spherical normal (equivalent to the kappa
parameter of von Mises-Fisher distribution). The units of the lambda parameter
are in 1/radians^2. The default values are 0 and 1000.

If the flag --distribution is defined, it will sample from the indicated
distribution. The sintaxis for a distribution is:

	<name>=<parameter>[,<parameter>...]

Valid distributions are:

	gamma	it requires two parameters, the shape (or alpha), and the rate
		(or lambda).

As the usual objetive of sampling from a distribution is to retrieve the
reconstructions, the flag -p, or --particles, define the number of particles
used for the stochastic mapping. The results will be stored in the file called
"<project>-<tree>-sampling-<samples>x<particles>.tab", as a TSV file. If the
flag -o or --output is defined, the value of the flag will be used as a prefix
for the output file.

By default the command performs an stepwise integration, the flag --parts
indicates the number of segments using for the integration. The default value
is 1000. If the flag --mc is defined, it will perform a Monte Carlo
integration using the indicated number of samples.

Results will be written in the standard output, as a TSV table with the
following columns:

	- tree, for the tree used in the sample
	- lambda, for the value of lambda used in the sample
		(in 1/radians^2)
	- stdDev, for the standard deviation
		(in Km/My)
	- logLike, the log likelihood for the reconstruction

By default, all available CPUs will be used in the processing. Set --cpu flag
to use a different number of CPUs.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var minFlag float64
var maxFlag float64
var mcParts int
var parts int
var numCPU int
var particles int
var stemAge float64
var distribution string
var output string

func setFlags(c *command.Command) {
	c.Flags().Float64Var(&minFlag, "min", 0, "")
	c.Flags().Float64Var(&maxFlag, "max", 1000, "")
	c.Flags().Float64Var(&stemAge, "stem", 0, "")
	c.Flags().IntVar(&numCPU, "cpu", runtime.GOMAXPROCS(0), "")
	c.Flags().IntVar(&mcParts, "mc", 0, "")
	c.Flags().IntVar(&parts, "parts", 1000, "")
	c.Flags().IntVar(&particles, "p", 1000, "")
	c.Flags().IntVar(&particles, "particles", 1000, "")
	c.Flags().StringVar(&distribution, "distribution", "", "")
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

	// Set the number of parallel processors
	diffusion.SetCPU(numCPU)

	dm, _ := earth.NewDistMatRingScale(landscape.Pixelation())

	param := diffusion.Param{
		Landscape: landscape,
		Rot:       rot,
		DM:        dm,
		PW:        pw,
		Ranges:    rc,
		Stages:    stages.Stages(),
	}

	fmt.Fprintf(c.Stdout(), "tree\tlambda\tstdDev\tlogLike\n")
	if distribution != "" {
		r, err := getDistribution()
		if err != nil {
			return err
		}
		for _, tn := range tc.Names() {
			t := tc.Tree(tn)
			param.Stem = int64(stemAge * 1_000_000)
			if err := sample(c.Stdout(), args[0], t, param, r); err != nil {
				return err
			}
		}
		return nil
	}

	fnInt := integrate
	if mcParts > 0 {
		fnInt = monteCarlo
	}
	for _, tn := range tc.Names() {
		t := tc.Tree(tn)
		param.Stem = int64(stemAge * 1_000_000)
		fnInt(c.Stdout(), t, param)
	}

	return nil
}

func sample(w io.Writer, projName string, t *timetree.Tree, p diffusion.Param, r rander) (err error) {
	name := t.Name()
	var bw *bufio.Writer
	var tsv *csv.Writer
	if particles > 0 {
		out := fmt.Sprintf("%s-%s-sampling-%dx%d.tab", projName, t.Name(), parts, particles)
		if output != "" {
			out = output + "-" + out
		}
		f, err := os.Create(out)
		if err != nil {
			return err
		}
		defer func() {
			e := f.Close()
			if err == nil && e != nil {
				err = e
			}
		}()
		bw = bufio.NewWriter(f)
		tsv, err = outHeader(bw, t.Name(), projName)
		if err != nil {
			return fmt.Errorf("while writing header on %q: %v", name, err)
		}
	}

	for i := 0; i < parts; i++ {
		p.Lambda = r.Rand()
		df := diffusion.New(t, p)
		like := df.DownPass()
		standard := calcStandardDeviation(p.Landscape.Pixelation(), p.Lambda)

		fmt.Fprintf(w, "%s\t%.6f\t%.6f\t%.6f\n", name, p.Lambda, standard, like)

		// up-pass
		if particles == 0 {
			continue
		}
		df.Simulate(particles)
		for x := 0; x < particles; x++ {
			if err := writeUpPass(tsv, x, i*particles, df); err != nil {
				return fmt.Errorf("while writing data on %q: %v", name, err)
			}
		}
	}

	if particles == 0 {
		return nil
	}
	tsv.Flush()
	if err := tsv.Error(); err != nil {
		return fmt.Errorf("while writing data on %q: %v", name, err)
	}
	if err := bw.Flush(); err != nil {
		return fmt.Errorf("while writing data on %q: %v", name, err)
	}
	return nil
}

func integrate(w io.Writer, t *timetree.Tree, p diffusion.Param) {
	name := t.Name()
	step := (maxFlag - minFlag) / float64(parts)
	for i := minFlag + step/2; i < maxFlag; i += step {
		p.Lambda = i
		df := diffusion.New(t, p)
		like := df.DownPass()
		standard := calcStandardDeviation(p.Landscape.Pixelation(), p.Lambda)

		fmt.Fprintf(w, "%s\t%.6f\t%.6f\t%.6f\n", name, p.Lambda, standard, like)
	}
}

func monteCarlo(w io.Writer, t *timetree.Tree, p diffusion.Param) {
	name := t.Name()
	size := maxFlag - minFlag
	for i := 0; i < mcParts; i++ {
		p.Lambda = rand.Float64()*size + minFlag
		df := diffusion.New(t, p)
		like := df.DownPass()
		standard := calcStandardDeviation(p.Landscape.Pixelation(), p.Lambda)

		fmt.Fprintf(w, "%s\t%.6f\t%.6f\t%.6f\n", name, p.Lambda, standard, like)
	}
}

// Rander is an interface for probability distributions
// that produce random numbers.
type rander interface {
	Rand() float64
}

func getDistribution() (rander, error) {
	s := strings.Split(distribution, "=")
	if len(s) < 2 {
		return nil, fmt.Errorf("invalid --distribution value: %q", distribution)
	}
	name := strings.ToLower(strings.TrimSpace(s[0]))
	if name == "" {
		return nil, fmt.Errorf("invalid --distribution value: %q", distribution)
	}

	switch name {
	case "gamma":
		p := strings.Split(s[1], ",")
		if len(p) < 2 {
			return nil, fmt.Errorf("invalid --distribution %q: gamma distribution require two parameter values", distribution)
		}
		alpha, err := strconv.ParseFloat(p[0], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid --distribution %q: shape parameter of gamma distribution: %v", distribution, err)
		}
		beta, err := strconv.ParseFloat(p[1], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid --distribution %q: rate parameter of gamma distribution: %v", distribution, err)
		}
		return distuv.Gamma{
			Alpha: alpha,
			Beta:  beta,
			Src:   nil,
		}, nil
	}
	return nil, fmt.Errorf("invalid --distribution: unknown distribution %q", distribution)
}

func outHeader(w io.Writer, t, p string) (*csv.Writer, error) {
	fmt.Fprintf(w, "# diff.integrate on tree %q of project %q\n", t, p)
	fmt.Fprintf(w, "# sampling from distribution: %s\n", distribution)
	fmt.Fprintf(w, "# up-pass particles: %d\n", particles*parts)
	fmt.Fprintf(w, "# date: %s\n", time.Now().Format(time.RFC3339))

	tsv := csv.NewWriter(w)
	tsv.Comma = '\t'
	tsv.UseCRLF = true
	if err := tsv.Write([]string{"tree", "particle", "node", "age", "from", "to"}); err != nil {
		return nil, err
	}

	return tsv, nil
}

func writeUpPass(tsv *csv.Writer, p, cum int, t *diffusion.Tree) error {
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
				strconv.Itoa(p + cum),
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
