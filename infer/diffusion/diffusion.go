// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package diffusion implements an spherical diffusion
// approximated using a discrete isolatitude pixelation
// for a phylogenetic biogeography analysis.
package diffusion

import (
	"math"

	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/stat/dist"
	"github.com/js-arias/earth/stat/pixprob"
	"github.com/js-arias/ranges"
	"github.com/js-arias/timetree"
	"golang.org/x/exp/slices"
)

// Param is a collection of parameters
// for the initialization of a tree.
type Param struct {
	// Paleolandscape model
	Landscape *model.TimePix

	// Stage rotation model
	Rot *model.StageRot

	// Distance matrix
	DM *earth.DistMat

	// Pixel priors
	PP pixprob.Pixel

	// Ranges is the collection of terminal ranges
	Ranges *ranges.Collection

	// Length in years of the stem node
	Stem int64

	// Lambda is the concentration parameter per million years
	// in 1/radian units
	Lambda float64
}

// A Tree os a phylogenetic tree for biogeography.
type Tree struct {
	t     *timetree.Tree
	nodes map[int]*node

	landscape *model.TimePix
	rot       *model.StageRot
	dm        *earth.DistMat
	pp        pixprob.Pixel
	logPrior  map[int]float64
}

// New creates a new tree by copying the indicated source tree.
func New(t *timetree.Tree, p Param) *Tree {
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

		if !nt.t.IsTerm(n.id) {
			continue
		}

		// last terminal stage
		st := n.stages[len(n.stages)-1]

		rng := p.Ranges.Range(nt.t.Taxon(n.id))
		var sum float64
		for _, p := range rng {
			sum += p
		}

		st.logLike = make(map[int]float64, len(rng))
		for px, p := range rng {
			st.logLike[px] = math.Log(p) - math.Log(sum)
		}
	}
	root.fullDownPass(nt)

	return nt
}

// LogLike returns the logLikelihood of the whole reconstruction
// in the most basal stem node.
func (t *Tree) LogLike() float64 {
	root := t.nodes[t.t.Root()]
	ts := root.stages[0]

	max := -math.MaxFloat64
	for _, p := range ts.logLike {
		if p > max {
			max = p
		}
	}

	var sum float64
	for _, p := range ts.logLike {
		sum += math.Exp(p - max)
	}
	return math.Log(sum) + max
}

// Name returns the name of the tree.
func (t *Tree) Name() string {
	return t.t.Name()
}

// Nodes return an slice with IDs
// of the nodes of the tree.
func (t *Tree) Nodes() []int {
	return t.t.Nodes()
}

// SrcDest return the source and destination pixel
// for a given node,
// at a given age stage
// (in years),
// for a particular simulation.
func (t *Tree) SrcDest(n, p int, age int64) SrcDest {
	nn, ok := t.nodes[n]
	if !ok {
		return SrcDest{From: -1, To: -1}
	}

	i, ok := slices.BinarySearchFunc(nn.stages, age, func(st *timeStage, age int64) int {
		if st.age == age {
			return 0
		}
		if st.age < age {
			return 1
		}
		return -1
	})
	if !ok {
		return SrcDest{From: -1, To: -1}
	}

	if p >= len(nn.stages[i].particles) {
		return SrcDest{From: -1, To: -1}
	}
	return nn.stages[i].particles[p]
}

// Stages return age of the stages of a node
// (i.e., internodes)
// in years.
func (t *Tree) Stages(n int) []int64 {
	nn, ok := t.nodes[n]
	if !ok {
		return nil
	}

	ages := make([]int64, 0, len(nn.stages))
	for _, st := range nn.stages {
		ages = append(ages, st.age)
	}
	return ages
}

// A Node is a node in a phylogenetic tree.
type node struct {
	id     int
	stages []*timeStage

	lambda float64
}

const millionYears = 1_000_000

func (n *node) copySource(t *Tree, tp *model.TimePix, stem int64) {
	children := t.t.Children(n.id)
	for _, c := range children {
		nc := &node{
			id: c,
		}
		nc.copySource(t, tp, stem)
		t.nodes[nc.id] = nc
	}

	nAge := t.t.Age(n.id)

	// post-split
	prev := nAge + stem
	if !t.t.IsRoot(n.id) {
		prev = t.t.Age(t.t.Parent(n.id))
	}
	n.stages = append(n.stages, &timeStage{
		node: n,
		age:  prev,
	})

	// add time stage
	for a := tp.ClosestStageAge(prev - 1); a > nAge; a = tp.ClosestStageAge(a - 1) {
		ts := &timeStage{
			node:     n,
			age:      a,
			duration: float64(prev-a) / millionYears,
		}
		n.stages = append(n.stages, ts)
		prev = a
	}

	// at split or a terminal
	ts := &timeStage{
		node:     n,
		isTerm:   t.t.IsTerm(n.id),
		age:      nAge,
		duration: float64(prev-nAge) / millionYears,
	}
	n.stages = append(n.stages, ts)
}

func (n *node) setPDF(pix *earth.Pixelation, lambda float64) {
	n.lambda = lambda
	for _, ts := range n.stages {
		if ts.duration == 0 {
			continue
		}

		ts.pdf = dist.NewNormal(lambda/ts.duration, pix)
	}
}

// A TimeStage is a branch segment at a given time stage.
type timeStage struct {
	node   *node
	isTerm bool

	age      int64
	duration float64

	// likelihood at each pixel
	logLike map[int]float64

	// scaled likelihood (not in log-form)
	// updated with the destination prior
	scaled map[int]float64

	// store particle locations
	particles []SrcDest

	pdf dist.Normal
}
