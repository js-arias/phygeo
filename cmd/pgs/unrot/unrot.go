// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package unrot implements a command to rotate a reconstruction
// from past to present coordinates.
package unrot

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
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/phygeo/project"
)

var Command = &command.Command{
	Usage: `unrot -i|--input <file> [-o|--output <file>]
	<project>`,
	Short: "rotate a reconstruction to present coordinates",
	Long: `
Command unrot reads a reconstruction and rotates the pixels to the present
coordinates.

The flag --input, or -i, is required and indicates the input reconstruction.

By default, the output file will have the same name as the input, with the
prefix "unrot-". The flag --output, or -o, can be used to define a particular
name for the output file.

The argument of the command is the name of the project file. The project must
contain a plate motion model.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var input string
var output string

func setFlags(c *command.Command) {
	c.Flags().StringVar(&input, "input", "", "")
	c.Flags().StringVar(&input, "i", "", "")
	c.Flags().StringVar(&output, "output", "", "")
	c.Flags().StringVar(&output, "o", "", "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}
	pFile := args[0]
	p, err := project.Read(pFile)
	if err != nil {
		return err
	}

	if input == "" {
		return c.UsageError("expecting input file, flag --input")
	}
	if output == "" {
		output = "unrot-" + input
	}

	rotF := p.Path(project.GeoMotion)
	if rotF == "" {
		msg := fmt.Sprintf("plate motion model not defined in project %q", args[0])
		return c.UsageError(msg)
	}
	tot, err := readRotation(rotF)
	if err != nil {
		return err
	}

	rec, tp, err := readRecon(input, tot.Pixelation())
	if err != nil {
		return err
	}

	// make rotation
	for _, t := range rec {
		for _, n := range t.nodes {
			for _, s := range n.stages {
				s.rotate(tot)
			}
		}
	}

	if err := writeFrequencies(rec, output, args[0], tp, tot.Pixelation().Len(), tot.Pixelation().Equator()); err != nil {
		return err
	}

	return nil
}

func readRotation(name string) (*model.Total, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rot, err := model.ReadTotal(f, nil, true)
	if err != nil {
		return nil, fmt.Errorf("on file %q: %v", name, err)
	}

	return rot, nil
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
	node *recNode
	age  int64
	rec  map[int]float64
}

func (r *recStage) rotate(tot *model.Total) {
	rot := tot.Rotation(r.age)

	nr := make(map[int]float64, len(r.rec))
	for px, v := range r.rec {
		dst := rot[px]
		for _, np := range dst {
			nr[np] = v
		}
	}
	r.rec = nr
}

var headerFields = []string{
	"tree",
	"node",
	"age",
	"type",
	"equator",
	"pixel",
	"value",
}

func readRecon(name string, pix *earth.Pixelation) (map[string]*recTree, string, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()

	tsv := csv.NewReader(f)
	tsv.Comma = '\t'
	tsv.Comment = '#'

	head, err := tsv.Read()
	if err != nil {
		return nil, "", fmt.Errorf("while reading header: %v", err)
	}
	fields := make(map[string]int, len(head))
	for i, h := range head {
		h = strings.ToLower(h)
		fields[h] = i
	}
	for _, h := range headerFields {
		if _, ok := fields[h]; !ok {
			return nil, "", fmt.Errorf("expecting field %q", h)
		}
	}

	var tp string
	rt := make(map[string]*recTree)
	for {
		row, err := tsv.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		ln, _ := tsv.FieldPos(0)
		if err != nil {
			return nil, "", fmt.Errorf("on row %d: %v", ln, err)
		}

		f := "tree"
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
			return nil, "", fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}

		f = "age"
		age, err := strconv.ParseInt(row[fields[f]], 10, 64)
		if err != nil {
			return nil, "", fmt.Errorf("on row %d: field %q: %v", ln, f, err)
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

		st, ok := n.stages[age]
		if !ok {
			st = &recStage{
				node: n,
				age:  age,
				rec:  make(map[int]float64),
			}
			n.stages[age] = st
		}

		f = "type"
		tpV := strings.ToLower(strings.Join(strings.Fields(row[fields[f]]), " "))
		if tpV == "" {
			return nil, "", fmt.Errorf("on row %d: field %q: expecting reconstruction type", ln, f)
		}
		if tp == "" {
			tp = tpV
		}
		if tp != tpV {
			return nil, "", fmt.Errorf("on row %d: field %q: got %q want %q", ln, f, tpV, tp)
		}

		f = "equator"
		eq, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, "", fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if eq != pix.Equator() {
			return nil, "", fmt.Errorf("on row %d: field %q: invalid equator value %d", ln, f, eq)
		}

		f = "pixel"
		px, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, "", fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if px >= pix.Len() {
			return nil, "", fmt.Errorf("on row %d: field %q: invalid pixel value %d", ln, f, px)
		}

		f = "value"
		v, err := strconv.ParseFloat(row[fields[f]], 64)
		if err != nil {
			return nil, "", fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		st.rec[px] = v
	}
	if len(rt) == 0 {
		return nil, "", fmt.Errorf("while reading data: %v", io.EOF)
	}

	return rt, tp, nil
}

func writeFrequencies(rt map[string]*recTree, name, p, tp string, numPix, eq int) (err error) {
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
	fmt.Fprintf(w, "# pgs.freq, project %q\n", p)
	fmt.Fprintf(w, "# rotated pixels\n")
	if tp == "kde" {
		fmt.Fprintf(w, "# KDE smoothing\n")
	}
	fmt.Fprintf(w, "# date: %s\n", time.Now().Format(time.RFC3339))

	tsv := csv.NewWriter(w)
	tsv.Comma = '\t'
	tsv.UseCRLF = true
	if err := tsv.Write([]string{"tree", "node", "age", "type", "equator", "pixel", "value"}); err != nil {
		return err
	}

	trees := make([]string, 0, len(rt))
	for tn := range rt {
		trees = append(trees, tn)
	}
	slices.Sort(trees)

	for _, tn := range trees {
		t := rt[tn]
		nodes := make([]int, 0, len(t.nodes))
		for id := range t.nodes {
			nodes = append(nodes, id)
		}
		slices.Sort(nodes)
		for _, id := range nodes {
			n := t.nodes[id]
			stages := make([]int64, 0, len(n.stages))
			for a := range n.stages {
				stages = append(stages, a)
			}
			slices.Sort(stages)

			for i := len(stages) - 1; i >= 0; i-- {
				s := n.stages[stages[i]]
				for px := 0; px < numPix; px++ {
					f, ok := s.rec[px]
					if !ok {
						continue
					}
					if f <= 1e-15 {
						continue
					}
					row := []string{
						t.name,
						strconv.Itoa(n.id),
						strconv.FormatInt(s.age, 10),
						tp,
						strconv.Itoa(eq),
						strconv.Itoa(px),
						strconv.FormatFloat(f, 'f', 15, 64),
					}
					if err := tsv.Write(row); err != nil {
						return err
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
