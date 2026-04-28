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
	geomodel "github.com/js-arias/earth/model"
	"github.com/js-arias/phygeo/infer/model"
	"github.com/js-arias/phygeo/infer/walk"
	"github.com/js-arias/phygeo/infer/walker"
	"github.com/js-arias/phygeo/project"
)

var Command = &command.Command{
	Usage: `particles [-p|--particles <number>]
	-i|--input <file> [-o|--output <file>]
	[--cpu <number>]
	--model <model-file>
	<project-file>`,
	Short: "perform a stochastic mapping",
	Long: `
Command particles reads a file with the conditional likelihoods of one or more
trees in a project and writes the results of a stochastic mapping.

The argument of the command is the name of the project file.

By default, 1000 particles will be simulated for the stochastic mapping. The
number of particles can be changed with the flag --particles, or -p.

The flag --model is required, and it is used to read the model parameter
values. Any undefined value will be set as zero.

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
var modelFile string

func setFlags(c *command.Command) {
	c.Flags().IntVar(&numCPU, "cpu", 0, "")
	c.Flags().IntVar(&numParticles, "p", 1000, "")
	c.Flags().IntVar(&numParticles, "particles", 1000, "")
	c.Flags().StringVar(&inputFile, "input", "", "")
	c.Flags().StringVar(&inputFile, "i", "", "")
	c.Flags().StringVar(&modelFile, "model", "", "")
	c.Flags().StringVar(&outPrefix, "output", "", "")
	c.Flags().StringVar(&outPrefix, "o", "", "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		c.UsageError("expecting project file")
	}
	if modelFile == "" {
		return c.UsageError("--model flag should be defined")
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
	rot.SetUndefAsFix()

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

	mp, err := openModel(modelFile)
	if err != nil {
		return err
	}

	net := earth.NewNetwork(landscape.Pixelation())

	mv := mp.Movement(tr, keys)
	st := mp.Settlement(tr, keys)
	states := tr.States()
	landProb := make([]walker.Model, len(states))
	for i, c := range states {
		sett := walker.Settlement(landscape.Pixelation(), net, mp.Lambda(), int(mp.Steps()))
		lp := walker.New(landscape, net, mv, st, sett, c, i, keys)
		landProb[i] = lp
	}

	rt, err := getRec(inputFile, landscape)
	if err != nil {
		return err
	}

	param := walk.Param{
		Landscape: landscape,
		Rot:       rot,
		Stages:    stages.Stages(),
		Ranges:    rc,
		Traits:    tr,
		Keys:      keys,
		Walker:    landProb,
		Steps:     mp.Steps(),
		Stem:      mp.StemAge(),
		Particles: numParticles,
	}

	walk.StartMap(numCPU, landscape.Pixelation(), len(states))
	for _, t := range rt {
		ct := tc.Tree(t.name)
		if ct == nil {
			continue
		}

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

				states := wt.States()
				for _, tr := range states {
					logLike, ok := s.traits[tr]
					if !ok {
						continue
					}
					wt.SetConditional(n, a, tr, logLike.rec)
				}
			}
		}
		name := fmt.Sprintf("%s-up-%s-x%d.tab", outPrefix, wt.Name(), numParticles)
		if err := upPass(wt, name, p.Name(), mp); err != nil {
			return err
		}
	}
	walk.EndMap()
	return nil
}

func openModel(name string) (*model.Model, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	mp, err := model.Read(f)
	if err != nil {
		return nil, fmt.Errorf("on file %q: %v", name, err)
	}
	return mp, nil
}

func getRec(name string, landscape *geomodel.TimePix) (map[string]*recTree, error) {
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
	name  string
	nodes map[int]*recNode
}

type recNode struct {
	id     int
	tree   *recTree
	stages map[int64]*recStage
}

type recStage struct {
	node   *recNode
	age    int64
	traits map[string]*recTrait
}

type recTrait struct {
	stage *recStage
	trait string
	rec   map[int]float64
}

var headerFields = []string{
	"tree",
	"node",
	"age",
	"type",
	"state",
	"equator",
	"pixel",
	"value",
}

func readRecon(r io.Reader, landscape *geomodel.TimePix) (map[string]*recTree, error) {
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

		f = "tree"
		tn := strings.Join(strings.Fields(row[fields[f]]), " ")
		if tn == "" {
			continue
		}
		tn = strings.ToLower(tn)
		t, ok := rt[tn]
		if !ok {
			t = &recTree{
				name:  tn,
				nodes: make(map[int]*recNode),
			}
			rt[tn] = t
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
				node:   n,
				age:    age,
				traits: make(map[string]*recTrait),
			}
			n.stages[age] = st
		}

		f = "state"
		trait := strings.ToLower(strings.Join(strings.Fields(row[fields[f]]), " "))
		tr, ok := st.traits[trait]
		if !ok {
			tr = &recTrait{
				stage: st,
				trait: trait,
				rec:   make(map[int]float64),
			}
			st.traits[trait] = tr
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
	}
	if len(rt) == 0 {
		return nil, fmt.Errorf("while reading data: %v", io.EOF)
	}

	return rt, nil
}

func upPass(t *walk.Tree, name, p string, mp *model.Model) (err error) {
	pc := t.Mapping()

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
	mp.WriteAsComment(w)
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
		"equator",
		"from",
		"to",
		"path",
	}
	if err := tsv.Write(header); err != nil {
		return fmt.Errorf("while writing header on %q: %v", name, err)
	}
	if err := writeUpPass(tsv, pc, t); err != nil {
		return fmt.Errorf("while writing data on %q: %v", name, err)
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

func writeUpPass(tsv *csv.Writer, pc chan walk.PathChan, t *walk.Tree) error {
	eq := strconv.Itoa(t.Equator())
	states := t.States()

	for path := range pc {
		// skip "post-split" stages
		if !t.IsRoot(path.Node) {
			anc := t.Parent(path.Node)
			if t.Age(anc) == path.Age {
				continue
			}
		}

		nID := strconv.Itoa(path.Node)
		stageAge := strconv.FormatInt(path.Age, 10)
		for i, p := range path.Particles {
			particleID := strconv.Itoa(i)
			row := []string{
				path.Tree,
				particleID,
				nID,
				stageAge,
				eq,
				pathLocation(p, 0, states),
				pathLocation(p, p.Len()-1, states),
				fullPath(p, states),
			}
			if err := tsv.Write(row); err != nil {
				// consume the channel
				for range pc {
				}
				return err
			}
		}
	}
	return nil
}

func pathLocation(p walk.Path, step int, states []string) string {
	px, tr := p.Pos(step)
	return fmt.Sprintf("%s:%d", states[tr], px)
}

func fullPath(p walk.Path, states []string) string {
	var b bytes.Buffer
	for i := range p.Len() {
		if i > 0 {
			b.WriteRune(',')
		}
		b.WriteString(pathLocation(p, i, states))
	}
	return b.String()
}
