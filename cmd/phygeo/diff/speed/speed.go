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
	"math"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/stat/dist"
	"github.com/js-arias/phygeo/probmap"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/phygeo/timestage"
	"github.com/js-arias/timetree"
	"gonum.org/v1/gonum/stat"
)

var Command = &command.Command{
	Usage: `speed 
	[--tree <file-prefix>]
	[--step <number>] [--scale <value>]
	[--color <color-scale>] [--width <value>]
	[--box <number>] [--tick <tick-value>]
	[--time] [--plot <file-prefix>]
	[--null <number>]
	-i|--input <file> <project-file>`,
	Short: "calculates speed and distance for a reconstruction",
	Long: `
Command speed reads a file with a sampled pixels from stochastic mapping of
one or more trees in a project, and calculates the distance and speed of the
reconstructed histories.

The distance is a 'biological' distance, in the sense that the distance is the
product of the diffusion process. It is calculated using the great circle
distances between the beginning and ending pixel on each time segment in a
branch.

To test if particles move faster or slower than expected, a simulation is made
with the lambda value used for stochastic sampling and the branch segments of
each lineage. Then it reports the fraction of particles that move more than
95% of the simulations (i.e., they are faster) and the fraction of particles
that move less than 5% of the simulations (i.e., they are slowest). By
default, the number of simulations is 1000; this can be changed with the flag
--null.

The argument of the command is the name of the project file.

The flag --input, or -i, is required and indicates the input file.

If the flag --tree is defined with a file prefix, each tree will be saved as
SVG with each branch colored by the speed of the branch in a red(=fast)-green-
blue(=slow), scale. The scale was made using the log10 of the speed in
kilometers per million year. If the speed of the branch is zero, the minimum
value will used for the branch. The tree will be stored using the indicated
file prefix and the tree name. By default, the time scale is set in million
years. To change the time scale, use the flag --scale with the value in years
of the scale. By default, 10 pixels units will be used per units of the time
scale, use the flag --step to define a different value (it can have decimal
points). The flag --box defines shaded boxes each indicated time steps. The
size of the box is in time scale units. By default, a timescale with ticks
every time scale unit will be added at the bottom of the drawing. Use the flag
--tick to define the tick lines, using the following format:
"<min-tick>,<max-tick>,<label-tick>", in which min-tick indicates minor ticks,
max-tick indicates major ticks, and label-tick the ticks that will be labeled;
for example, the default is "1,5,5" which means that small ticks will be added
each time scale units, major ticks will be added every 5 time scale units, and
labels will be added every 5 time scale units. By default, a rainbow color
scale will be used, other color scales can be defined using the --scale flag.
Valid scale values are mostly based on Paul Tol color scales:

	- iridescent  <https://personal.sron.nl/~pault/#fig:scheme_iridescent>
	- rainbow     default value (from purple to red)
	        <https://personal.sron.nl/~pault/#fig:scheme_rainbow_smooth>
	- incandescent
		<https://personal.sron.nl/~pault/#fig:scheme_incandescent>
	- gray         a gray scale from black to mid gray (RGB: 127).
	- gray2        a gray scale from black to light gray (RBG: 200).

By default, the tree branches will be draw with a 4 pixels, to change the
width use the flag --width.	

The output will be printed in the standard output, as a Tab-delimited table
with the following columns:

	tree      the name of the tree
	node      the ID of the node in the tree
	distance  the median of the traveled distance in kilometers
	d-025     the 2.5% of the empirical CDF of the distance in Km
	d-975     the 97.5% of the empirical CDF of the distance in Km
	dist-rad  the median of the traveled distance in radians
	dr-025    the 2.5% of the empirical CDF of the distance in radians
	dr-975    the 97.5% of the empirical CDF of the distance in radians
	brLen     the length of the branch in million years
	x-050     the 5% of the distance for simulated CDF in kilometers
	x-950     the 95% of the distance for simulated CDF in kilometers
	slower    fraction of particles slower than the 5% of the simulations
	faster    fraction of particles faster than the 95% of the simulations
	speed     the median of the speed in kilometers per million year
	speed-rad the median of the speed in radians per million year

If the flag --time is used, instead of calculating the speed per branch, the
speed will be calculated for each time slice. In this case the whole traveled
distance of each branch segment that pass trough a time slice will be divided
by the total length of all branch segments. The output file will be a
tab-delimited file with the following columns:

	tree      the name of the tree
	age       age of the time slice
	distance  the median of the traveled distance in kilometers
	d-025     the 2.5% of the empirical CDF
	d-975     the 97.5% of the empirical CDF
	brLen     the length of the branch in million years
	speed     the median of the speed in kilometers per million year

If the flag --plot is defined with a file prefix, a box plot for each tree
will be produced, using the speed of each time segment.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var useTime bool
var stepX float64
var timeBox float64
var scale float64
var widthFlag float64
var nullFlag int
var treePrefix string
var inputFile string
var plotPrefix string
var tickFlag string
var colorScale string

func setFlags(c *command.Command) {
	c.Flags().BoolVar(&useTime, "time", false, "")
	c.Flags().Float64Var(&stepX, "step", 10, "")
	c.Flags().Float64Var(&timeBox, "box", 0, "")
	c.Flags().Float64Var(&scale, "scale", timestage.MillionYears, "")
	c.Flags().Float64Var(&widthFlag, "width", 4, "")
	c.Flags().IntVar(&nullFlag, "null", 1000, "")
	c.Flags().StringVar(&inputFile, "input", "", "")
	c.Flags().StringVar(&inputFile, "i", "", "")
	c.Flags().StringVar(&treePrefix, "tree", "", "")
	c.Flags().StringVar(&plotPrefix, "plot", "", "")
	c.Flags().StringVar(&tickFlag, "tick", "", "")
	c.Flags().StringVar(&colorScale, "color", "rainbow", "")
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

	if useTime {
		stages, err := readStages(p, landscape)
		if err != nil {
			return err
		}

		tSlice, err := getTimeSlice(inputFile, tc, landscape, stages)
		if err != nil {
			return err
		}

		if err := writeTimeSlice(c.Stdout(), tSlice); err != nil {
			return err
		}

		if plotPrefix != "" {
			for _, name := range tc.Names() {
				t := tc.Tree(name)
				dt, ok := tSlice[name]
				if !ok {
					continue
				}
				if err := timeSpeedPlot(t, dt); err != nil {
					continue
				}
			}
		}
		return nil
	}

	tBranch, err := getBranches(inputFile, tc, landscape)
	if err != nil {
		return err
	}

	var gradient probmap.Gradienter
	switch strings.ToLower(colorScale) {
	case "gray":
		gradient = probmap.HalfGrayScale{}
	case "gray2":
		gradient = probmap.LightGrayScale{}
	case "rainbow":
		gradient = probmap.RainbowPurpleToRed{}
	case "incandescent":
		gradient = probmap.Incandescent{}
	case "iridescent":
		gradient = probmap.Iridescent{}
	default:
		gradient = probmap.RainbowPurpleToRed{}
	}

	// make the simulations
	tSim := make(map[string]*recTree, len(tBranch))
	for _, name := range tc.Names() {
		dt, ok := tBranch[name]
		if !ok {
			continue
		}

		t := tc.Tree(name)
		tSim[name] = nullRec(landscape.Pixelation(), dt, t.Root())
	}

	if err := writeRecBranch(c.Stdout(), tc, tBranch, tSim); err != nil {
		return err
	}

	if treePrefix != "" {
		if err := plotTrees(tc, tBranch, gradient); err != nil {
			return err
		}
	}

	return nil
}

func readStages(p *project.Project, landscape *model.TimePix) (timestage.Stages, error) {
	rot, err := p.StageRotation(landscape.Pixelation())
	if err != nil {
		return nil, err
	}

	stages, err := p.Stages(landscape, rot)
	if err != nil {
		return nil, err
	}
	return stages, nil
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
	name   string
	nodes  map[int]*recNode
	lambda float64
}

type recNode struct {
	id   int
	tree *recTree
	recs map[int]*recBranch
	ages map[int64]bool
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
	"lambda",
	"to",
}

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
				ages: make(map[int64]bool),
			}
			t.nodes[id] = n
			if !tv.IsRoot(id) {
				n.ages[tv.Age(tv.Parent(id))] = true
			}
		}
		if tv.IsRoot(id) {
			continue
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
		n.ages[age] = true

		f = "lambda"
		lambda, err := strconv.ParseFloat(row[fields[f]], 64)
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		t.lambda = lambda

		// add to the whole tree reconstruction
		root := t.nodes[tv.Root()]
		p, ok = root.recs[pN]
		if !ok {
			p = &recBranch{
				id:   pN,
				node: root,
			}
			root.recs[pN] = p
		}
		p.dist += dist
	}

	if len(rt) == 0 {
		return nil, fmt.Errorf("while reading data: %v", io.EOF)
	}

	return rt, nil
}

func nullRec(pix *earth.Pixelation, t *recTree, root int) *recTree {
	st := &recTree{
		name:   t.name,
		lambda: t.lambda,
		nodes:  make(map[int]*recNode, len(t.nodes)),
	}
	for id, n := range t.nodes {
		sn := &recNode{
			id:   id,
			tree: st,
			recs: make(map[int]*recBranch, nullFlag),
		}
		st.nodes[id] = sn
		if root == id {
			for i := 0; i < nullFlag; i++ {
				sn.recs[i] = &recBranch{
					id:   i,
					node: sn,
				}
			}
			continue
		}
		ages := make([]float64, 0, len(n.ages))
		for a := range n.ages {
			ages = append(ages, float64(a)/timestage.MillionYears)
		}
		slices.Sort(ages)

		PDFs := make([]dist.Normal, 0, len(ages)-1)
		for i, a := range ages {
			if i == 0 {
				continue
			}
			brLen := a - ages[i-1]
			PDFs = append(PDFs, dist.NewNormal(st.lambda/brLen, pix))
		}
		for i := 0; i < nullFlag; i++ {
			var sum float64
			px := pix.ID(0)
			for _, p := range PDFs {
				nx := p.Rand(px)
				sum += earth.Distance(px.Point(), nx.Point())
				px = nx
			}
			sn.recs[i] = &recBranch{
				id:   i,
				node: sn,
				dist: sum,
			}
		}
	}

	// distances for the whole tree
	rn := st.nodes[root]
	for i, r := range rn.recs {
		for id, n := range st.nodes {
			if id == root {
				continue
			}
			r.dist += n.recs[i].dist
		}
	}

	return st
}

func writeRecBranch(w io.Writer, tc *timetree.Collection, rt, rSim map[string]*recTree) error {
	tab := csv.NewWriter(w)
	tab.Comma = '\t'
	tab.UseCRLF = true

	if err := tab.Write([]string{"tree", "node", "distance", "d-025", "d-975", "dist-rad", "dr-025", "dr-975", "brLen", "x-005", "x-095", "slower", "faster", "speed", "speed-rad"}); err != nil {
		return err
	}
	for _, name := range tc.Names() {
		dt, ok := rt[name]
		if !ok {
			continue
		}
		t := tc.Tree(name)
		st := rSim[name]

		for _, nID := range t.Nodes() {
			n := dt.nodes[nID]
			dist := make([]float64, 0, len(n.recs))
			weights := make([]float64, 0, len(n.recs))
			for _, r := range n.recs {
				dist = append(dist, r.dist)
				weights = append(weights, 1.0)
			}
			slices.Sort(dist)

			brLen := float64(t.Len()) / timestage.MillionYears
			pN := t.Parent(nID)
			if pN >= 0 {
				brLen = float64(t.Age(pN)-t.Age(nID)) / timestage.MillionYears
			}

			dR := stat.Quantile(0.5, stat.Empirical, dist, weights)
			d := dR * earth.Radius / 1000
			sR := dR / brLen
			s := d / brLen

			sn := st.nodes[nID]
			nullDist := make([]float64, 0, len(sn.recs))
			nullWeights := make([]float64, 0, len(sn.recs))
			for _, r := range sn.recs {
				nullDist = append(nullDist, r.dist*earth.Radius/1000)
				nullWeights = append(nullWeights, 1.0)
			}
			slices.Sort(nullDist)
			n05 := stat.Quantile(0.05, stat.Empirical, nullDist, nullWeights)
			n95 := stat.Quantile(0.95, stat.Empirical, nullDist, nullWeights)
			var fast, slow int
			for _, od := range dist {
				od *= earth.Radius / 1000
				if od > n95 {
					fast++
				}
				if od < n05 {
					slow++
				}
			}

			row := []string{
				name,
				strconv.Itoa(nID),
				strconv.FormatFloat(d, 'f', 3, 64),
				strconv.FormatFloat(stat.Quantile(0.025, stat.Empirical, dist, weights)*earth.Radius/1000, 'f', 3, 64),
				strconv.FormatFloat(stat.Quantile(0.975, stat.Empirical, dist, weights)*earth.Radius/1000, 'f', 3, 64),
				strconv.FormatFloat(dR, 'f', 3, 64),
				strconv.FormatFloat(stat.Quantile(0.025, stat.Empirical, dist, weights), 'f', 3, 64),
				strconv.FormatFloat(stat.Quantile(0.975, stat.Empirical, dist, weights), 'f', 3, 64),
				strconv.FormatFloat(brLen, 'f', 3, 64),
				strconv.FormatFloat(n05, 'f', 3, 64),
				strconv.FormatFloat(n95, 'f', 3, 64),
				strconv.FormatFloat(float64(slow)/float64(len(dist)), 'f', 3, 64),
				strconv.FormatFloat(float64(fast)/float64(len(dist)), 'f', 3, 64),
				strconv.FormatFloat(s, 'f', 3, 64),
				strconv.FormatFloat(sR, 'f', 3, 64),
			}
			if nID == 0 {
				// root node is the whole tree
				row[1] = "--"
			}
			if err := tab.Write(row); err != nil {
				return err
			}
		}
	}

	tab.Flush()
	if err := tab.Error(); err != nil {
		return err
	}
	return nil
}

func plotTrees(tc *timetree.Collection, rt map[string]*recTree, gradient probmap.Gradienter) error {
	tv, err := parseTick()
	if err != nil {
		return err
	}

	for _, name := range tc.Names() {
		rec, ok := rt[name]
		if !ok {
			continue
		}

		t := tc.Tree(name)
		st := copyTree(t, stepX, tv.min, tv.max, tv.label)

		sp := make(map[int]float64)
		var avg float64
		min := math.MaxFloat64
		max := math.SmallestNonzeroFloat64
		for _, nID := range t.Nodes() {
			n := rec.nodes[nID]
			dist := make([]float64, 0, len(n.recs))
			weights := make([]float64, 0, len(n.recs))
			for _, r := range n.recs {
				dist = append(dist, r.dist*earth.Radius/1000)
				weights = append(weights, 1.0)
			}
			slices.Sort(dist)

			// root node
			pN := t.Parent(nID)
			if pN < 0 {
				brLen := float64(t.Len()) / timestage.MillionYears
				d := stat.Quantile(0.5, stat.Empirical, dist, weights)
				avg = math.Log10(d / brLen)
				continue
			}

			brLen := float64(t.Age(pN)-t.Age(nID)) / timestage.MillionYears
			d := stat.Quantile(0.5, stat.Empirical, dist, weights)
			s := math.Log10(d / brLen)

			if s < 0 {
				continue
			}
			if s > max {
				max = s
			}
			if s < min {
				min = s
			}
			sp[nID] = s
		}
		st.setColor(sp, min, max, avg, gradient)

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

type tickValues struct {
	min   int
	max   int
	label int
}

func parseTick() (tickValues, error) {
	if tickFlag == "" {
		return tickValues{
			min:   1,
			max:   5,
			label: 5,
		}, nil
	}

	vals := strings.Split(tickFlag, ",")
	if len(vals) != 3 {
		return tickValues{}, fmt.Errorf("invalid tick values: %q", tickFlag)
	}

	min, err := strconv.Atoi(vals[0])
	if err != nil {
		return tickValues{}, fmt.Errorf("invalid minor tick value: %q: %v", tickFlag, err)
	}

	max, err := strconv.Atoi(vals[1])
	if err != nil {
		return tickValues{}, fmt.Errorf("invalid major tick value: %q: %v", tickFlag, err)
	}

	label, err := strconv.Atoi(vals[2])
	if err != nil {
		return tickValues{}, fmt.Errorf("invalid label tick value: %q: %v", tickFlag, err)
	}

	return tickValues{
		min:   min,
		max:   max,
		label: label,
	}, nil
}
