// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package speed implements a command to measure
// the diffusion speed in a reconstruction.
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
	[--particles <number>] [--cats]
	-i|--input <file> <project-file>`,
	Short: "calculates speed for a reconstruction",
	Long: `
Command speed reas a file with the sampled pixels from stochastic mapping of
one or more trees in a project, and calculates the speed of reconstructed
diffusion parameter.

The diffusion speed is 'biological' speed, in the sense that it is the product
of the diffusion process. It is calculated from the diffusion categories used
in the relaxed random walk. The marginal of each category is used to pick a
lambda, with that value, a particle is simulated, and the speed is the
distance moved over a million of years. By default, the simulations will use
1000 particles. Use the flag --particles to change the number of particles.
The simulation assumes an homogeneous landscape.

The argument of the command is the name of the project file.

The flag --input, or -i, is required and indicates the input file.

The output will be printed in the standard output, as a Tab-delimited table
with the following columns:

	tree      the name of the tree
	node      the ID of the node in the tree
	lambda    the overall lambda value
	speed     the average speed in kilometers per million years
	s-05      the 5% of the simulated CDF in Km/My
	s-95      the 95% of the simulated CDF in Km/My

The row marked with '--' for the node, indicates the average speed of the
whole tree taking into account the length of the branches.

If the flag --cats is used, then it will print the marginal distribution of
each diffusion category per node.

	tree      the name of the tree
	node      the ID of the node in the tree
	cat       the category of the relaxed diffusion
	lambda    the lambda value associated with the category
	marginal  the marginal probability of the category

If the flag --tree is defined with a file prefix, and outputs a tree colored
with the speed of each branch. Each tree will be saved as SVG with each branch
colored by the speed of the branch in a red(=fast)-green-blue(=slow), scale.
The scale was made using the log10 of the speed in kilometers per million
year. If the speed of the branch is zero, the minimum value will used for the
branch. If flag --cats is defined, instead of the speeds, it will color the
branched with the category with the greater marginal. The tree will be stored
using the indicated file prefix and the tree name. By default, the time scale
is set in million years. To change the time scale, use the flag --scale with
the value in years of the scale. By default, 10 pixels units will be used per
units of the time scale, use the flag --step to define a different value (it
can have decimal points). The flag --box defines shaded boxes each indicated
time steps. The size of the box is in time scale units. By default, a
timescale with ticks every time scale unit will be added at the bottom of the
drawing. Use the flag --tick to define the tick lines, using the following
format: "<min-tick>,<max-tick>,<label-tick>", in which min-tick indicates
minor ticks, max-tick indicates major ticks, and label-tick the ticks that
will be labeled; for example, the default is "1,5,5" which means that small
ticks will be added each time scale units, major ticks will be added every 5
time scale units, and labels will be added every 5 time scale units. By
default, a rainbow color scale will be used, other color scales can be defined
using the --scale flag. Valid scale values are mostly based on Paul Tol color
scales:

	- iridescent  <https://personal.sron.nl/~pault/#fig:scheme_iridescent>
	- rainbow     default value (from purple to red)
	        <https://personal.sron.nl/~pault/#fig:scheme_rainbow_smooth>
	- incandescent
		<https://personal.sron.nl/~pault/#fig:scheme_incandescent>
	- gray         a gray scale from black to mid gray (RGB: 127).
	- gray2        a gray scale from black to light gray (RBG: 200).

By default, the tree branches will be draw with a 4 pixels, to change the
width use the flag --width.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var useCats bool
var scale float64
var stepX float64
var timeBox float64
var widthFlag float64
var numParticles int
var colorScale string
var inputFile string
var tickFlag string
var treePrefix string

func setFlags(c *command.Command) {
	c.Flags().BoolVar(&useCats, "cats", false, "")
	c.Flags().Float64Var(&scale, "scale", timestage.MillionYears, "")
	c.Flags().Float64Var(&stepX, "step", 10, "")
	c.Flags().Float64Var(&timeBox, "box", 0, "")
	c.Flags().Float64Var(&widthFlag, "width", 4, "")
	c.Flags().IntVar(&numParticles, "particles", 1000, "")
	c.Flags().StringVar(&colorScale, "color", "rainbow", "")
	c.Flags().StringVar(&inputFile, "input", "", "")
	c.Flags().StringVar(&inputFile, "i", "", "")
	c.Flags().StringVar(&tickFlag, "tick", "", "")
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

	landscape, err := p.Landscape(nil)
	if err != nil {
		return err
	}

	tc, err := p.Trees()
	if err != nil {
		return err
	}

	catVals, err := getLambda(inputFile, tc, landscape)
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

	if useCats {
		if err := writeNodeCats(c.Stdout(), tc, catVals); err != nil {
			return err
		}

		if treePrefix != "" {
			if err := plotTrees(tc, catVals, gradient); err != nil {
				return err
			}
		}
		return nil
	}

	calcSpeed(tc, landscape.Pixelation(), catVals)
	if err := writeNodeSpeed(c.Stdout(), tc, catVals); err != nil {
		return err
	}
	if treePrefix != "" {
		if err := plotTrees(tc, catVals, gradient); err != nil {
			return err
		}
	}
	return nil
}

func getLambda(name string, tc *timetree.Collection, landscape *model.TimePix) (map[string]*recTree, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rt, err := readRecCats(f, tc, landscape)
	if err != nil {
		return nil, fmt.Errorf("on input file %q: %v", name, err)
	}
	return rt, nil
}

type recTree struct {
	name   string
	lambda float64
	nodes  map[int]*recNode
	speed  float64
	s05    float64
	s95    float64
}

type recNode struct {
	id        int
	tree      *recTree
	particles int
	cats      map[int]*recCat
	speed     float64
	s05       float64
	s95       float64
}

type recCat struct {
	node   *recNode
	cat    int
	lambda float64
	count  int
}

var headerFields = []string{
	"tree",
	"particle",
	"node",
	"age",
	"lambda",
	"cat",
	"scaled",
	"equator",
}

func readRecCats(r io.Reader, tc *timetree.Collection, tp *model.TimePix) (map[string]*recTree, error) {
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
			f = "lambda"
			lambda, err := strconv.ParseFloat(row[fields[f]], 64)
			if err != nil {
				return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
			}
			t = &recTree{
				name:   tn,
				lambda: lambda,
				nodes:  make(map[int]*recNode),
			}
			rt[tn] = t
		}

		f = "node"
		id, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if tv.IsRoot(id) {
			continue
		}
		n, ok := t.nodes[id]
		if !ok {
			n = &recNode{
				id:   id,
				tree: t,
				cats: make(map[int]*recCat),
			}
			t.nodes[id] = n
		}

		f = "age"
		age, err := strconv.ParseInt(row[fields[f]], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if age != tv.Age(id) {
			continue
		}

		f = "equator"
		eq, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if eq != tp.Pixelation().Equator() {
			return nil, fmt.Errorf("on row %d: field %q: invalid equator value %d", ln, f, eq)
		}

		f = "cat"
		cv, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		cat, ok := n.cats[cv]
		if !ok {
			f = "scaled"
			lambda, err := strconv.ParseFloat(row[fields[f]], 64)
			if err != nil {
				return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
			}
			cat = &recCat{
				node:   n,
				cat:    cv,
				lambda: lambda,
			}
			n.cats[cv] = cat
		}
		cat.count++
		n.particles++
	}
	if len(rt) == 0 {
		return nil, fmt.Errorf("while reading data: %v", io.EOF)
	}

	return rt, nil
}

func calcSpeed(tc *timetree.Collection, pix *earth.Pixelation, rt map[string]*recTree) {
	for _, name := range tc.Names() {
		dt, ok := rt[name]
		if !ok {
			continue
		}
		t := tc.Tree(name)

		// speed of each node
		for _, nID := range t.Nodes() {
			n, ok := dt.nodes[nID]
			if !ok {
				continue
			}
			dist := make([]float64, 0, numParticles)
			for _, c := range n.cats {
				d := c.distance(pix, numParticles)
				dist = append(dist, d...)
			}
			slices.Sort(dist)
			weights := make([]float64, 0, len(dist))
			var sum float64
			for _, d := range dist {
				sum += d
				weights = append(weights, 1.0)
			}
			km := earth.Radius / 1000.0
			n.speed = sum * km / float64(len(dist))
			n.s05 = stat.Quantile(0.05, stat.Empirical, dist, weights) * km
			n.s95 = stat.Quantile(0.95, stat.Empirical, dist, weights) * km
		}

		// speed of the whole tree
		var max int64
		for _, nID := range t.Nodes() {
			if _, ok := dt.nodes[nID]; !ok {
				continue
			}
			if t.IsRoot(nID) {
				continue
			}
			brLen := t.Age(t.Parent(nID)) - t.Age(nID)
			if brLen > max {
				max = brLen
			}
		}
		var dist []float64
		for _, nID := range t.Nodes() {
			n, ok := dt.nodes[nID]
			if !ok {
				continue
			}
			if t.IsRoot(nID) {
				continue
			}
			brLen := t.Age(t.Parent(nID)) - t.Age(nID)
			particles := float64(brLen) * float64(numParticles) / float64(max)
			for _, c := range n.cats {
				d := c.distance(pix, int(particles))
				dist = append(dist, d...)
			}
		}
		slices.Sort(dist)
		weights := make([]float64, 0, len(dist))
		var sum float64
		for _, d := range dist {
			sum += d
			weights = append(weights, 1.0)
		}
		km := earth.Radius / 1000.0
		dt.speed = sum * km / float64(len(dist))
		dt.s05 = stat.Quantile(0.05, stat.Empirical, dist, weights) * km
		dt.s95 = stat.Quantile(0.95, stat.Empirical, dist, weights) * km
	}
}

func (r *recCat) distance(pix *earth.Pixelation, particles int) []float64 {
	prob := float64(r.count) / float64(r.node.particles)
	np := pix.Pixel(90, 0)
	sn := dist.NewNormal(r.lambda, pix)
	parts := int(float64(particles) * prob)
	d := make([]float64, 0, parts)
	for range parts {
		dp := sn.Rand(np)
		dist := earth.Distance(np.Point(), dp.Point())
		d = append(d, dist)
	}
	return d
}

func writeNodeSpeed(w io.Writer, tc *timetree.Collection, rt map[string]*recTree) error {
	tab := csv.NewWriter(w)
	tab.Comma = '\t'
	tab.UseCRLF = true

	if err := tab.Write([]string{"tree", "node", "lambda", "speed", "s-05", "s-95"}); err != nil {
		return err
	}
	for _, name := range tc.Names() {
		dt, ok := rt[name]
		if !ok {
			continue
		}
		t := tc.Tree(name)
		lambda := strconv.FormatFloat(dt.lambda, 'f', 6, 64)

		// speed of the whole tree
		row := []string{
			name,
			"--",
			lambda,
			strconv.FormatFloat(dt.speed, 'f', 6, 64),
			strconv.FormatFloat(dt.s05, 'f', 6, 64),
			strconv.FormatFloat(dt.s95, 'f', 6, 64),
		}
		if err := tab.Write(row); err != nil {
			return err
		}

		for _, nID := range t.Nodes() {
			n, ok := dt.nodes[nID]
			if !ok {
				continue
			}
			nodeID := strconv.Itoa(nID)
			row := []string{
				name,
				nodeID,
				lambda,
				strconv.FormatFloat(n.speed, 'f', 6, 64),
				strconv.FormatFloat(n.s05, 'f', 6, 64),
				strconv.FormatFloat(n.s95, 'f', 6, 64),
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

func writeNodeCats(w io.Writer, tc *timetree.Collection, rt map[string]*recTree) error {
	tab := csv.NewWriter(w)
	tab.Comma = '\t'
	tab.UseCRLF = true

	if err := tab.Write([]string{"tree", "node", "cat", "lambda", "marginal"}); err != nil {
		return err
	}
	for _, name := range tc.Names() {
		dt, ok := rt[name]
		if !ok {
			continue
		}
		t := tc.Tree(name)
		for _, nID := range t.Nodes() {
			n, ok := dt.nodes[nID]
			if !ok {
				continue
			}
			cats := make([]int, 0, len(n.cats))
			for _, c := range n.cats {
				cats = append(cats, c.cat)
			}
			slices.Sort(cats)

			nodeID := strconv.Itoa(nID)
			for _, cv := range cats {
				cr := n.cats[cv]
				prob := float64(cr.count) / float64(n.particles)
				row := []string{
					name,
					nodeID,
					strconv.Itoa(cr.cat),
					strconv.FormatFloat(cr.lambda, 'f', 6, 64),
					strconv.FormatFloat(prob, 'f', 6, 64),
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

		if useCats {
			sp := make(map[int]int)
			var max int
			for _, nID := range t.Nodes() {
				n, ok := rec.nodes[nID]
				if !ok {
					continue
				}
				bc := 0
				var prob float64
				for i, c := range n.cats {
					if c.cat > max {
						max = c.cat
					}
					p := float64(c.count) / float64(n.particles)
					if p > prob {
						bc = i
						prob = p
					}
				}
				sp[nID] = bc
			}
			st.setCat(sp, max, gradient)
		} else {
			sp := make(map[int]float64)
			min := math.MaxFloat64
			max := math.SmallestNonzeroFloat64
			for _, nID := range t.Nodes() {
				n, ok := rec.nodes[nID]
				if !ok {
					continue
				}
				s := math.Log10(n.speed)
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
			st.setColor(sp, min, max, gradient)
		}

		fName := treePrefix + "-" + name
		if useCats {
			fName += "-cats"
		}
		fName += ".svg"
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
