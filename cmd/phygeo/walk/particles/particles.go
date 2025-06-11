// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package particles implements a command
// to run stochastic mapping
// from a down-pass reconstruction.
package particles

import (
	"bufio"
	"bytes"
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
	"github.com/js-arias/phygeo/cats"
	"github.com/js-arias/phygeo/infer/walk"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/phygeo/timestage"
	"github.com/js-arias/timetree"
)

var Command = &command.Command{
	Usage: `particles
	[--min <number>] [--max <number>]
	[--walkers <value>] [-p|--particles]
	-i|--input <file> [-o|--output <file>]
	[--cpu <number>]
	<project-file>`,
	Short: "perform a stochastic mapping",
	Long: `
Command particles reads a file with the conditional likelihoods of one or more
trees in a project and writes the results of a stochastic mapping.

The argument of the command is the name of the project file.

Flags --min and --max define the minimum and maximum number of steps in a
branch-category. Defaults are 3 and 1000.

By default, 1000 particles will be simulated for the stochastic mapping. The
number of particles can be changed with the flag --particles, or -p. By
default 100 walkers will be attempted per particle. Use the flag --walkers to
set a different number.

The flag --input, or -i, is required and indicates the input file. The input
file is a pixel probability file with stored log-likelihoods.

The output file is a TSV file, including the name of the tree, the node, the
time stage, the particle simulation, the pixel location, and the trait, at the
beginning and the end of the stage. The prefix of the output file is the name
of the project file. To set a different prefix, use the flag --output, or -o.
The output file name will have the output prefix, the word 'pp' with the
number of particles, amd the tree name. The extension will be '.tab'.

By default, all available CPU will be used in the calculations. Set the flag
--cpu to use a different number of CPUs.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var minSteps int
var maxSteps int
var walkers int
var numCPU int
var numParticles int
var inputFile string
var outPrefix string

func setFlags(c *command.Command) {
	c.Flags().IntVar(&minSteps, "min", 3, "")
	c.Flags().IntVar(&maxSteps, "max", 1000, "")
	c.Flags().IntVar(&numParticles, "p", 1000, "")
	c.Flags().IntVar(&numParticles, "particles", 1000, "")
	c.Flags().IntVar(&walkers, "walkers", 100, "")
	c.Flags().IntVar(&numCPU, "cpu", 0, "")
	c.Flags().StringVar(&inputFile, "input", "", "")
	c.Flags().StringVar(&inputFile, "i", "", "")
	c.Flags().StringVar(&outPrefix, "output", "", "")
	c.Flags().StringVar(&outPrefix, "o", "", "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		c.UsageError("expecting project file")
	}

	p, err := project.Read(args[0])
	if err != nil {
		return err
	}
	if outPrefix == "" {
		outPrefix = p.NameRoot()
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
		MinSteps:   minSteps,
		MaxSteps:   maxSteps,
		Walkers:    walkers,
	}

	wtc, err := getRec(inputFile, tc, param)
	if err != nil {
		return err
	}

	for _, t := range wtc {
		t.UpPass(numCPU, numParticles)
		if err := writeOutput(t, p.Name()); err != nil {
			return err
		}
	}

	return nil
}

func getRec(name string, tc *timetree.Collection, p walk.Param) (map[string]*walk.Tree, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rt, err := readRecons(f, tc, p)
	if err != nil {
		return nil, fmt.Errorf("on input file %q: %v", name, err)
	}

	wtc := make(map[string]*walk.Tree)
	for _, tt := range rt {
		bt := tc.Tree(tt.name)
		if bt == nil {
			continue
		}
		p.Steps = tt.steps
		p.Discrete = tt.cats
		p.Stem = tt.oldest - bt.Age(bt.Root())
		wt := walk.New(bt, p)
		nodes := wt.Nodes()
		for _, n := range nodes {
			nn, ok := tt.nodes[n]
			if !ok {
				return nil, fmt.Errorf("tree %q: node %d: undefined node", tt.name, n)
			}
			stages := wt.Stages(n)
			for _, a := range stages {
				s, ok := nn.stages[a]
				if !ok {
					return nil, fmt.Errorf("tree %q: node %d: age %.6f: undefined conditional likelihood", tt.name, n, float64(a)/timestage.MillionYears)
				}
				wt.SetConditional(n, a, s.rec)
			}
		}
		wtc[wt.Name()] = wt
	}
	return wtc, nil
}

type recTree struct {
	name   string
	nodes  map[int]*recNode
	steps  float64
	cats   cats.Discrete
	oldest int64
}

type recNode struct {
	stages map[int64]*recStage
}

type recStage struct {
	rec map[string]map[int]float64
}

var headerFields = []string{
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

func readRecons(r io.Reader, tc *timetree.Collection, p walk.Param) (map[string]*recTree, error) {
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
			return nil, fmt.Errorf("on row %d: field %q: expecting 'log-like' type", ln, f)
		}

		f = "tree"
		tn := strings.Join(strings.Fields((row[fields[f]])), " ")
		if tn == "" {
			continue
		}
		tt := tc.Tree(tn)
		if tt == nil {
			continue
		}
		t, ok := rt[tt.Name()]
		if !ok {
			f = "steps"
			steps, err := strconv.ParseFloat(row[fields[f]], 64)
			if err != nil {
				return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
			}

			f = "cats"
			numCats, err := strconv.Atoi(row[fields[f]])
			if err != nil {
				return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
			}

			f = "relaxed"
			dd, err := cats.Parse(row[fields[f]], numCats)
			if err != nil {
				return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
			}

			t = &recTree{
				name:  tt.Name(),
				nodes: make(map[int]*recNode, len(tt.Nodes())),
				steps: steps,
				cats:  dd,
			}
			rt[t.name] = t
		}

		f = "node"
		id, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		n, ok := t.nodes[id]
		if !ok {
			n = &recNode{
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
				rec: make(map[string]map[int]float64),
			}
			n.stages[age] = st
		}

		f = "trait"
		tr := strings.Join(strings.Fields(strings.ToLower(row[fields[f]])), " ")
		if !p.Traits.HasTrait(tr) {
			return nil, fmt.Errorf("on row %d: field %q: unexpected trait %q", ln, f, tr)
		}
		if _, ok := st.rec[tr]; !ok {
			st.rec[tr] = make(map[int]float64)
		}

		f = "equator"
		eq, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if eq != p.Landscape.Pixelation().Equator() {
			return nil, fmt.Errorf("on row %d: field %q: invalid equator value %d", ln, f, eq)
		}

		f = "pixel"
		px, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if px >= p.Landscape.Pixelation().Len() {
			return nil, fmt.Errorf("on row %d: field %q: invalid pixel value %d", ln, f, px)
		}

		f = "value"
		v, err := strconv.ParseFloat(row[fields[f]], 64)
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		st.rec[tr][px] = v

		if age > t.oldest {
			t.oldest = age
		}
	}
	if len(rt) == 0 {
		return nil, fmt.Errorf("while reading data: %v", io.EOF)
	}

	return rt, nil
}

func writeOutput(t *walk.Tree, p string) (err error) {
	name := fmt.Sprintf("%s-pp-%d-%s.tab", outPrefix, numParticles, t.Name())
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

	w := bufio.NewWriter(f)
	fmt.Fprintf(w, "# stochastic mapping on tree %q of project %q\n", t.Name(), p)
	fmt.Fprintf(w, "# base steps per million year: %.6f\n", t.Steps())
	fmt.Fprintf(w, "# walkers per rate category: %d\n", walkers)
	fmt.Fprintf(w, "# logLikelihood: %.6f\n", t.LogLike())
	fmt.Fprintf(w, "# up-pass particles: %d\n", numParticles)
	fmt.Fprintf(w, "# date: %s\n", time.Now().Format(time.RFC3339))

	tsv := csv.NewWriter(w)
	tsv.Comma = '\t'
	tsv.UseCRLF = true
	header := []string{
		"tree",
		"particle",
		"node",
		"age",
		"steps",
		"relaxed",
		"cats",
		"equator",
		"rate",
		"from",
		"to",
		"path",
		"ini-trait",
		"end-trait",
	}
	if err := tsv.Write(header); err != nil {
		return err
	}
	steps := strconv.FormatFloat(t.Steps(), 'f', 6, 64)
	relaxed := t.Discrete().String()
	numCats := strconv.Itoa(t.NumCats())
	eq := strconv.Itoa(t.Equator())

	for pp := range numParticles {
		ppID := strconv.Itoa(pp)
		nodes := t.Nodes()
		for _, n := range nodes {
			nID := strconv.Itoa(n)
			stages := t.Stages(n)
			for i := range stages {
				if !t.IsRoot(n) {
					// skip the first stage of the node
					// (i.e., the "post-split")
					if i == 0 {
						continue
					}
				}
				if t.IsRoot(n) {
					// skip all root stages
					// except the split
					if i < len(stages)-1 {
						continue
					}
				}
				a := stages[i]
				path := t.Path(pp, n, a)
				if path.From < 0 {
					continue
				}
				row := []string{
					t.Name(),
					ppID,
					nID,
					strconv.FormatInt(a, 10),
					steps,
					relaxed,
					numCats,
					eq,
					strconv.Itoa(path.Cat),
					strconv.Itoa(path.From),
					strconv.Itoa(path.To),
					pathString(path.Path),
					path.TraitStart,
					path.TraitEnd,
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

func pathString(path []int) string {
	var buf bytes.Buffer
	for i, px := range path {
		if i > 0 {
			buf.WriteRune('-')
		}
		buf.WriteString(strconv.Itoa(px))
	}
	return buf.String()
}
