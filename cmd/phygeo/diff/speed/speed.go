// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package speed implements a command to measure
// the speed and distance traveled in a reconstruction.
package speed

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/timetree"
	"golang.org/x/exp/slices"
)

var Command = &command.Command{
	Usage: `speed [--time]
	[--tree <file-prefix>] [--step <number>] [--box <number>]
	-i|--input <file> [-o|--output <file>] <project-file>`,
	Short: "calculates speed and distance for a reconstruction",
	Long: `
Command speed reads a file with a range reconstruction for the nodes of one or
more trees in a project, and calculates the distance and speed of the
reconstructed particles.

The distance is a 'biological' distance, in the sense that it is the distance
product of the diffusion process. It is calculated using the great circle
distances between the beginning and ending pixel on a each branch segment of a
branch. A geo-distance indicates the distance between the beginning and ending
pixel of the whole branch, so both biological and tectonic movement is taken
into account.

The argument of the command is the name of the project file.

The flag --input, or -i, is required and indicates the input file.

By default the output will be printed in the standard output. Use the flag
--output, or -o, to define an output file.

The default output reports the speed and distance for each branch in each
particle using a tab-delimited format with the following columns:

	tree	 the name of the tree
	particle the number of the particle
	node	 the ID of the node in the tree
	speed	 the speed in radians per million years on the branch
	distance the traveled distance in radians on the branch
	geo-dist the distance in radians between the ancestor and the end
		point of the branch

If the flag --tree is defined with a file prefix. Each tree will be saved as
SVG with each branch colored by the speed of the branch in a red(=fast)-green-
blue(=slow), scale. The scale was made using the log10 of the speed in
kilometers per million year. The tree will be stored using the indicated file
prefix and the tree name. By default, 10 pixels units will be used per million
year, use the flag --step to define a different value (it can have decimal
points). The flag --box defines shaded boxes each indicated time steps. The
size of the box is in million years.

IF the flag --time is used, instead of calculating the speed per branch, the
speed will be calculated for each time slice. In this case the speed of each
branch segment that pass trough a time slice will be averaged. In the case of
splits or terminals that became extinct in the time slice, they will be
counted as lineages in the time slice. The output file will be a tab-delimited
file with the following columns:

	tree	  the name of the tree
	particle  the number of the particle
	age	  the age of the time slice
	avg-speed the average speed in the time slice in radians per million
		years
	lineages  the number of lineages that are included in the time slice
	`,
	SetFlags: setFlags,
	Run:      run,
}

var useTime bool
var stepX float64
var timeBox float64
var treePrefix string
var inputFile string
var outputFile string

func setFlags(c *command.Command) {
	c.Flags().BoolVar(&useTime, "time", false, "")
	c.Flags().Float64Var(&stepX, "step", 10, "")
	c.Flags().Float64Var(&timeBox, "box", 0, "")
	c.Flags().StringVar(&inputFile, "input", "", "")
	c.Flags().StringVar(&inputFile, "i", "", "")
	c.Flags().StringVar(&outputFile, "output", "", "")
	c.Flags().StringVar(&outputFile, "o", "", "")
	c.Flags().StringVar(&treePrefix, "tree", "", "")
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

	tf := p.Path(project.Trees)
	if tf == "" {
		msg := fmt.Sprintf("tree file not defined in project %q", args[0])
		return c.UsageError(msg)
	}
	tc, err := readTreeFile(tf)
	if err != nil {
		return err
	}

	if useTime {
		tSlice, err := getTimeSlice(inputFile, tc, landscape)
		if err != nil {
			return err
		}

		if err := timeSliceFile(c.Stdout(), tSlice); err != nil {
			return err
		}
		return nil
	}

	tBranch, err := getBranches(inputFile, tc, landscape)
	if err != nil {
		return err
	}

	if err := recBranchFile(c.Stdout(), tc, tBranch); err != nil {
		return err
	}

	if treePrefix != "" {
		if err := plotTrees(tc, tBranch); err != nil {
			return err
		}
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

func getBranches(name string, tc *timetree.Collection, landscape *model.TimePix) (map[string]*recTree, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rt, err := readRecBranches(f, tc, landscape)
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
	id   int
	tree *recTree
	recs map[int]*recBranch
}

type recBranch struct {
	id    int
	node  *recNode
	dist  float64
	endPt earth.Point
}

var headerFields = []string{
	"tree",
	"particle",
	"node",
	"age",
	"to",
}

const millionYears = 1_000_000

func readRecBranches(r io.Reader, tc *timetree.Collection, tp *model.TimePix) (map[string]*recTree, error) {
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
				id:   id,
				tree: t,
				recs: make(map[int]*recBranch),
			}
			t.nodes[id] = n
		}

		f = "particle"
		pN, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		p, ok := n.recs[pN]
		if !ok {
			p = &recBranch{
				id:   pN,
				node: n,
			}
			n.recs[pN] = p
		}

		f = "from"
		fPx, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if fPx >= tp.Pixelation().Len() {
			return nil, fmt.Errorf("on row %d: field %q: invalid pixel value %d", ln, f, fPx)
		}
		from := tp.Pixelation().ID(fPx).Point()

		f = "to"
		tPx, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if tPx >= tp.Pixelation().Len() {
			return nil, fmt.Errorf("on row %d: field %q: invalid pixel value %d", ln, f, tPx)
		}
		to := tp.Pixelation().ID(tPx).Point()

		dist := earth.Distance(from, to)
		p.dist += dist

		f = "age"
		age, err := strconv.ParseInt(row[fields[f]], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if age == tv.Age(id) {
			p.endPt = to
		}
	}

	if len(rt) == 0 {
		return nil, fmt.Errorf("while reading data: %v", io.EOF)
	}

	return rt, nil
}

func recBranchFile(w io.Writer, tc *timetree.Collection, rt map[string]*recTree) error {
	if outputFile != "" {
		f, err := os.Create(outputFile)
		if err != nil {
			return err
		}
		defer func() {
			e := f.Close()
			if e != nil && err == nil {
				err = e
			}
		}()
		w = f
	} else {
		outputFile = "stdout"
	}

	if err := writeRecBranch(w, tc, rt); err != nil {
		return fmt.Errorf("when writing on file %q: %v", outputFile, err)
	}
	return nil
}

func writeRecBranch(w io.Writer, tc *timetree.Collection, rt map[string]*recTree) error {
	tab := csv.NewWriter(w)
	tab.Comma = '\t'
	tab.UseCRLF = true

	if err := tab.Write([]string{"tree", "particle", "node", "speed", "distance", "geo-dist"}); err != nil {
		return err
	}

	for _, name := range tc.Names() {
		dt, ok := rt[name]
		if !ok {
			continue
		}

		t := tc.Tree(name)
		for _, nID := range t.Nodes() {
			// skip root node
			pN := t.Parent(nID)
			if pN < 0 {
				continue
			}

			n := dt.nodes[nID]
			a := dt.nodes[pN]
			brLen := float64(t.Age(pN)-t.Age(nID)) / millionYears

			ps := make([]int, 0, len(n.recs))
			for p := range n.recs {
				ps = append(ps, p)
			}
			slices.Sort(ps)

			for _, p := range ps {
				nd := n.recs[p]
				sp := nd.dist / brLen
				gd := earth.Distance(a.recs[p].endPt, nd.endPt)

				row := []string{
					name,
					strconv.Itoa(p),
					strconv.Itoa(nID),
					strconv.FormatFloat(sp, 'f', 6, 64),
					strconv.FormatFloat(nd.dist, 'f', 6, 64),
					strconv.FormatFloat(gd, 'f', 6, 64),
				}
				if err := tab.Write(row); err != nil {
					return err
				}
			}
		}
	}

	tab.Flush()
	if err := tab.Error(); err != nil {
		return err
	}
	return nil
}

func plotTrees(tc *timetree.Collection, rt map[string]*recTree) error {
	for _, name := range tc.Names() {
		rec, ok := rt[name]
		if !ok {
			continue
		}

		t := tc.Tree(name)
		st := copyTree(t, stepX)
		st.setColor(t, rec)

		fName := treePrefix + "-" + name + ".svg"
		if err := writeSVGTree(fName, st); err != nil {
			return err
		}
	}
	return nil
}

func writeSVGTree(name string, t svgTree) (err error) {
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

	bw := bufio.NewWriter(f)
	if err := t.draw(bw); err != nil {
		return fmt.Errorf("while writing file %q: %v", name, err)
	}
	if err := bw.Flush(); err != nil {
		return fmt.Errorf("while writing file %q: %v", name, err)
	}
	return nil
}
