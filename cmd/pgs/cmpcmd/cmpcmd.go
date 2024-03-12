// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package cmpcmd implements a command to compare
// two reconstructions.
package cmpcmd

import (
	"cmp"
	"encoding/csv"
	"errors"
	"fmt"
	"image/color"
	"io"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/js-arias/command"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/timetree"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
)

var Command = &command.Command{
	Usage: `cmp --got <file> --want <file>
	--trees <file> [-o|--output <file>]
	[--plot <file>]
	[--bound <value>]
	<project>`,
	Short: "compare two reconstructions",
	Long: `
Command cmp read two reconstructions and report the number of pixels in a
reconstruction that are present in a reference reconstruction.

The flag --got is required and indicates the file that contains the pixels to
be evaluated. The flag --want is required and indicates the file that contains
the reference reconstruction.

The flag --trees, is required and defines the file with the simulated trees.

The flag --output, or -o, defines the name of the file with the amount of
shared pixels. If no name is given, it will use '<project>-pixel-results.tab'.

By default, when reading a KDE reconstruction, it will only map the pixels in
the 0.95 of the CDF. Use the flag --bound to change this bound value.

The argument of the command is the name of the project file.

The comparison is restricted to cladogenetic (or split) nodes. Intermediate
nodes (i.e., nodes inserted when branches pass several time stages), as well
as terminal nodes, are ignored.

If the flag --plot is defined, a plot with the proportion of nodes in which
the number of correct pixels is greater than the 45%, will be saved in the
indicated file.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var gotFile string
var wantFile string
var treeFile string
var output string
var plotFile string
var bound float64

func setFlags(c *command.Command) {
	c.Flags().StringVar(&gotFile, "got", "", "")
	c.Flags().StringVar(&wantFile, "want", "", "")
	c.Flags().StringVar(&treeFile, "trees", "", "")
	c.Flags().StringVar(&output, "output", "", "")
	c.Flags().StringVar(&output, "o", "", "")
	c.Flags().StringVar(&plotFile, "plot", "", "")
	c.Flags().Float64Var(&bound, "bound", 0.95, "")
}

func run(c *command.Command, args []string) (err error) {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}
	if gotFile == "" {
		return c.UsageError("expecting input file, flag --got")
	}
	if wantFile == "" {
		return c.UsageError("expecting want file, flag --want")
	}
	if treeFile == "" {
		return c.UsageError("expecting tree file prefix, flag --trees")
	}

	if output == "" {
		output = fmt.Sprintf("%s-pixel-results.tab", args[0])
	}

	p, err := project.Read(args[0])
	if err != nil {
		return err
	}

	tc, err := readTreeFile()
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

	got, err := readRecon(gotFile, landscape, tc)
	if err != nil {
		return err
	}
	want, err := readRecon(wantFile, landscape, tc)
	if err != nil {
		return err
	}

	f, err := os.Create(output)
	if err != nil {
		return err
	}
	defer func() {
		e := f.Close()
		if e != nil && err == nil {
			err = e
		}
	}()

	freq := make(map[string][]int, len(got))

	date := time.Now().Format(time.RFC3339)
	fmt.Fprintf(f, "# results from simulated data from project %q\n", args[0])
	fmt.Fprintf(f, "# date: %s\n", date)
	fmt.Fprintf(f, "tree\tnode\tage\tpixels\n")
	for _, tn := range tc.Names() {
		gt, ok := got[tn]
		if !ok {
			continue
		}
		wt, ok := want[tn]
		if !ok {
			continue
		}

		fv := make([]int, 11)

		nodes := make([]int, 0, len(wt.nodes))
		for _, n := range wt.nodes {
			nodes = append(nodes, n.id)
		}
		slices.Sort(nodes)

		for _, id := range nodes {
			gn, ok := gt.nodes[id]
			if !ok {
				continue
			}
			wn, ok := wt.nodes[id]
			if !ok {
				continue
			}

			ages := make([]int64, 0, len(wn.stages))
			for _, st := range wn.stages {
				ages = append(ages, st.age)
			}
			slices.Sort(ages)

			for _, a := range ages {
				gs, ok := gn.stages[a]
				if !ok {
					continue
				}
				ws, ok := wn.stages[a]
				if !ok {
					continue
				}

				var sum, scale float64
				for px := range ws.rec {
					sum += gs.rec[px]
				}
				for _, v := range gs.rec {
					scale += v
				}

				i := int(math.Round(sum * 10 / scale))
				fv[i]++

				fmt.Fprintf(f, "%s\t%d\t%d\t%.6f\n", tn, id, a, sum/scale)
			}
		}
		freq[tn] = fv
	}

	if plotFile != "" {
		if err := makePlot(freq); err != nil {
			return err
		}
	}

	return nil
}

func readTreeFile() (*timetree.Collection, error) {
	f, err := os.Open(treeFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	c, err := timetree.ReadTSV(f)
	if err != nil {
		return nil, fmt.Errorf("while reading file %q: %v", treeFile, err)
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

var headerFields = []string{
	"tree",
	"node",
	"age",
	"type",
	"equator",
	"pixel",
	"value",
}

func readRecon(name string, landscape *model.TimePix, coll *timetree.Collection) (map[string]*recTree, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tsv := csv.NewReader(f)
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

	var tp string
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

		tt := coll.Tree(tn)
		if tt == nil {
			continue
		}
		tn = tt.Name()

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

		f = "age"
		age, err := strconv.ParseInt(row[fields[f]], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}

		if tt.IsTerm(id) {
			continue
		}
		if tt.Age(id) != age {
			continue
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
			return nil, fmt.Errorf("on row %d: field %q: expecting reconstruction type", ln, f)
		}
		if tp == "" {
			tp = tpV
		}
		if tp != tpV {
			return nil, fmt.Errorf("on row %d: field %q: got %q want %q", ln, f, tpV, tp)
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
		if tp == "kde" && v < 1-bound {
			continue
		}
		st.rec[px] = v
	}
	if len(rt) == 0 {
		return nil, fmt.Errorf("while reading data: %v", io.EOF)
	}

	if tp == "freq" {
		// scale frequencies
		for _, t := range rt {
			for _, n := range t.nodes {
				for _, s := range n.stages {
					var sum float64
					for _, p := range s.rec {
						sum += p
					}
					for px, p := range s.rec {
						s.rec[px] = p / sum
					}
				}
			}
		}
	}

	return rt, nil
}

func makePlot(freq map[string][]int) error {
	p := plot.New()
	p.Y.Label.Text = "nodes (proportion)"

	w := vg.Points(3)
	sum := make(map[string]int, len(freq))
	names := make([]string, 0, len(freq))
	for n, f := range freq {
		s := 0
		for _, v := range f {
			s += v
		}
		sum[n] = s
		names = append(names, n)
	}
	slices.SortFunc(names, func(a, b string) int {
		fA := freq[a]
		fB := freq[b]

		var xA, xB float64
		for i := 10; i >= 5; i-- {
			xA += float64(fA[i])
			xB += float64(fB[i])
		}

		return cmp.Compare(xA/float64(sum[a]), xB/float64(sum[b]))
	})

	grayScale := []uint8{
		255, // 0
		255, // 1
		250, // 2
		245, // 3
		240, // 4
		200, // 5
		160, // 6
		120, // 7
		80,  // 8
		40,  // 9
		0,   // 10
	}

	var prev *plotter.BarChart
	for i := 10; i >= 5; i-- {
		var vals plotter.Values

		for _, n := range names {
			f := freq[n]
			vals = append(vals, float64(f[i])/float64(sum[n]))
		}

		bars, err := plotter.NewBarChart(vals, w)
		if err != nil {
			return fmt.Errorf("while building chart: %v", err)
		}
		bars.LineStyle.Width = vg.Length(0)
		bars.Color = color.Gray{grayScale[i]}

		if prev != nil {
			bars.StackOn(prev)
		}
		p.Add(bars)
		prev = bars
	}

	if err := p.Save(5*vg.Inch, 3*vg.Inch, plotFile); err != nil {
		return err
	}
	return nil
}
