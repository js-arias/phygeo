// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package speed

import (
	"encoding/xml"
	"fmt"
	"image/color"
	"io"
	"math"
	"os"
	"strconv"

	"github.com/js-arias/earth"
	"github.com/js-arias/timetree"
)

const yStep = 12

type node struct {
	x     float64
	y     int
	topY  int
	botY  int
	color color.RGBA

	id  int
	tax string
	age float64

	anc  *node
	desc []*node
}

type svgTree struct {
	y     int
	x     float64
	taxSz int
	root  *node
}

func copyTree(t *timetree.Tree, xStep float64) svgTree {
	maxSz := 0
	var root *node
	ids := make(map[int]*node)
	for _, id := range t.Nodes() {
		var anc *node
		p := t.Parent(id)
		if p >= 0 {
			anc = ids[p]
		}

		n := &node{
			id:  id,
			tax: t.Taxon(id),
			anc: anc,
			age: float64(t.Age(id)) / millionYears,
		}
		if anc == nil {
			root = n
		} else {
			anc.desc = append(anc.desc, n)
		}
		ids[id] = n
		if len(n.tax) > maxSz {
			maxSz = len(n.tax)
		}
	}

	s := svgTree{root: root}
	s.prepare(root, xStep)
	s.y = s.y * yStep
	s.taxSz = maxSz

	return s
}

func (s *svgTree) prepare(n *node, xStep float64) {
	n.x = (s.root.age-n.age)*xStep + 10
	if s.x < n.x {
		s.x = n.x
	}

	if n.desc == nil {
		n.y = s.y*yStep + 5
		s.y += 1
		return
	}

	botY := 0
	topY := math.MaxInt
	for _, d := range n.desc {
		s.prepare(d, xStep)
		if d.y < topY {
			topY = d.y
		}
		if d.y > botY {
			botY = d.y
		}
	}
	n.topY = topY
	n.botY = botY
	n.y = topY + (botY-topY)/2
}

func (s *svgTree) setColor(t *timetree.Tree, rec *recTree) {
	var max float64
	min := math.MaxFloat64
	nSp := make(map[int]float64, len(rec.nodes))
	for id, n := range rec.nodes {
		// skip root node
		pN := t.Parent(id)
		if pN < 0 {
			continue
		}

		brLen := float64(t.Age(pN)-t.Age(id)) / millionYears
		var sum float64
		for _, nd := range n.recs {
			sum += nd.dist / brLen
		}
		avg := sum / float64(len(n.recs))

		// scale to km per million year
		avg *= earth.Radius / 1000
		// and take the logarithm
		if avg == 0 {
			continue
		}
		avg = math.Log10(avg)
		nSp[id] = avg
		if avg > max {
			max = avg
		}
		if avg < min {
			min = avg
		}

		fmt.Fprintf(os.Stderr, "%d -> %.6f\n", id, avg)
	}
	fmt.Fprintf(os.Stderr, "[%.6f - %.6f]\n", min, max)

	s.root.setColor(nSp, min, max)
}

func (n *node) setColor(sp map[int]float64, min, max float64) {
	n.color = color.RGBA{0, 0, 255, 255}
	if v, ok := sp[n.id]; ok {
		n.color = scaleColor((v - min) / (max - min))
	}

	for _, d := range n.desc {
		d.setColor(sp, min, max)
	}

}

func (s *svgTree) draw(w io.Writer) error {
	fmt.Fprintf(w, "%s", xml.Header)
	e := xml.NewEncoder(w)
	svg := xml.StartElement{
		Name: xml.Name{Local: "svg"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "height"}, Value: strconv.Itoa(s.y + 5)},
			// assume that each character has 6 pixels wide
			{Name: xml.Name{Local: "width"}, Value: strconv.Itoa(int(s.x) + s.taxSz*6)},
			{Name: xml.Name{Local: "xmlns"}, Value: "http://www.w3.org/2000/svg"},
		},
	}
	e.EncodeToken(svg)

	g := xml.StartElement{
		Name: xml.Name{Local: "g"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "stroke-width"}, Value: "2"},
			{Name: xml.Name{Local: "stroke"}, Value: "black"},
			{Name: xml.Name{Local: "stroke-linecap"}, Value: "round"},
			{Name: xml.Name{Local: "font-family"}, Value: "Verdana"},
			{Name: xml.Name{Local: "font-size"}, Value: "10"},
		},
	}
	e.EncodeToken(g)

	s.root.draw(e)
	s.root.label(e)

	e.EncodeToken(g.End())
	e.EncodeToken(svg.End())
	if err := e.Flush(); err != nil {
		return err
	}
	return nil
}

func (n node) draw(e *xml.Encoder) {
	r, g, b, _ := n.color.RGBA()
	rgb := fmt.Sprintf("rgb(%d,%d,%d)", r>>8, g>>8, b>>8)

	// horizontal line
	ln := xml.StartElement{
		Name: xml.Name{Local: "line"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "x1"}, Value: strconv.Itoa(int(n.x - 5))},
			{Name: xml.Name{Local: "y1"}, Value: strconv.Itoa(int(n.y))},
			{Name: xml.Name{Local: "x2"}, Value: strconv.Itoa(int(n.x))},
			{Name: xml.Name{Local: "y2"}, Value: strconv.Itoa(int(n.y))},
			{Name: xml.Name{Local: "stroke"}, Value: rgb},
		},
	}
	if n.anc != nil {
		ln.Attr[0].Value = strconv.Itoa(int(n.anc.x))
	}
	e.EncodeToken(ln)
	e.EncodeToken(ln.End())

	// terminal name
	if n.desc == nil {
		return
	}

	// draws vertical line
	ln.Attr[0].Value = ln.Attr[2].Value
	ln.Attr[1].Value = strconv.Itoa(int(n.topY))
	ln.Attr[3].Value = strconv.Itoa(int(n.botY))
	e.EncodeToken(ln)
	e.EncodeToken(ln.End())

	for _, d := range n.desc {
		d.draw(e)
	}
}

func (n node) label(e *xml.Encoder) {
	if n.desc == nil {
		tx := xml.StartElement{
			Name: xml.Name{Local: "text"},
			Attr: []xml.Attr{
				{Name: xml.Name{Local: "x"}, Value: strconv.Itoa(int(n.x + 10))},
				{Name: xml.Name{Local: "y"}, Value: strconv.Itoa(int(n.y + 5))},
				{Name: xml.Name{Local: "stroke-width"}, Value: "0"},
				{Name: xml.Name{Local: "font-style"}, Value: "italic"},
			},
		}
		e.EncodeToken(tx)
		e.EncodeToken(xml.CharData(n.tax))
		e.EncodeToken(tx.End())
	}

	for _, d := range n.desc {
		d.label(e)
	}
}

func scaleColor(scale float64) color.RGBA {
	switch {
	case scale < 0.25:
		g := scale * 4 * 255
		return color.RGBA{0, uint8(g), 255, 255}
	case scale < 0.50:
		b := (scale - 0.25) * 4 * 255
		return color.RGBA{0, 255, 255 - uint8(b), 255}
	case scale < 0.75:
		r := (scale - 0.5) * 4 * 255
		return color.RGBA{uint8(r), 255, 0, 255}
	}
	g := (scale - 0.75) * 4 * 255
	return color.RGBA{255, 255 - uint8(g), 0, 255}
}
