// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package diffusion

import (
	"math"
	"math/rand/v2"

	"github.com/js-arias/earth"
	"github.com/js-arias/earth/stat/dist"
	"github.com/js-arias/earth/stat/pixprob"
	"github.com/js-arias/timetree"
)

// NewSimData creates a new tree
// for data simulation
// by copying the indicated source tree.
//
// To make the simulation
// use method Simulate.
func NewSimData(t *timetree.Tree, p Param) *Tree {
	nt := &Tree{
		t:         t,
		nodes:     make(map[int]*node, len(t.Nodes())),
		landscape: p.Landscape,
		rot:       p.Rot,
		dm:        p.DM,
		pp:        p.PP,
		logPrior:  make(map[int]float64, len(p.PP.Values())),
	}
	for _, v := range p.PP.Values() {
		p := p.PP.Prior(v)
		if p == 0 {
			continue
		}
		nt.logPrior[v] = math.Log(p)
	}

	root := &node{
		id: t.Root(),
	}
	nt.nodes[root.id] = root
	root.copySource(nt, p.Landscape, p.Stem)

	// Prepare nodes and time stages
	for _, n := range nt.nodes {
		n.setPDF(p.Landscape.Pixelation(), p.Lambda)
	}

	// Create the centroid for the simulation
	source := nt.startParticle(p.Lambda)
	root.centroidSimulation(nt, source)
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

	pdf := dist.NewNormal(lambda, pix)
	for {
		px := pix.Random().ID()
		accept := t.pp.Prior(stage[px])
		if rand.Float64() < accept {
			prob := buildDensity(pix, pdf, t.dm, px, stage, t.pp)
			rs.logLike = make(map[int]float64, len(prob))
			for px, p := range prob {
				rs.logLike[px] = math.Log(p)
			}
			return rotPix(t.rot, t.landscape, px, rs.age, t.pp)
		}
	}
}

func (n *node) centroidSimulation(t *Tree, source int) {
	for i := 1; i < len(n.stages); i++ {
		ts := n.stages[i]
		source = ts.centroidSimulation(t, source)
	}
	like := n.stages[len(n.stages)-1].logLike

	for _, cID := range t.t.Children(n.id) {
		c := t.nodes[cID]
		sp := c.stages[0]
		sp.logLike = make(map[int]float64, len(like))
		for px, p := range like {
			sp.logLike[px] = p
		}
		c.centroidSimulation(t, source)
	}
}

func (ts *timeStage) centroidSimulation(t *Tree, source int) int {
	age := t.landscape.ClosestStageAge(ts.age)
	stage := t.landscape.Stage(age)

	pix := t.landscape.Pixelation()
	density := buildDensity(pix, ts.pdf, t.dm, source, stage, t.pp)

	centroid := pick(density)
	prob := buildDensity(pix, ts.pdf, t.dm, centroid, stage, t.pp)
	ts.logLike = make(map[int]float64, len(prob))
	for px, p := range prob {
		ts.logLike[px] = math.Log(p)
	}
	return rotPix(t.rot, t.landscape, centroid, ts.age, t.pp)

}

func buildDensity(pix *earth.Pixelation, pdf dist.Normal, dm *earth.DistMat, source int, stage map[int]int, pp pixprob.Pixel) []float64 {
	density := make([]float64, 0, pix.Len())
	var max float64

	if dm != nil {
		// use distance matrix
		for px := 0; px < pix.Len(); px++ {
			prior := pp.Prior(stage[px])
			if prior == 0 {
				density = append(density, 0)
				continue
			}
			p := pdf.ProbRingDist(dm.At(source, px)) * prior
			density = append(density, p)
			if p > max {
				max = p
			}
		}
	} else {
		// use raw distance
		pt1 := pix.ID(source).Point()
		for px := 0; px < pix.Len(); px++ {
			prior := pp.Prior(stage[px])
			if prior == 0 {
				density = append(density, 0)
				continue
			}
			pt2 := pix.ID(px).Point()
			dist := earth.Distance(pt1, pt2)
			p := pdf.Prob(dist) * prior
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
