// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package xmap implements a command to draw
// range reconstructions from pixel probability files.
package xmap

import (
	"encoding/csv"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/phygeo/pixkey"
	"github.com/js-arias/phygeo/probmap"
	"github.com/js-arias/phygeo/project"
)

var Command = &command.Command{
	Usage: `xmap [-c|--columns <value>] [--key <key-file>] [--gray]
	[--bound <value>]
	[--unrot] [--present] [--contour <image-file>]
	-i|--input <file> [-o|--output <file-prefix>] <project-file>`,
	Short: "draw a map reconstruction",
	Long: `
Command map reads a file with a probability reconstruction for the nodes of
one or more trees in a project and draws the reconstruction as an image map
using a plate carrée (equirectangular) projection.

The argument of the command is the name of the project file.

The flag --input, or -i, is required and indicates the input file.

By default, when reading a KDE reconstruction, it will only map the pixels in
the 0.95 of the CDF. Use the flag --bound to change this bound value.

By default, the reconstructions will be mapped using their respective time
stages. If the flag --unrot is given, then the reconstructions will be drawn
at the present time. By default, the landscape of the time stage will be used
for the background; if the flag --present is given, the present landscape will
be used for the background. If the --contour flag is defined with a file, the
given image will be used as a contour of the output map. The contour map will
set the size of the output image and should be fully transparent, except for
the contour, which will always be drawn in black.

By default, the output image will have the input file name as a prefix. To
change the prefix, use the flag --output or -o. The suffix of the file will be
the tree name, the node ID, and the time stage.

By default, the resulting image will be 3600 pixels wide. Use the flag
--column, or -c, to define a different number of columns. By default, the
images will have a gray background. Use the flag --key to define the landscape
colors of the image. If the flag --gray is set, then gray colors will be used.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var grayFlag bool
var unRot bool
var present bool
var colsFlag int
var bound float64
var contourFile string
var keyFile string
var inputFile string
var outPrefix string

func setFlags(c *command.Command) {
	c.Flags().BoolVar(&grayFlag, "gray", false, "")
	c.Flags().BoolVar(&unRot, "unrot", false, "")
	c.Flags().BoolVar(&present, "present", false, "")
	c.Flags().IntVar(&colsFlag, "columns", 3600, "")
	c.Flags().IntVar(&colsFlag, "c", 3600, "")
	c.Flags().Float64Var(&bound, "bound", 0.95, "")
	c.Flags().StringVar(&keyFile, "key", "", "")
	c.Flags().StringVar(&inputFile, "input", "", "")
	c.Flags().StringVar(&inputFile, "i", "", "")
	c.Flags().StringVar(&outPrefix, "output", "", "")
	c.Flags().StringVar(&outPrefix, "o", "", "")
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
		colsFlag = contour.Bounds().Dx()
	}
	if colsFlag%2 != 0 {
		colsFlag++
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

	var keys *pixkey.PixKey
	if keyFile != "" {
		keys, err = pixkey.Read(keyFile)
		if err != nil {
			return err
		}
		if grayFlag && !keys.HasGrayScale() {
			keys = nil
		}
	}

	if outPrefix == "" {
		outPrefix = inputFile
	}

	rt, err := getRec(inputFile, landscape)
	if err != nil {
		return err
	}

	for _, t := range rt {
		for _, n := range t.nodes {
			for _, s := range n.stages {
				age := float64(s.age) / 1_000_000
				out := fmt.Sprintf("%s-%s-n%d-%.3f.png", outPrefix, t.name, n.id, age)

				pm := &probmap.Image{
					Cols:      colsFlag,
					Age:       s.age,
					Landscape: landscape,
					Keys:      keys,
					Rng:       s.rec,
					Contour:   contour,
					Present:   present,
					Gray:      grayFlag,
				}
				pm.Format(tot)

				if err := writeImage(out, pm); err != nil {
					return err
				}
			}
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
	node *recNode
	age  int64
	rec  map[int]float64
}

var headerFields = []string{
	"tree",
	"node",
	"age",
	"type",
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
		st.rec[px] = v
	}
	if len(rt) == 0 {
		return nil, fmt.Errorf("while reading data: %v", io.EOF)
	}

	switch tp {
	case "log-like":
		// scale log-like values
		for _, t := range rt {
			for _, n := range t.nodes {
				for _, s := range n.stages {
					max := -math.MaxFloat64
					for _, p := range s.rec {
						if p > max {
							max = p
						}
					}
					for px, p := range s.rec {
						s.rec[px] = math.Exp(p - max)
					}
				}
			}
		}
	case "freq":
		// scale frequencies
		for _, t := range rt {
			for _, n := range t.nodes {
				for _, s := range n.stages {
					var max float64
					for _, p := range s.rec {
						if p > max {
							max = p
						}
					}
					for px, p := range s.rec {
						s.rec[px] = p / max
					}
				}
			}
		}
	case "kde":
		// remove pixels outside the bound
		for _, t := range rt {
			for _, n := range t.nodes {
				for _, s := range n.stages {
					for px, p := range s.rec {
						if p < 1-bound {
							delete(s.rec, px)
						}
					}
				}
			}
		}
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
