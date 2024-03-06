// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package sim implements a command to simulate
// a phylogenetic tree
// and its biogeographic data.
package sim

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/stat/pixprob"
	"github.com/js-arias/phygeo/infer/diffusion"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/timetree"
	"github.com/js-arias/timetree/simulate"
)

var Command = &command.Command{
	Usage: `sim [-o|--output <file>]
	[--trees <number>] [--terms <range>] [-p|--particles <number>]
	--age <range> --lambda <range> <project-file>`,
	Short: "simulate data",
	Long: `
Command sim creates one or more random trees with its biogeographic data.

The argument of the command is a PhyGeo project file, used to define the plate
motion model, the landscape model, and the values for the pixel priors.

The flag --age is required and provides the range of the root age. The range
can be a single number (all simulations will have the same age) or a range
separated by a comma; for example, "66,251" will simulate trees selecting root
ages between 251 and 66 million years.

By default, 100 trees will be created. Use the flag --trees to define a
different number of trees.

By default, each tree will have between 40 and 80 terminals. Use the flag
--terms to define a range. The range can be a single number (all simulated
trees will have the indicated number of terminals) or a range separated by a
comma; for example, "40,80" defines the default range.

Trees will be simulated using a Yule process, with the speciation rate defined
as spRate = (ln(terms) - ln(2)) / rootAge.

The flag --lambda is required and provides the range of the concentration
parameter. The range can be a single number (all simulations will have the
same concentration parameter) or a range separated by a comma: for example
"0,100" will simulate diffusion with concentration parameters between 0 and
100.

By default, 100 particles will be simulated for the stochastic mapping. The
number of particles can be changed with the flag --particles, or -p.

	`,
	SetFlags: setFlags,
	Run:      run,
}

var output string
var ageFlag string
var termFlag string
var lambdaFlag string
var numTrees int
var numParticles int

func setFlags(c *command.Command) {
	c.Flags().StringVar(&output, "output", "sim", "")
	c.Flags().StringVar(&output, "o", "sim", "")
	c.Flags().StringVar(&ageFlag, "age", "", "")
	c.Flags().StringVar(&termFlag, "terms", "40,80", "")
	c.Flags().StringVar(&lambdaFlag, "lambda", "", "")
	c.Flags().IntVar(&numTrees, "trees", 100, "")
	c.Flags().IntVar(&numParticles, "p", 100, "")
	c.Flags().IntVar(&numParticles, "particles", 100, "")
}

const millionYears = 1_000_000

func run(c *command.Command, args []string) (err error) {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}

	if ageFlag == "" {
		return c.UsageError("flag --age undefined")
	}

	p, err := project.Read(args[0])
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

	dm, err := earth.NewDistMatRingScale(landscape.Pixelation())
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

	min, max, err := parseFloatRange(ageFlag)
	if err != nil {
		return err
	}
	minAge := int64(min * millionYears)
	maxAge := int64(max * millionYears)

	minTerm, maxTerm, err := parseIntRange(termFlag)
	if err != nil {
		return err
	}
	avgTerm := minTerm + (maxTerm-minTerm)/2

	minLambda, maxLambda, err := parseFloatRange(lambdaFlag)
	if err != nil {
		return err
	}

	outFile := fmt.Sprintf("%s-particles.tab", output)
	f, err := os.Create(outFile)
	if err != nil {
		return err
	}
	defer func() {
		e := f.Close()
		if err == nil && e != nil {
			err = e
		}
	}()
	tsv, err := outHeader(f, args[0])
	if err != nil {
		return fmt.Errorf("while writing header on %q: %v", outFile, err)
	}

	coll := timetree.NewCollection()
	vals := make(map[string]float64, numTrees)
	for i := 0; i < numTrees; i++ {
		name := fmt.Sprintf("random-%d", i)

		// simulate the tree
		var t *timetree.Tree
		for {
			root := maxAge
			if d := maxAge - minAge; d > 0 {
				root = rand.Int64N(d) + minAge
			}

			spRate := (math.Log(float64(avgTerm)) - math.Log(2)) / (float64(root) / millionYears)
			t, _ = simulate.Yule(name, spRate, root, maxTerm*2)
			if tm := len(t.Terms()); tm >= minTerm && tm <= maxTerm {
				break
			}
		}
		t.Format()
		coll.Add(t)

		lambda := maxLambda
		if maxLambda != minLambda {
			diff := maxLambda - minLambda
			lambda = rand.Float64()*diff + minLambda
		}

		rootAge := t.Age(t.Root())

		// geographic part
		param := diffusion.Param{
			Landscape: landscape,
			Rot:       rot,
			DM:        dm,
			PP:        pp,
			Stem:      rootAge / 10,
			Lambda:    lambda,
		}

		sim := diffusion.NewSimData(t, param)
		sim.Simulate(numParticles)
		if err := writeSimulation(tsv, sim, landscape.Pixelation().Equator()); err != nil {
			return fmt.Errorf("while writing data on %q: %v", outFile, err)
		}

		vals[t.Name()] = lambda
	}

	tsv.Flush()
	if err := tsv.Error(); err != nil {
		return fmt.Errorf("while writing data on %q: %v", outFile, err)
	}

	if err := writeLambdaVals(vals, args[0]); err != nil {
		return err
	}

	if err := writeTrees(coll); err != nil {
		return err
	}
	return nil
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

func parseFloatRange(s string) (min, max float64, err error) {
	f := strings.Split(s, ",")
	if len(f) == 1 {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid range %q: %v", s, err)
		}
		return v, v, nil
	}

	if len(f) != 2 {
		return 0, 0, fmt.Errorf("invalid range %q: expecting two values", s)
	}

	min, err = strconv.ParseFloat(f[0], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid range %q: %v", s, err)
	}

	max, err = strconv.ParseFloat(f[1], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid range %q: %v", s, err)
	}

	if max < min {
		min, max = max, min
	}

	return min, max, nil
}

func parseIntRange(s string) (min, max int, err error) {
	f := strings.Split(s, ",")
	if len(f) == 1 {
		v, err := strconv.Atoi(s)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid range %q: %v", s, err)
		}
		return v, v, nil
	}

	if len(f) != 2 {
		return 0, 0, fmt.Errorf("invalid range %q: expecting two values", s)
	}

	min, err = strconv.Atoi(f[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid range %q: %v", s, err)
	}

	max, err = strconv.Atoi(f[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid range %q: %v", s, err)
	}

	if max < min {
		min, max = max, min
	}

	return min, max, nil
}

func writeTrees(coll *timetree.Collection) (err error) {
	name := fmt.Sprintf("%s-trees.tab", output)
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	defer func() {
		e := f.Close()
		if e != nil && err == nil {
			err = e
		}
	}()

	if err := coll.TSV(f); err != nil {
		return fmt.Errorf("while writing to %q: %v", output, err)
	}

	return nil
}

func outHeader(w io.Writer, p string) (*csv.Writer, error) {
	fmt.Fprintf(w, "# simulated data of project %q\n", p)
	fmt.Fprintf(w, "# simulated particles: %d\n", numParticles)
	fmt.Fprintf(w, "# date: %s\n", time.Now().Format(time.RFC3339))

	tsv := csv.NewWriter(w)
	tsv.Comma = '\t'
	tsv.UseCRLF = true
	if err := tsv.Write([]string{"tree", "particle", "node", "age", "equator", "from", "to"}); err != nil {
		return nil, err
	}

	return tsv, nil
}

func writeSimulation(tsv *csv.Writer, t *diffusion.Tree, eq int) error {
	nodes := t.Nodes()

	for _, n := range nodes {
		stages := t.Stages(n)
		// skip the first stage
		// (i.e. the post-split stage)
		for i := 1; i < len(stages); i++ {
			a := stages[i]

			nv := strconv.Itoa(n)
			av := strconv.FormatInt(a, 10)
			eqv := strconv.Itoa(eq)

			for p := 0; p < t.Particles(n, a); p++ {
				st := t.SrcDest(n, p, a)
				if st.From == -1 {
					continue
				}
				row := []string{
					t.Name(),
					strconv.Itoa(p),
					nv,
					av,
					eqv,
					strconv.Itoa(st.From),
					strconv.Itoa(st.To),
				}
				if err := tsv.Write(row); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func writeLambdaVals(lv map[string]float64, p string) (err error) {
	name := fmt.Sprintf("%s-lambda.tab", output)
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	defer func() {
		e := f.Close()
		if e != nil && err == nil {
			err = fmt.Errorf("on file %q: %v", name, e)
		}
	}()

	fmt.Fprintf(f, "# simulated lambda of project %q\n", p)
	fmt.Fprintf(f, "# date: %s\n", time.Now().Format(time.RFC3339))

	tsv := csv.NewWriter(f)
	tsv.Comma = '\t'
	tsv.UseCRLF = true
	if err := tsv.Write([]string{"tree", "lambda"}); err != nil {
		return fmt.Errorf("unable to write header to %q: %v", name, err)
	}

	trees := make([]string, 0, len(lv))
	for t := range lv {
		trees = append(trees, t)
	}
	slices.Sort(trees)

	for _, t := range trees {
		v := strconv.FormatFloat(lv[t], 'f', 6, 64)
		if err := tsv.Write([]string{t, v}); err != nil {
			return fmt.Errorf("unable to write data to %q: %v", name, err)
		}
	}

	tsv.Flush()
	if err := tsv.Error(); err != nil {
		return fmt.Errorf("unable to write data to %q: %v", name, err)
	}

	return nil
}
