// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package particles implements a command
// to run a stochastic mapping
// from a down-pass reconstruction
// using random walks.
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
	"github.com/js-arias/earth/model"
	"github.com/js-arias/phygeo/cats"
	"github.com/js-arias/phygeo/infer/catwalk"
	"github.com/js-arias/phygeo/infer/walk"
	"github.com/js-arias/phygeo/project"
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
name will be the output prefix, the word 'up', the tree name and the number of
particles. The extension will be '.tab'.

The output file is a TSV file, indicating the name of the tree, the number of
the particle simulation, the node, the age of the node time stage, the
pixel location and trait state of the particle at the beginning and end of the
stage, and the full path.

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
	c.Flags().IntVar(&numCPU, "cpu", 0, "")
	c.Flags().IntVar(&numParticles, "p", 1000, "")
	c.Flags().IntVar(&numParticles, "particles", 1000, "")
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

	wp, err := p.WalkParam(landscape.Pixelation())
	if err != nil {
		return err
	}

	net := earth.NewNetwork(landscape.Pixelation())

	rt, err := getRec(inputFile, landscape)
	if err != nil {
		return err
	}

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
		Steps:      wp.Steps(),
		MinSteps:   wp.MinSteps(),
		Particles:  numParticles,
	}

	walk.StartMap(numCPU, landscape.Pixelation(), len(tr.States()), numParticles)
	for _, t := range rt {
		ct := tc.Tree(t.name)
		if ct == nil {
			continue
		}

		param.Lambda = t.lambda
		dd, err := cats.Parse(t.relaxed, wp.Cats())
		if err != nil {
			return fmt.Errorf("tree %s: relaxed function: %v", t.name, err)
		}
		if dd.Function() != wp.Function() {
			return fmt.Errorf("tree %s: invalid relaxed function, got %q, want %q", t.name, dd.Function(), wp.Function())
		}
		settCats := catwalk.Cats(landscape.Pixelation(), net, t.lambda, wp.Steps(), dd)
		param.Discrete = settCats

		wt := walk.New(ct, param)
		nodes := wt.Nodes()
		for _, n := range nodes {
			nn, ok := t.nodes[n]
			if !ok {
				return fmt.Errorf("tree %q: node %d: undefined node", wt.Name(), n)
			}
			stages := wt.Stages(n)
			for _, a := range stages {
				s, ok := nn.stages[a]
				if !ok {
					return fmt.Errorf("tree %q: node %d: age %d: undefined stage", wt.Name(), n, a)
				}

				for i := range wt.Cats() {
					c, ok := s.cats[i+1]
					if !ok {
						continue
					}

					traits := wt.Traits()
					for _, tr := range traits {
						logLike, ok := c.traits[tr]
						if !ok {
							continue
						}
						wt.SetConditional(n, a, i, tr, logLike.rec)
					}
				}
			}
		}
		name := fmt.Sprintf("%s-up-%s-x%d.tab", outPrefix, wt.Name(), numParticles)
		if err := upPass(wt, name, p.Name(), param.Lambda, dd); err != nil {
			return err
		}
	}
	walk.EndMap()
	return nil
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
	name    string
	relaxed string
	nodes   map[int]*recNode
	lambda  float64
	oldest  int64
}

type recNode struct {
	id     int
	tree   *recTree
	stages map[int64]*recStage
}

type recStage struct {
	node *recNode
	age  int64
	cats map[int]*recCat
}

type recCat struct {
	stage  *recStage
	cat    int
	traits map[string]*recTrait
}

type recTrait struct {
	cat   *recCat
	trait string
	rec   map[int]float64
}

var headerFields = []string{
	"tree",
	"node",
	"age",
	"type",
	"lambda",
	"relaxed",
	"cat",
	"trait",
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

		f = "relaxed"
		relaxed := strings.ToLower(strings.Join(strings.Fields(row[fields[f]]), " "))

		f = "tree"
		tn := strings.Join(strings.Fields(row[fields[f]]), " ")
		if tn == "" {
			continue
		}
		tn = strings.ToLower(tn)
		t, ok := rt[tn]
		if !ok {
			t = &recTree{
				name:    tn,
				nodes:   make(map[int]*recNode),
				lambda:  lambda,
				relaxed: relaxed,
			}
			rt[tn] = t
		}
		if t.lambda != lambda {
			return nil, fmt.Errorf("on row %d: field %q: got %.6f want %.6f", ln, "lambda", lambda, t.lambda)
		}
		if t.relaxed != relaxed {
			return nil, fmt.Errorf("on row %d: field %q: got %q want %q", ln, "relaxed", relaxed, t.relaxed)
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
				cats: make(map[int]*recCat),
			}
			n.stages[age] = st
		}

		f = "cat"
		cat, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		dc, ok := st.cats[cat]
		if !ok {
			dc = &recCat{
				stage:  st,
				cat:    cat,
				traits: make(map[string]*recTrait),
			}
			st.cats[cat] = dc
		}

		f = "trait"
		trait := strings.ToLower(strings.Join(strings.Fields(row[fields[f]]), " "))
		tr, ok := dc.traits[trait]
		if !ok {
			tr = &recTrait{
				cat:   dc,
				trait: trait,
				rec:   make(map[int]float64),
			}
			dc.traits[trait] = tr
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
		tr.rec[px] = v

		if age > t.oldest {
			t.oldest = age
		}
	}
	if len(rt) == 0 {
		return nil, fmt.Errorf("while reading data: %v", io.EOF)
	}

	return rt, nil
}

func upPass(t *walk.Tree, name, p string, lambda float64, dd cats.Discrete) (err error) {
	t.Mapping()

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
	fmt.Fprintf(w, "# stochastic mapping on tree %q of project %q\n", t.Name(), p)
	fmt.Fprintf(w, "# lambda: %.6f * 1/radian^2\n", lambda)
	fmt.Fprintf(w, "# relaxed diffusion function: %s with %d categories\n", dd, len(dd.Cats()))
	fmt.Fprintf(w, "# steps per million year: %d\n", t.Steps())
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
		"lambda",
		"relaxed",
		"cats",
		"cat",
		"scaled",
		"equator",
		"from",
		"to",
		"path",
	}
	if err := tsv.Write(header); err != nil {
		return fmt.Errorf("while writing header on %q: %v", name, err)
	}
	for p := range numParticles {
		if err := writeUpPass(tsv, p, t, lambda, dd); err != nil {
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

func writeUpPass(tsv *csv.Writer, p int, t *walk.Tree, lambda float64, dd cats.Discrete) error {
	particle := strconv.Itoa(p)
	cats := dd.Cats()
	numberCats := strconv.Itoa(len(cats))
	eq := strconv.Itoa(t.Equator())
	lambdaVal := strconv.FormatFloat(lambda, 'f', 6, 64)

	nodes := t.Nodes()
	for _, n := range nodes {
		nID := strconv.Itoa(n)
		stages := t.Stages(n)
		for _, a := range stages {
			if !t.IsRoot(n) {
				// skip "post-split" stages
				anc := t.Parent(n)
				if t.Age(anc) == a {
					continue
				}
			}
			stageAge := strconv.FormatInt(a, 10)
			path := t.Path(n, a, p)
			c := path.Cat()
			currCat := strconv.Itoa(c + 1)
			scaled := strconv.FormatFloat(lambda*cats[c], 'f', 6, 64)
			row := []string{
				t.Name(),
				particle,
				nID,
				stageAge,
				lambdaVal,
				dd.String(),
				numberCats,
				currCat,
				scaled,
				eq,
				pathLocation(path, 0),
				pathLocation(path, path.Len()-1),
				fullPath(path),
			}
			if err := tsv.Write(row); err != nil {
				return err
			}
		}
	}
	return nil
}

func pathLocation(p walk.Path, step int) string {
	px, tr := p.Pos(step)
	return fmt.Sprintf("%s:%d", tr, px)
}

func fullPath(p walk.Path) string {
	var b bytes.Buffer
	for i := range p.Len() {
		if i > 0 {
			b.WriteRune(',')
		}
		b.WriteString(pathLocation(p, i))
	}
	return b.String()
}
