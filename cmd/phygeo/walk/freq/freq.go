// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package freq implements a command
// to calculate pixel frequencies
// from the stochastic mapping output.
package freq

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/js-arias/command"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/timetree"
)

var Command = &command.Command{
	Usage: `freq
	-i|--input <file>
	[-o|--output <file>] <project-file>`,
	Short: "calculate pixel frequencies",
	Long: `
Command freq reads a file from a stochastic mapping reconstruction for nodes
of one or more trees in a project and produces an equivalent file with pixel
frequencies.

The argument of the command is the name of the project file.

The flag --input, or -i, indicates the input file from a stochastic mapping.
This is a required parameter.

By default, the output will use the project name as a prefix. Use the flag
--output, or -i, to set a different output prefix. After the prefix, the word
'freq' will be added and the name of the tree. The extension will be '.tab'.

The output file is a tab delimited file, with the node and age stages, and the
pixel frequencies for each category and trait.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var inputFile string
var outPrefix string

func setFlags(c *command.Command) {
	c.Flags().StringVar(&inputFile, "input", "", "")
	c.Flags().StringVar(&inputFile, "i", "", "")
	c.Flags().StringVar(&outPrefix, "output", "", "")
	c.Flags().StringVar(&outPrefix, "o", "", "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}
	if inputFile == "" {
		return c.UsageError("expecting input file, flag --input")
	}

	p, err := project.Read(args[0])
	if err != nil {
		return err
	}

	landscape, err := p.Landscape(nil)
	if err != nil {
		return err
	}

	tc, err := p.Trees()
	if err != nil {
		return err
	}

	rt, err := getRec(tc, landscape)
	if err != nil {
		return err
	}
	scale(rt)

	if outPrefix == "" {
		outPrefix = p.NameRoot()
	}

	for _, t := range rt {
		if err := writeFrequencies(t, landscape, p.NameRoot()); err != nil {
			return err
		}
	}
	return nil
}

func getRec(tc *timetree.Collection, tp *model.TimePix) (map[string]*recTree, error) {
	f, err := os.Open(inputFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rt, err := readRecPixels(f, tc, tp)
	if err != nil {
		return nil, fmt.Errorf("on input file %q: %v", inputFile, err)
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
	node      *recNode
	age       int64
	cats      map[int]*recCat
	particles int
}

type recCat struct {
	cat    int
	stage  *recStage
	traits map[string]*recTrait
}

type recTrait struct {
	trait string
	cat   *recCat
	freq  map[int]float64
	count map[int]int
}

var headerFields = []string{
	"tree",
	"node",
	"age",
	"cat",
	"equator",
	"to",
}

func readRecPixels(r io.Reader, tc *timetree.Collection, tp *model.TimePix) (map[string]*recTree, error) {
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

		f := "tree"
		tn := strings.Join(strings.Fields(row[fields[f]]), " ")
		if tn == "" {
			continue
		}
		tn = strings.ToLower(tn)
		tv := tc.Tree(tn)
		if tv == nil {
			continue
		}
		t, ok := rt[tn]
		if !ok {
			t = &recTree{
				name:  tn,
				nodes: make(map[int]*recNode),
			}
		}
		rt[tn] = t

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
		cv, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		cat, ok := st.cats[cv]
		if !ok {
			cat = &recCat{
				stage:  st,
				cat:    cv,
				traits: make(map[string]*recTrait),
			}
			st.cats[cv] = cat
		}

		f = "equator"
		eq, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if eq != tp.Pixelation().Equator() {
			return nil, fmt.Errorf("on row %d: field %q: invalid equator value %d", ln, f, eq)
		}

		f = "to"
		tr, px, err := parseTraitPix(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if px >= tp.Pixelation().Len() {
			return nil, fmt.Errorf("on row %d: field %q: invalid pixel value %d", ln, f, px)
		}

		trait, ok := cat.traits[tr]
		if !ok {
			trait = &recTrait{
				trait: tr,
				cat:   cat,
				count: make(map[int]int),
			}
			cat.traits[tr] = trait
		}
		trait.count[px]++
		st.particles++
	}
	if len(rt) == 0 {
		return nil, fmt.Errorf("while reading data: %v", io.EOF)
	}

	return rt, nil
}

func parseTraitPix(tp string) (string, int, error) {
	v := strings.Split(tp, ":")
	if len(v) != 2 {
		return "", 0, fmt.Errorf("invalid trait-pix value: %q", tp)
	}
	px, err := strconv.Atoi(v[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid trait-pix value: %q: %v", tp, err)

	}
	return v[0], px, nil
}

func scale(rt map[string]*recTree) {
	for _, t := range rt {
		for _, n := range t.nodes {
			for _, s := range n.stages {
				scaleStage(s)
			}
		}
	}
}

func scaleStage(s *recStage) {
	p := float64(s.particles)
	for _, c := range s.cats {
		for _, tr := range c.traits {
			tr.freq = make(map[int]float64)
			for px, count := range tr.count {
				tr.freq[px] = float64(count) / p
			}
		}
	}
}

func writeFrequencies(t *recTree, tp *model.TimePix, p string) (err error) {
	name := fmt.Sprintf("%s-freq-%s.tab", outPrefix, t.name)
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
	fmt.Fprintf(w, "# pixel frequencies for tree %q or project %q", t.name, p)
	fmt.Fprintf(w, "# date: %s\n", time.Now().Format(time.RFC3339))

	tsv := csv.NewWriter(w)
	tsv.Comma = '\t'
	tsv.UseCRLF = true

	header := []string{
		"tree",
		"node",
		"age",
		"type",
		"cat",
		"trait",
		"equator",
		"pixel",
		"value",
	}
	if err := tsv.Write(header); err != nil {
		return fmt.Errorf("while writing header on %q: %v", name, err)
	}

	eq := strconv.Itoa(tp.Pixelation().Equator())

	nodes := make([]int, 0, len(t.nodes))
	for id := range t.nodes {
		nodes = append(nodes, id)
	}
	slices.Sort(nodes)
	for _, id := range nodes {
		n := t.nodes[id]
		node := strconv.Itoa(n.id)
		stages := make([]int64, 0, len(n.stages))
		for a := range n.stages {
			stages = append(stages, a)
		}
		slices.Sort(stages)
		for i := len(stages) - 1; i >= 0; i-- {
			s := n.stages[stages[i]]
			age := strconv.FormatInt(s.age, 10)
			cats := make([]int, 0, len(s.cats))
			for c := range s.cats {
				cats = append(cats, c)
			}
			slices.Sort(cats)
			for _, cv := range cats {
				c := s.cats[cv]
				cat := strconv.Itoa(c.cat)
				traits := make([]string, 0, len(c.traits))
				for tr := range c.traits {
					traits = append(traits, tr)
				}
				slices.Sort(traits)
				for _, tr := range traits {
					trait := c.traits[tr]
					pix := make([]int, 0, len(trait.freq))
					for px := range trait.freq {
						pix = append(pix, px)
					}
					slices.Sort(pix)
					for _, px := range pix {
						row := []string{
							t.name,
							node,
							age,
							"freq",
							cat,
							tr,
							eq,
							strconv.Itoa(px),
							strconv.FormatFloat(trait.freq[px], 'f', 6, 64),
						}
						if err := tsv.Write(row); err != nil {
							return fmt.Errorf("while writing data on %q: %v", name, err)
						}
					}
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
