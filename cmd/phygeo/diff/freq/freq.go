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
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/js-arias/command"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/stat"
	"github.com/js-arias/earth/stat/dist"
	"github.com/js-arias/earth/stat/pixprob"
	"github.com/js-arias/phygeo/project"
)

var Command = &command.Command{
	Usage: `freq [--kde <value>] [--cpu <number>]
	-i|--input <file> [-o|--output <file>] <project-file>`,
	Short: "calculate pixel frequencies",
	Long: `
Command freq reads a file from a stochastic mapping reconstruction for the
nodes of one or more trees in a project and produces an equivalent file with
pixel frequencies.

The argument of the command is the name of the project file.

The flag --input, or -i, is required and indicates the input file.

By default, the ranges are taken as given. If the flag --kde is defined, a
kernel density estimation using a spherical normal will be used to smooth the
results with the indicated concentration parameter (in 1/radians^2). As
calculating the KDE can be computationally expensive, this procedure is run in
parallel using all available processors. Use the flag --cpu to change the
number of processors.

By default, the output file will have the name of the input file with the
prefix "freq" or "kde" if the --kde flag is used. With the flag --output, or
-o, a different prefix can be defined.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var numCPU int
var kdeLambda float64
var inputFile string
var outPrefix string

func setFlags(c *command.Command) {
	c.Flags().IntVar(&numCPU, "cpu", runtime.GOMAXPROCS(0), "")
	c.Flags().Float64Var(&kdeLambda, "kde", 0, "")
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

	lsf := p.Path(project.Landscape)
	if lsf == "" {
		msg := fmt.Sprintf("landscape not defined in project %q", args[0])
		return c.UsageError(msg)
	}
	landscape, err := readLandscape(lsf)
	if err != nil {
		return err
	}

	rt, err := getRec(inputFile, landscape)
	if err != nil {
		return err
	}

	if outPrefix == "" {
		outPrefix = "freq"
		if kdeLambda > 0 {
			outPrefix = "kde"
		}
	}

	tp := "freq"
	if kdeLambda > 0 {
		var pp pixprob.Pixel
		ppF := p.Path(project.PixPrior)
		if ppF == "" {
			msg := fmt.Sprintf("pixel priors not defined in project %q", args[0])
			return c.UsageError(msg)
		}
		pp, err = readPriors(ppF)
		if err != nil {
			return err
		}

		setKDE(rt, landscape, pp)
		tp = "kde"
	} else {
		scale(rt)
	}

	name := fmt.Sprintf("%s-%s-%s.tab", outPrefix, args[0], inputFile)
	if err := writeFrequencies(rt, name, args[0], tp, landscape.Pixelation().Len()); err != nil {
		return err
	}

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
	rec       map[int]float64
	sum       float64
	landscape *model.TimePix
}

var headerFields = []string{
	"tree",
	"node",
	"age",
	"to",
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
				node:      n,
				age:       age,
				rec:       make(map[int]float64),
				landscape: landscape,
			}
			n.stages[age] = st
		}

		f = "to"
		px, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if px >= landscape.Pixelation().Len() {
			return nil, fmt.Errorf("on row %d: field %q: invalid pixel value %d", ln, f, px)
		}

		st.rec[px]++
		st.sum++
	}
	if len(rt) == 0 {
		return nil, fmt.Errorf("while reading data: %v", io.EOF)
	}

	return rt, nil
}

func scale(rt map[string]*recTree) {
	for _, t := range rt {
		for _, n := range t.nodes {
			for _, s := range n.stages {
				for px, f := range s.rec {
					s.rec[px] = f / s.sum
				}
			}
		}
	}
}

type stageChan struct {
	t   string          // tree ID
	n   int             // node ID
	age int64           // stage age
	rec map[int]float64 // stage reconstruction
}

func makeKDE(in, out chan stageChan, wg *sync.WaitGroup, norm dist.Normal, landscape *model.TimePix, pp pixprob.Pixel) {
	for d := range in {
		rec := stat.KDE(norm, d.rec, landscape, d.age, pp)
		out <- stageChan{
			t:   d.t,
			n:   d.n,
			age: d.age,
			rec: rec,
		}
		wg.Done()
	}
}

func setKDE(rt map[string]*recTree, landscape *model.TimePix, prior pixprob.Pixel) {
	pp := pixprob.New()
	for _, v := range prior.Values() {
		if prior.Prior(v) > 0 {
			pp.Set(v, 1)
		}
	}
	norm := dist.NewNormal(kdeLambda, landscape.Pixelation())

	in := make(chan stageChan, numCPU*2)
	out := make(chan stageChan, numCPU*2)
	var wg sync.WaitGroup
	for i := 0; i < numCPU; i++ {
		go makeKDE(in, out, &wg, norm, landscape, pp)
	}

	go func() {
		// send the reconstructions
		for _, t := range rt {
			for _, n := range t.nodes {
				for _, s := range n.stages {
					wg.Add(1)
					in <- stageChan{
						t:   t.name,
						n:   n.id,
						age: s.age,
						rec: s.rec,
					}
				}
			}
		}
		wg.Wait()
		close(out)
	}()

	for a := range out {
		t := rt[a.t]
		n := t.nodes[a.n]
		s := n.stages[a.age]
		s.rec = a.rec
	}
	close(in)
}

func writeFrequencies(rt map[string]*recTree, name, p, tp string, numPix int) (err error) {
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
	fmt.Fprintf(w, "# diff.freq, project %q\n", p)
	if tp == "kde" {
		fmt.Fprintf(w, "# KDE smoothing: lambda %.6f * 1/radian^2\n", kdeLambda)
	}
	fmt.Fprintf(w, "# date: %s\n", time.Now().Format(time.RFC3339))

	tsv := csv.NewWriter(w)
	tsv.Comma = '\t'
	tsv.UseCRLF = true
	if err := tsv.Write([]string{"tree", "node", "age", "type", "pixel", "value"}); err != nil {
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
