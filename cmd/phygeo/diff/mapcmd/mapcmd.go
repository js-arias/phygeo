// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package mapcmd implements a command to draw
// the range reconstructions of a tree nodes.
package mapcmd

import (
	"encoding/csv"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/js-arias/blind"
	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/stat"
	"github.com/js-arias/earth/stat/dist"
	"github.com/js-arias/earth/stat/pixprob"
	"github.com/js-arias/phygeo/project"
)

var Command = &command.Command{
	Usage: `map [-c|--columns <value>] [--key <key-file>] [--gray]
	[--kde <value>] [--bound <value>] [-cpu <number>]
	[--unrot] [--present] [--contour <image-file>]
	-i|--input <file> [-o|--output <file-prefix>] <project-file>`,
	Short: "draw a map of a reconstruction",
	Long: `
Command map reads a file with a range reconstruction for the nodes of one or
more trees in a project, and draws the reconstruction as an image map using a
plate carrée (equirectangular) projection.

The argument of the command is the name of the project file.

The flag --input, or -i, is required an indicates the input file.

By default the ranges will be taken as given. If the flag --kde is defined,
a kernel density estimation using an spherical normal will be done using the
indicated value as the concentration parameter (in 1/radians^2). Only the
pixels in the .95 of the maximum value will be used. Use the flag --bound to
change this bound value.

As the number of nodes might be large, and when calculating a KDE the number
of computations can be large, the process is run in parallel using all
available processors. Use the flag --cpu to change the number of processors.

By default, the ranges will be produced using their respective time stage. If
the flag --unrot is given, then the estimated ranges will be draw at the
present time. By default, the paleogeography of the time stage will be used.
If --present flag is defined, the present time pixelation will be used for the
background.

If --contour is defined with a file, the given image will be used as a contour
of the output map. The contour image should have the same size of the output
image, and fully transparent, except for the contour, that will be always draw
in black. 

By default the output file image will have the input file name as prefix. To
change the prefix use the flag --output, or -o. The suffix of the file will be
the tree name, the node ID, and time stage, for example the suffix
"-vireya-n4-10.000.png" will be produced for node 4 of the 'vireya' tree, at
the time stage of 10 million years. By default the resulting image will be
3600 pixels wide. Use the flag --columns, or -c, tp define a different number
of columns.

By default the output images will be plain gray background. Use the flag --key
to define a set of colors for the image (using the project landscape).
If the flag --gray is given, then a gray colors will be used. The key file is
a tab-delimited file with the following required columns:

	-key	the value used as identifier
	-color	an RGB value separated by commas,
		for example "125,132,148".

Optionally it can contain the following columns:

	-gray:  for a gray scale value

Any other columns, will be ignored. Here is an example of a key file:

	key	color	gray	comment
	0	0, 26, 51	0	deep ocean
	1	0, 84, 119	10	oceanic plateaus
	2	68, 167, 196	20	continental shelf
	3	251, 236, 93	90	lowlands
	4	255, 165, 0	100	highlands
	5	229, 229, 224	50	ice sheets
	`,
	SetFlags: setFlags,
	Run:      run,
}

var grayFlag bool
var unRot bool
var present bool
var colsFlag int
var numCPU int
var kdeLambda float64
var bound float64
var keyFile string
var inputFile string
var outputPre string
var contourFile string

func setFlags(c *command.Command) {
	c.Flags().BoolVar(&grayFlag, "gray", false, "")
	c.Flags().BoolVar(&unRot, "unrot", false, "")
	c.Flags().BoolVar(&present, "present", false, "")
	c.Flags().IntVar(&colsFlag, "columns", 3600, "")
	c.Flags().IntVar(&colsFlag, "c", 3600, "")
	c.Flags().IntVar(&numCPU, "cpu", runtime.GOMAXPROCS(0), "")
	c.Flags().Float64Var(&kdeLambda, "kde", 0, "")
	c.Flags().Float64Var(&bound, "bound", 0.95, "")
	c.Flags().StringVar(&keyFile, "key", "", "")
	c.Flags().StringVar(&inputFile, "input", "", "")
	c.Flags().StringVar(&inputFile, "i", "", "")
	c.Flags().StringVar(&outputPre, "output", "", "")
	c.Flags().StringVar(&outputPre, "o", "", "")
	c.Flags().StringVar(&contourFile, "contour", "", "")
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

	var contour image.Image
	if contourFile != "" {
		contour, err = readContour(contourFile)
		if err != nil {
			return err
		}
	}

	var tot *model.Total
	if unRot {
		rotF := p.Path(project.GeoMotion)
		if rotF == "" {
			msg := fmt.Sprintf("plate motion model not defined in project %q", args[0])
			return c.UsageError(msg)
		}
		tot, err = readRotation(rotF, landscape.Pixelation())
		if err != nil {
			return err
		}
	}

	rec, err := getRec(inputFile, landscape)
	if err != nil {
		return err
	}

	var keys *pixKey
	if keyFile != "" {
		keys, err = readKeys(keyFile)
		if err != nil {
			return err
		}
	}

	var pp pixprob.Pixel
	var norm dist.Normal
	if kdeLambda > 0 {
		ppF := p.Path(project.PixPrior)
		if ppF == "" {
			msg := fmt.Sprintf("pixel priors not defined in project %q", args[0])
			return c.UsageError(msg)
		}
		pp, err = readPriors(ppF)
		if err != nil {
			return err
		}

		norm = dist.NewNormal(kdeLambda, landscape.Pixelation())
	}

	if outputPre == "" {
		outputPre = inputFile
	}

	sc := make(chan stageChan, numCPU*2)
	for i := 0; i < numCPU; i++ {
		go procStage(sc)
	}

	errChan := make(chan error)
	doneChan := make(chan struct{})
	var wg sync.WaitGroup
	for _, t := range rec {
		for _, n := range t.nodes {
			for _, s := range n.stages {
				// the age is in million years
				age := float64(s.age) / 1_000_000
				suf := fmt.Sprintf("-%s-n%d-%.3f", t.name, n.id, age)

				s.step = 360 / float64(colsFlag)
				s.keys = keys
				s.contour = contour
				wg.Add(1)
				sc <- stageChan{
					rs:        s,
					out:       outputPre + suf + ".png",
					err:       errChan,
					wg:        &wg,
					norm:      norm,
					pp:        pp,
					landscape: landscape,
					tot:       tot,
				}
			}
		}
	}

	go func() {
		wg.Wait()
		close(doneChan)
	}()

	select {
	case err := <-errChan:
		return err
	case <-doneChan:
	}

	return nil
}

type stageChan struct {
	rs  *recStage
	out string
	err chan error
	wg  *sync.WaitGroup

	norm      dist.Normal
	pp        pixprob.Pixel
	landscape *model.TimePix
	tot       *model.Total
}

func procStage(c chan stageChan) {
	for sc := range c {
		s := sc.rs

		if kdeLambda > 0 {
			rng := stat.KDE(sc.norm, s.rec, sc.landscape, s.cAge, sc.pp, bound)
			s.rec = rng
			var max float64
			for _, p := range s.rec {
				if p > max {
					max = p
				}
			}
			s.max = max
		}
		if unRot {
			s.tot = sc.tot.Rotation(s.cAge)
		}

		if err := writeImage(sc.out, s); err != nil {
			sc.err <- err
		}
		sc.wg.Done()
	}
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

func readRotation(name string, pix *earth.Pixelation) (*model.Total, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rot, err := model.ReadTotal(f, pix, false)
	if err != nil {
		return nil, fmt.Errorf("on file %q: %v", name, err)
	}

	return rot, nil
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
	cAge      int64
	rec       map[int]float64
	max       float64
	landscape *model.TimePix
	tot       map[int][]int
	step      float64
	keys      *pixKey

	contour image.Image
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
				cAge:      landscape.ClosestStageAge(age),
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
		if v := st.rec[px]; v > st.max {
			st.max = v
		}
	}
	if len(rt) == 0 {
		return nil, fmt.Errorf("while reading data: %v", io.EOF)
	}

	return rt, nil
}

func (rs *recStage) ColorModel() color.Model { return color.RGBAModel }
func (rs *recStage) Bounds() image.Rectangle { return image.Rect(0, 0, colsFlag, colsFlag/2) }
func (rs *recStage) At(x, y int) color.Color {
	if rs.contour != nil {
		_, _, _, a := rs.contour.At(x, y).RGBA()
		if a > 100 {
			return color.RGBA{A: 255}
		}
	}

	lat := 90 - float64(y)*rs.step
	lon := float64(x)*rs.step - 180

	pix := rs.landscape.Pixelation().Pixel(lat, lon)

	if unRot {
		// Total rotation from present time
		// to stage time
		dst := rs.tot[pix.ID()]
		if len(dst) == 0 {
			v, _ := rs.landscape.At(0, pix.ID())
			if grayFlag {
				if c, ok := rs.keys.Gray(v); ok {
					return c
				}
			} else {
				if c, ok := rs.keys.Color(v); ok {
					return c
				}
			}
			return color.RGBA{211, 211, 211, 255}
		}

		// Check if the pixel is in the range
		// of the stage time
		var max float64
		for _, px := range dst {
			p := rs.rec[px]
			if p > max {
				max = p
			}
		}
		if max > 0 {
			return blind.Gradient(max / rs.max)
		}

		// Check the value of the pixel
		// at the stage time
		var v int
		if present {
			v, _ = rs.landscape.At(0, pix.ID())
		} else {
			for _, px := range dst {
				vv, _ := rs.landscape.At(rs.cAge, px)
				if vv > v {
					v = vv
				}
			}
		}
		if grayFlag {
			if c, ok := rs.keys.Gray(v); ok {
				return c
			}
		} else {
			if c, ok := rs.keys.Color(v); ok {
				return c
			}
		}

		return color.RGBA{211, 211, 211, 255}
	}

	if p, ok := rs.rec[pix.ID()]; ok {
		return blind.Gradient(p / rs.max)
	}

	if rs.keys == nil {
		return color.RGBA{211, 211, 211, 255}
	}

	v, _ := rs.landscape.At(rs.cAge, pix.ID())
	if present {
		v, _ = rs.landscape.At(0, pix.ID())
	}
	if grayFlag {
		if c, ok := rs.keys.Gray(v); ok {
			return c
		}
	} else {
		if c, ok := rs.keys.Color(v); ok {
			return c
		}
	}

	return color.RGBA{211, 211, 211, 255}
}

func writeImage(name string, rs *recStage) (err error) {
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

	if err := png.Encode(f, rs); err != nil {
		return fmt.Errorf("when encoding image file %q: %v", name, err)
	}

	return nil
}
