// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package mapcmd implements a command to draw
// range reconstructions from pixel probability files.
package mapcmd

import (
	"encoding/csv"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/js-arias/command"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/phygeo/probmap"
	"github.com/js-arias/phygeo/project"
)

var Command = &command.Command{
	Usage: `map [-c|--columns <value>]
	[--gray] [--scale <color-scale>]
	[--bound <value>]
	[--unrot] [--present] [--contour <image-file>]
	[--recent] [--trees <tree-list>] [--nodes <node-list>]
	-i|--input <file> [-o|--output <file-prefix>] <project-file>`,
	Short: "draw a map reconstruction",
	Long: `
Command map reads a file with a probability reconstruction for the nodes of
one or more trees in a project and draws the reconstruction as an image map
using a plate carrée (equirectangular) projection.

The argument of the command is the name of the project file.

The flag --input, or -i, is required and indicates the input file. The input
file is a pixel probability file.

By default, it will only map pixels in the 0.95 of the CDF. Use the flag
--bound to change this bound value.

By default, the reconstructions will be mapped using their respective time
stages. If the flag --unrot is given, then the reconstructions will be drawn
at the present time. By default, the landscape of the time stage will be used
for the background; if the flag --present is given, the present landscape will
be used for the background. If the --contour flag is defined with a file, the
given image will be used as a contour of the output map. The contour map will
set the size of the output image and should be fully transparent, except for
the contour, which will always be drawn in black.

By default, it will output the results of each node. If the flag --recent is
defined, only the most recent time stage for each node (i.e., splits and
terminals) will be used for output. If the flag trees is defined, only the
indicated trees will be used for output, the format is the tree names
separated by commas, for example "tree-1,tree-2" will produce maps for nodes
on trees tree-1 and tree-2. If the flag --nodes is defined, only the indicated
nodes will be used for output, the format is the node IDs separated by commas,
for example "0,1,6,10" will produce maps for nodes 0, 1, 6 and 10.

By default, the output image will have the input file name as a prefix. To
change the prefix, use the flag --output or -o. The suffix of the file will be
the tree name, the node ID, and the time stage.

By default, the resulting image will be 3600 pixels wide. Use the flag
--column, or -c, to define a different number of columns. By default, the
images will use the key defined in the project. If no key was defined, it will
use a gray background. If the flag --gray is set, it will use the gray colors
defined in the key.

By default, a rainbow color scale will be used, other color scales can be
defined using the --scale flag. Valid scale values are mostly based on Paul
Tol color scales:

	- iridescent  <https://personal.sron.nl/~pault/#fig:scheme_iridescent>
	- rainbow     default value (from purple to red)
	        <https://personal.sron.nl/~pault/#fig:scheme_rainbow_smooth>
	- incandescent
		<https://personal.sron.nl/~pault/#fig:scheme_incandescent>
	- gray         a gray scale from black to mid gray, so it can be
		coupled with a gray color key (gray values should be greater
		than 128).
	`,
	SetFlags: setFlags,
	Run:      run,
}

var grayFlag bool
var unRot bool
var present bool
var recentFlag bool
var colsFlag int
var bound float64
var treesFlag string
var nodesFlag string
var contourFile string
var inputFile string
var outPrefix string
var scale string

func setFlags(c *command.Command) {
	c.Flags().BoolVar(&grayFlag, "gray", false, "")
	c.Flags().BoolVar(&unRot, "unrot", false, "")
	c.Flags().BoolVar(&present, "present", false, "")
	c.Flags().BoolVar(&recentFlag, "recent", false, "")
	c.Flags().IntVar(&colsFlag, "columns", 3600, "")
	c.Flags().IntVar(&colsFlag, "c", 3600, "")
	c.Flags().Float64Var(&bound, "bound", 0.95, "")
	c.Flags().StringVar(&nodesFlag, "nodes", "", "")
	c.Flags().StringVar(&treesFlag, "trees", "", "")
	c.Flags().StringVar(&inputFile, "input", "", "")
	c.Flags().StringVar(&inputFile, "i", "", "")
	c.Flags().StringVar(&outPrefix, "output", "", "")
	c.Flags().StringVar(&outPrefix, "o", "", "")
	c.Flags().StringVar(&contourFile, "contour", "", "")
	c.Flags().StringVar(&scale, "scale", "rainbow", "")
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

	tc, err := p.Trees()
	if err != nil {
		return err
	}

	landscape, err := p.Landscape(nil)
	if err != nil {
		return err
	}

	tr, err := p.Traits()
	if err != nil {
		return err
	}

	var contour image.Image
	if contourFile != "" {
		contour, err = readContour(contourFile)
		if err != nil {
			return err
		}
		colsFlag = contour.Bounds().Dx()
	}
	if colsFlag%2 != 0 {
		colsFlag++
	}

	var tot *model.Total
	if unRot {
		tot, err = p.TotalRotation(landscape.Pixelation(), false)
		if err != nil {
			return err
		}
	}

	keys, err := p.Keys()
	if err != nil {
		return err
	}
	if grayFlag && !keys.HasGrayScale() {
		grayFlag = false
	}

	var gradient probmap.Gradienter
	switch strings.ToLower(scale) {
	case "gray":
		gradient = probmap.HalfGrayScale{}
	case "rainbow":
		gradient = probmap.RainbowPurpleToRed{}
	case "incandescent":
		gradient = probmap.Incandescent{}
	case "iridescent":
		gradient = probmap.Iridescent{}
	}

	if outPrefix == "" {
		outPrefix = inputFile
	}

	nodes, err := parseNodes()
	if err != nil {
		return err
	}
	trees := parseTreeNames()

	rt, err := getRec(inputFile, landscape)
	if err != nil {
		return err
	}

	treeList := tc.Names()
	if len(trees) == 0 {
		trees = make(map[string]bool, len(treeList))
		for _, t := range treeList {
			trees[t] = true
		}
	}

	for _, tn := range treeList {
		if !trees[tn] {
			continue
		}
		t, ok := rt[tn]
		if !ok {
			continue
		}
		pT := tc.Tree(tn)

		ns := pT.Nodes()
		nodeList := nodes
		if len(nodeList) == 0 {
			nodeList = make(map[int]bool, len(nodes))
			for id := range ns {
				nodeList[id] = true
			}
		}
		for _, id := range ns {
			if !nodeList[id] {
				continue
			}
			n, ok := t.nodes[id]
			if !ok {
				continue
			}
			stages := make([]int64, 0, len(n.stages))
			for a := range n.stages {
				// skip post-split stage
				if !pT.IsRoot(id) && pT.Age(pT.Parent(id)) == a {
					continue
				}
				stages = append(stages, a)
			}
			slices.Sort(stages)
			if recentFlag {
				stages = stages[:1]
			}

			for _, a := range stages {
				s := n.stages[a]
				age := float64(s.age) / 1_000_000
				trCDF := s.cdf()
				for _, trState := range tr.States() {
					prob, ok := trCDF[trState]
					if !ok {
						continue
					}
					out := fmt.Sprintf("%s-%s-n%d-%s-%.3f.png", outPrefix, t.name, n.id, trState, age)
					pm := &probmap.Image{
						Cols:      colsFlag,
						Age:       s.age,
						Landscape: landscape,
						Keys:      keys,
						Rng:       prob,
						Contour:   contour,
						Present:   present,
						Gray:      grayFlag,
						Gradient:  gradient,
					}
					pm.Format(tot)

					if err := writeImage(out, pm); err != nil {
						return err
					}
				}
			}

		}
	}

	return nil
}

func readContour(name string) (image.Image, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("on image file %q: %v", name, err)
	}
	return img, nil
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
	name  string
	rec   map[int]float64
}

type pixel struct {
	id    int
	trait string
	prob  float64
}

func (st *recStage) cdf() map[string]map[int]float64 {
	var sz int
	for _, r := range st.traits {
		sz += len(r.rec)
	}
	pixProb := make([]pixel, 0, sz)
	for _, r := range st.traits {
		for px, p := range r.rec {
			prob := pixel{
				id:    px,
				trait: r.name,
				prob:  p,
			}
			pixProb = append(pixProb, prob)
		}
	}
	slices.SortFunc(pixProb, func(a, b pixel) int {
		if a.prob > b.prob {
			return -1
		}
		if a.prob < b.prob {
			return 1
		}
		return 0
	})

	tr := make(map[string]map[int]float64, len(st.traits))
	for _, r := range st.traits {
		tr[r.name] = make(map[int]float64, len(r.rec))
	}

	prob := 1.0
	for _, px := range pixProb {
		tr[px.trait][px.id] = prob
		prob -= px.prob
		if 1-bound > prob {
			break
		}
	}
	return tr
}

var headerFields = []string{
	"tree",
	"node",
	"age",
	"type",
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
				node:   n,
				age:    age,
				traits: make(map[string]*recTrait),
			}
			n.stages[age] = st
		}

		f = "trait"
		trn := strings.Join(strings.Fields(row[fields[f]]), " ")
		if trn == "" {
			continue
		}
		trn = strings.ToLower(trn)
		tr, ok := st.traits[trn]
		if !ok {
			tr = &recTrait{
				stage: st,
				name:  trn,
				rec:   make(map[int]float64),
			}
			st.traits[trn] = tr
		}

		f = "type"
		tpV := strings.ToLower(strings.Join(strings.Fields(row[fields[f]]), " "))
		if tpV != "freq" {
			return nil, fmt.Errorf("on row %d: field %q: got %q want %q", ln, f, tpV, "pmf")
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
		tr.rec[px] += v
	}
	if len(rt) == 0 {
		return nil, fmt.Errorf("while reading data: %v", io.EOF)
	}
	return rt, nil
}

func writeImage(name string, m *probmap.Image) (err error) {
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

	if err := png.Encode(f, m); err != nil {
		return fmt.Errorf("when encoding image file %q: %v", name, err)
	}
	return nil
}

func parseTreeNames() map[string]bool {
	if treesFlag == "" {
		return nil
	}
	trees := strings.Split(treesFlag, ",")
	tm := make(map[string]bool, len(trees))
	for _, t := range trees {
		tm[strings.ToLower(t)] = true
	}
	return tm
}

func parseNodes() (map[int]bool, error) {
	if nodesFlag == "" {
		return nil, nil
	}

	ids := strings.Split(nodesFlag, ",")
	nodes := make(map[int]bool, len(ids))
	for _, id := range ids {
		n, err := strconv.Atoi(id)
		if err != nil {
			return nil, fmt.Errorf("on flag --nodes: %v", err)
		}
		nodes[n] = true
	}
	return nodes, nil
}
