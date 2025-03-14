// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package diffusion

import (
	"math"
	"math/rand/v2"

	"github.com/js-arias/earth"
	"github.com/js-arias/earth/stat/dist"
	"github.com/js-arias/earth/stat/pixweight"
	"github.com/js-arias/timetree"
)

// NewSimData creates a new tree
// for data simulation
// by copying the indicated source tree.
//
// To make the simulation
// use method Simulate.
func NewSimData(t *timetree.Tree, p Param, spread float64) *Tree {
	nt := &Tree{
		t:         t,
		nodes:     make(map[int]*node, len(t.Nodes())),
		landscape: p.Landscape,
		rot:       p.Rot,
		dm:        p.DM,
		pw:        p.PW,
	}

	root := &node{
		id: t.Root(),
	}
	nt.nodes[root.id] = root
	root.copySource(nt, p.Landscape, p.Stem, p.Stages)

	// Prepare nodes and time stages
	for _, n := range nt.nodes {
		n.setPDF(p.Landscape.Pixelation(), p.Lambda)
	}

	// Create the centroid for the simulation
	source := nt.startParticle(spread)
	root.centroidSimulation(nt, source, spread)
	return nt
}

// RootField creates the starting field
// and point of the simulation.
func (t *Tree) startParticle(lambda float64) int {
	root := t.nodes[t.t.Root()]
	rs := root.stages[0]

	age := t.landscape.ClosestStageAge(rs.age)
	stage := t.landscape.Stage(age)

	pix := t.landscape.Pixelation()

	px := -1
	for {
		px = pix.Random().ID()
		accept := t.pw.Weight(stage[px])
		if rand.Float64() < accept {
			break
		}
	}

	pdf := dist.NewNormal(lambda, pix)
	prob := buildDensity(pix, pdf, t.dm, px, stage, t.pw)
	rs.logLike = make(map[int]float64, len(prob))
	for px, p := range prob {
		rs.logLike[px] = math.Log(p)
	}
	return rotPix(t.rot, t.landscape, px, rs.age, t.pw)
}

func (n *node) centroidSimulation(t *Tree, source int, spread float64) {
	for i := 1; i < len(n.stages); i++ {
		ts := n.stages[i]
		source = ts.centroidSimulation(t, source, spread)
	}
	like := n.stages[len(n.stages)-1].logLike

	for _, cID := range t.t.Children(n.id) {
		c := t.nodes[cID]
		sp := c.stages[0]
		sp.logLike = make(map[int]float64, len(like))
		for px, p := range like {
			sp.logLike[px] = p
		}
		c.centroidSimulation(t, source, spread)
	}
}

func (ts *timeStage) centroidSimulation(t *Tree, source int, spread float64) int {
	age := t.landscape.ClosestStageAge(ts.age)
	stage := t.landscape.Stage(age)

	pix := t.landscape.Pixelation()
	density := buildDensity(pix, ts.pdf, t.dm, source, stage, t.pw)

	centroid := pick(density)
	pdf := dist.NewNormal(spread, pix)
	prob := buildDensity(pix, pdf, t.dm, centroid, stage, t.pw)
	ts.logLike = make(map[int]float64, len(prob))
	for px, p := range prob {
		ts.logLike[px] = math.Log(p)
	}
	return rotPix(t.rot, t.landscape, centroid, ts.age, t.pw)

}

func buildDensity(pix *earth.Pixelation, pdf dist.Normal, dm *earth.DistMat, source int, stage map[int]int, pw pixweight.Pixel) []float64 {
	density := make([]float64, 0, pix.Len())
	var max float64

	if dm != nil {
		// use distance matrix
		for px := 0; px < pix.Len(); px++ {
			weight := pw.Weight(stage[px])
			if weight == 0 {
				density = append(density, 0)
				continue
			}
			p := pdf.ProbRingDist(dm.At(source, px)) * weight
			density = append(density, p)
			if p > max {
				max = p
			}
		}
	} else {
		// use raw distance
		pt1 := pix.ID(source).Point()
		for px := 0; px < pix.Len(); px++ {
			weight := pw.Weight(stage[px])
			if weight == 0 {
				density = append(density, 0)
				continue
			}
			pt2 := pix.ID(px).Point()
			dist := earth.Distance(pt1, pt2)
			p := pdf.Prob(dist) * weight
			density = append(density, p)
			if p > max {
				max = p
			}
		}
	}

	for i, d := range density {
		density[i] = d / max
	}

	return density
}

func pick(density []float64) int {
	for {
		px := rand.IntN(len(density))
		accept := density[px]
		if rand.Float64() < accept {
			return px
		}
	}
}
