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
	"strconv"

	"github.com/js-arias/blind"
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
	y      int
	x      float64
	minAge float64
	xStep  float64

	// timeline ticks
	min   int // small ticks
	max   int // large ticks
	label int // label ticks

	taxSz int
	root  *node
}

func copyTree(t *timetree.Tree, xStep float64, minTick, maxTick, labelTick int) svgTree {
	maxSz := 0
	var root *node
	ids := make(map[int]*node)
	minAge := float64(t.Age(t.Root())) / millionYears
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
		if n.age < minAge {
			minAge = n.age
		}
	}

	s := svgTree{
		xStep:  xStep,
		minAge: minAge,
		root:   root,
		min:    minTick,
		max:    maxTick,
		label:  labelTick,
		taxSz:  maxSz,
	}
	s.prepare(root)
	s.y = s.y * yStep

	return s
}

func (s *svgTree) prepare(n *node) {
	n.x = (s.root.age-n.age)*s.xStep + 10
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
		s.prepare(d)
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

func (s *svgTree) setColor(sp map[int]float64, min, max, avg float64) {
	s.root.setColor(sp, min, max, avg)
	s.root.color = color.RGBA{102, 102, 102, 255}
}

func (n *node) setColor(sp map[int]float64, min, max, avg float64) {
	n.color = color.RGBA{0, 0, 255, 255}
	if v, ok := sp[n.id]; ok {
		if v > avg {
			n.color = blind.Gradient(0.5 + 0.5*(v-avg)/(max-avg))
		} else {
			n.color = blind.Gradient(0.5 * (v - min) / (avg - min))
		}
	} else {
		n.color = blind.Gradient(0)
	}

	for _, d := range n.desc {
		d.setColor(sp, min, max, avg)
	}

}

func (s *svgTree) draw(w io.Writer) error {
	fmt.Fprintf(w, "%s", xml.Header)
	e := xml.NewEncoder(w)
	svg := xml.StartElement{
		Name: xml.Name{Local: "svg"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "height"}, Value: strconv.Itoa(s.y + 5 + 2*yStep)},
			// assume that each character has 6 pixels wide
			{Name: xml.Name{Local: "width"}, Value: strconv.Itoa(int(s.x) + s.taxSz*6)},
			{Name: xml.Name{Local: "xmlns"}, Value: "http://www.w3.org/2000/svg"},
		},
	}
	e.EncodeToken(svg)

	g := xml.StartElement{
		Name: xml.Name{Local: "g"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "stroke-width"}, Value: "4"},
			{Name: xml.Name{Local: "stroke"}, Value: "black"},
			{Name: xml.Name{Local: "stroke-linecap"}, Value: "round"},
			{Name: xml.Name{Local: "font-family"}, Value: "Verdana"},
			{Name: xml.Name{Local: "font-size"}, Value: "10"},
		},
	}
	e.EncodeToken(g)

	s.drawTimeRecs(e)
	s.drawTimeScale(e)

	s.root.draw(e)
	s.root.label(e)

	e.EncodeToken(g.End())
	e.EncodeToken(svg.End())
	if err := e.Flush(); err != nil {
		return err
	}
	return nil
}

func (s svgTree) drawTimeRecs(e *xml.Encoder) {
	if timeBox == 0 {
		return
	}

	height := s.y
	for a := 0.0; ; a += timeBox * 2 {
		if a+timeBox < s.minAge {
			continue
		}
		maxX := (s.root.age-a)*s.xStep + 10
		if maxX > s.x {
			maxX = s.x
		}
		minX := (s.root.age-(a+timeBox))*s.xStep + 10

		if maxX < s.root.x {
			break
		}

		// rectangle
		rect := xml.StartElement{
			Name: xml.Name{Local: "rect"},
			Attr: []xml.Attr{
				{Name: xml.Name{Local: "x"}, Value: strconv.Itoa(int(minX))},
				{Name: xml.Name{Local: "width"}, Value: strconv.Itoa(int(maxX - minX))},
				{Name: xml.Name{Local: "height"}, Value: strconv.Itoa(int(height))},
				{Name: xml.Name{Local: "style"}, Value: "fill:rgb(200,200,200); stroke-width:0"},
			},
		}
		e.EncodeToken(rect)
		e.EncodeToken(rect.End())
	}
}

func (s svgTree) drawTimeScale(e *xml.Encoder) {
	y := s.y + yStep/2
	ln := xml.StartElement{
		Name: xml.Name{Local: "line"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "x1"}, Value: strconv.Itoa(int(s.root.x))},
			{Name: xml.Name{Local: "y1"}, Value: strconv.Itoa(int(y))},
			{Name: xml.Name{Local: "x2"}, Value: strconv.Itoa(int(s.x))},
			{Name: xml.Name{Local: "y2"}, Value: strconv.Itoa(int(y))},
			{Name: xml.Name{Local: "stroke-width"}, Value: "2"},
		},
	}
	e.EncodeToken(ln)
	e.EncodeToken(ln.End())

	// Add tick marks
	for a := 0.0; a < s.root.age; a += float64(s.min) {
		if a < s.minAge {
			continue
		}

		x := (s.root.age-a)*s.xStep + 10
		ln.Attr[0].Value = strconv.Itoa(int(x))
		ln.Attr[2].Value = strconv.Itoa(int(x))

		maxY := y + yStep/4
		if int(a)%s.max == 0 {
			maxY = y + yStep/2
		}
		ln.Attr[3].Value = strconv.Itoa(int(maxY))
		e.EncodeToken(ln)
		e.EncodeToken(ln.End())

		// tick label
		if int(a)%s.label != 0 {
			continue
		}
		tx := xml.StartElement{
			Name: xml.Name{Local: "text"},
			Attr: []xml.Attr{
				{Name: xml.Name{Local: "x"}, Value: strconv.Itoa(int(x - 5))},
				{Name: xml.Name{Local: "y"}, Value: strconv.Itoa(int(y + yStep + 5))},
				{Name: xml.Name{Local: "stroke-width"}, Value: "0"},
			},
		}
		e.EncodeToken(tx)
		e.EncodeToken(xml.CharData(strconv.Itoa(int(a))))
		e.EncodeToken(tx.End())

	}
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
