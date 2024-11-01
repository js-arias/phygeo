// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package particles implements a command
// to run a stochastic mapping
// from a down-pass reconstruction.
package particles

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/stat/dist"
	"github.com/js-arias/earth/stat/pixweight"
	"github.com/js-arias/phygeo/infer/diffusion"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/phygeo/timestage"
	"github.com/js-arias/ranges"
	"github.com/js-arias/timetree"
)

var Command = &command.Command{
	Usage: `particles [-p|--particles <number>]
	-i|--input <file> [-o|--output <file>]
	[--cpu <number>] <project-file>`,
	Short: "perform a stochastic mapping",
	Long: `
Command particles reads a file with the conditional likelihoods of one or more
trees in a project and writes the results of a stochastic mapping.

The argument of the command is the name of the project file.

By default, 1000 particles will be simulated for the stochastic mapping. The
number of particles can be changed with the flag --particles, or -p.

The flag --input, or -i, is required and indicates the input file. The input
file is a pixel probability file with stored log-likelihoods.

The prefix for the name of the output file will be the name of the project
file. To set a different prefix, use the flag --output, or -o. The full file
name will be the prefix, the tree name, the value of lambda, and the number of
particles.

The output file is a TSV file, indicating the name of the tree, the number of
the particle simulation, the node, the age of the node time stage, and the
pixel location of the particle at the beginning and end of the stage.

By default, all available CPUs will be used in the processing. Set the --cpu
flag to use a different number of CPUs.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var numCPU int
var numParticles int
var inputFile string
var outPrefix string

func setFlags(c *command.Command) {
	c.Flags().IntVar(&numCPU, "cpu", runtime.GOMAXPROCS(0), "")
	c.Flags().IntVar(&numParticles, "p", 1000, "")
	c.Flags().IntVar(&numParticles, "particles", 1000, "")
	c.Flags().StringVar(&inputFile, "input", "", "")
	c.Flags().StringVar(&inputFile, "i", "", "")
	c.Flags().StringVar(&outPrefix, "output", "", "")
	c.Flags().StringVar(&outPrefix, "o", "", "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}

	p, err := project.Read(args[0])
	if err != nil {
		return err
	}
	if outPrefix == "" {
		outPrefix = args[0]
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

	stF := p.Path(project.Stages)
	stages, err := readStages(stF, rot, landscape)
	if err != nil {
		return err
	}

	pwF := p.Path(project.PixWeight)
	if pwF == "" {
		msg := fmt.Sprintf("pixel weights not defined in project %q", args[0])
		return c.UsageError(msg)
	}
	pw, err := readPixWeights(pwF)
	if err != nil {
		return err
	}

	rf := p.Path(project.Ranges)
	rc, err := readRanges(rf)
	if err != nil {
		return err
	}

	dm, _ := earth.NewDistMatRingScale(landscape.Pixelation())

	rt, err := getRec(inputFile, landscape)
	if err != nil {
		return err
	}

	// Set the number of parallel processors
	diffusion.SetCPU(numCPU)

	param := diffusion.Param{
		Landscape: landscape,
		Rot:       rot,
		DM:        dm,
		PW:        pw,
		Ranges:    rc,
		Stages:    stages.Stages(),
	}

	for _, t := range rt {
		ct := tc.Tree(t.name)
		if ct == nil {
			continue
		}
		param.Lambda = t.lambda
		param.Stem = t.oldest - ct.Age(ct.Root())
		standard := calcStandardDeviation(landscape.Pixelation(), t.lambda)

		dt := diffusion.New(ct, param)
		nodes := dt.Nodes()
		for _, n := range nodes {
			nn, ok := t.nodes[n]
			if !ok {
				return fmt.Errorf("tree %q: node %d: undefined node", dt.Name(), n)
			}
			stages := dt.Stages(n)

			for _, a := range stages {
				s, ok := nn.stages[a]
				if !ok {
					return fmt.Errorf("tree %q: node %d: age %d: undefined conditional likelihood", dt.Name(), n, a)
				}

				dt.SetConditional(n, a, s.rec)
			}
		}

		name := fmt.Sprintf("%s-%s-%.6fx%d.tab", outPrefix, dt.Name(), t.lambda, numParticles)
		if err := upPass(dt, name, args[0], t.lambda, standard, numParticles, landscape.Pixelation().Equator()); err != nil {
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

func readStages(name string, rot *model.StageRot, landscape *model.TimePix) (timestage.Stages, error) {
	stages := timestage.New()
	stages.Add(rot)
	stages.Add(landscape)

	if name == "" {
		return stages, nil
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	st, err := timestage.Read(f)
	if err != nil {
		return nil, fmt.Errorf("when reading %q: %v", name, err)
	}
	stages.Add(st)

	return stages, nil
}

func readPixWeights(name string) (pixweight.Pixel, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	pw, err := pixweight.ReadTSV(f)
	if err != nil {
		return nil, fmt.Errorf("when reading %q: %v", name, err)
	}

	return pw, nil
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

func getRec(name string, landscape *model.TimePix) (map[string]*recTree, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rt, err := readRecon(f, landscape)
	if err != nil {
		return nil, fmt.Errorf("on input file %q: %v", name, err)
	}
	return rt, nil
}

type recTree struct {
	name   string
	nodes  map[int]*recNode
	lambda float64
	oldest int64
}

type recNode struct {
	id     int
	tree   *recTree
	stages map[int64]*recStage
}

type recStage struct {
	node *recNode
	age  int64
	rec  map[int]float64
}

var headerFields = []string{
	"tree",
	"node",
	"age",
	"type",
	"lambda",
	"equator",
	"pixel",
	"value",
}

func readRecon(r io.Reader, landscape *model.TimePix) (map[string]*recTree, error) {
	tsv := csv.NewReader(r)
	tsv.Comma = '\t'
	tsv.Comment = '#'

	head, err := tsv.Read()
	if err != nil {
		return nil, fmt.Errorf("while reading header: %v", err)
	}
	fields := make(map[string]int, len(head))
	for i, h := range head {
		h = strings.ToLower(h)
		fields[h] = i
	}
	for _, h := range headerFields {
		if _, ok := fields[h]; !ok {
			return nil, fmt.Errorf("expecting field %q", h)
		}
	}

	rt := make(map[string]*recTree)
	for {
		row, err := tsv.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		ln, _ := tsv.FieldPos(0)
		if err != nil {
			return nil, fmt.Errorf("on row %d: %v", ln, err)
		}

		f := "type"
		tpV := strings.ToLower(strings.Join(strings.Fields(row[fields[f]]), " "))
		if tpV != "log-like" {
			return nil, fmt.Errorf("on row %d: field %q: expecting log-like type", ln, f)
		}

		f = "lambda"
		lambda, err := strconv.ParseFloat(row[fields[f]], 64)
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}

		f = "tree"
		tn := strings.Join(strings.Fields(row[fields[f]]), " ")
		if tn == "" {
			continue
		}
		tn = strings.ToLower(tn)
		t, ok := rt[tn]
		if !ok {
			t = &recTree{
				name:   tn,
				nodes:  make(map[int]*recNode),
				lambda: lambda,
			}
			rt[tn] = t
		}
		if t.lambda != lambda {
			return nil, fmt.Errorf("on row %d: field %q: got %.6f want %.6f", ln, "lambda", lambda, t.lambda)
		}

		f = "node"
		id, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		n, ok := t.nodes[id]
		if !ok {
			n = &recNode{
				id:     id,
				tree:   t,
				stages: make(map[int64]*recStage),
			}
			t.nodes[id] = n
		}

		f = "age"
		age, err := strconv.ParseInt(row[fields[f]], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		st, ok := n.stages[age]
		if !ok {
			st = &recStage{
				node: n,
				age:  age,
				rec:  make(map[int]float64),
			}
			n.stages[age] = st
		}

		f = "equator"
		eq, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if eq != landscape.Pixelation().Equator() {
			return nil, fmt.Errorf("on row %d: field %q: invalid equator value %d", ln, f, eq)
		}

		f = "pixel"
		px, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if px >= landscape.Pixelation().Len() {
			return nil, fmt.Errorf("on row %d: field %q: invalid pixel value %d", ln, f, px)
		}

		f = "value"
		v, err := strconv.ParseFloat(row[fields[f]], 64)
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		st.rec[px] = v

		if age > t.oldest {
			t.oldest = age
		}
	}
	if len(rt) == 0 {
		return nil, fmt.Errorf("while reading data: %v", io.EOF)
	}

	return rt, nil
}

// CalcStandardDeviation returns the standard deviation
// (i.e. the square root of variance)
// in km per million year.
func calcStandardDeviation(pix *earth.Pixelation, lambda float64) float64 {
	n := dist.NewNormal(lambda, pix)
	v := n.Variance()
	return math.Sqrt(v) * earth.Radius / 1000
}

func upPass(t *diffusion.Tree, name, p string, lambda, standard float64, particles, eq int) (err error) {
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
		if err := writeUpPass(tsv, i, t, lambda, eq); err != nil {
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
	fmt.Fprintf(w, "# stochastic mapping on tree %q of project %q\n", t, p)
	fmt.Fprintf(w, "# lambda: %.6f * 1/radian^2\n", lambda)
	fmt.Fprintf(w, "# standard deviation: %.6f * Km/My\n", standard)
	fmt.Fprintf(w, "# logLikelihood: %.6f\n", logLike)
	fmt.Fprintf(w, "# up-pass particles: %d\n", numParticles)
	fmt.Fprintf(w, "# date: %s\n", time.Now().Format(time.RFC3339))

	tsv := csv.NewWriter(w)
	tsv.Comma = '\t'
	tsv.UseCRLF = true
	if err := tsv.Write([]string{"tree", "particle", "node", "age", "lambda", "equator", "from", "to"}); err != nil {
		return nil, err
	}

	return tsv, nil
}

func writeUpPass(tsv *csv.Writer, p int, t *diffusion.Tree, lambda float64, eq int) error {
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
				strconv.FormatFloat(lambda, 'f', 6, 64),
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
