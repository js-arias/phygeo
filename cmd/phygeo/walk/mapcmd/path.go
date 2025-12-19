// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package mapcmd

import (
	"encoding/csv"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/pixkey"
	"github.com/js-arias/phygeo/probmap"
	"github.com/js-arias/timetree"
)

func makePathMaps(trees map[string]bool, nodes map[int]bool, tc *timetree.Collection, tp *model.TimePix, tot *model.Total, gradient probmap.Gradienter, keys *pixkey.PixKey, contour image.Image) error {
	rt, err := getPath(inputFile, tp)
	if err != nil {
		return err
	}

	for tn := range trees {
		pT := tc.Tree(tn)
		if pT == nil {
			continue
		}
		t, ok := rt[tn]
		if !ok {
			continue
		}

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
				stages = append(stages, a)
			}
			slices.Sort(stages)
			if recentFlag {
				stages = stages[:1]
			}
			for _, a := range stages {
				s := n.stages[a]
				age := float64(s.age) / 1_000_000
				out := fmt.Sprintf("%s-%s-n%d-p%d-%.3f.png", outPrefix, t.name, n.id, particleID, age)
				pm := &probmap.Image{
					Cols:      colsFlag,
					Age:       s.age,
					Landscape: tp,
					Keys:      keys,
					Rng:       s.path,
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
	return nil
}

func getPath(name string, tp *model.TimePix) (map[string]*recTree, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rt, err := readPath(f, tp, particleID)
	if err != nil {
		return nil, fmt.Errorf("on input file %q: %v", name, err)
	}
	return rt, nil
}

var pathHeader = []string{
	"tree",
	"particle",
	"node",
	"age",
	"equator",
	"path",
}

func readPath(r io.Reader, tp *model.TimePix, pID int) (map[string]*recTree, error) {
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
	for _, h := range pathHeader {
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

		f = "particle"
		p, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if p != pID {
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
				path: make(map[int]float64),
			}
			n.stages[age] = st
		}

		f = "path"
		path := strings.Split(row[fields[f]], ",")
		if len(path) == 0 {
			return nil, fmt.Errorf("on row %d: field %q: empty path", ln, f)
		}
		fraction := 1.0 / float64(len(path))
		for i, pp := range path {
			_, px, err := parseTraitPix(pp)
			if err != nil {
				return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
			}
			if px >= tp.Pixelation().Len() {
				return nil, fmt.Errorf("on row %d: field %q: invalid pixel value %d", ln, f, px)
			}
			st.path[px] = float64(i+1) * fraction
		}
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
