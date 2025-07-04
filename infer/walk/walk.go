// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package walk implements a spherical random walk
// approximated using a discrete isolatitude pixelation
// for a phylogenetic biogeography analysis.
package walk

import (
	"math"
	"slices"

	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/pixkey"
	"github.com/js-arias/phygeo/cats"
	"github.com/js-arias/phygeo/timestage"
	"github.com/js-arias/phygeo/trait"
	"github.com/js-arias/ranges"
	"github.com/js-arias/timetree"
)

// Param is a collection of parameters
// for the initialization of a tree.
type Param struct {
	// Paleolandscape model
	Landscape *model.TimePix

	// Stage rotation model
	Rot *model.StageRot

	// Stages is the time stages used to split branches
	Stages []int64

	// Network
	Net earth.Network

	// Ranges is the collection of terminal ranges
	Ranges *ranges.Collection

	// Traits is the collection of terminal traits
	Traits *trait.Data

	// Keys are the keys to relate trait data with the landscape
	Keys *pixkey.PixKey

	// Movement and Settlement are the movement
	// and settlement matrices.
	Movement   *trait.Matrix
	Settlement *trait.Matrix

	// Length in years of the stem node
	Stem int64

	// Steps is the number of average steps per million year
	Steps float64

	// Minimum and maximum number of steps in a branch.
	MinSteps int
	MaxSteps int

	// Walkers is the number of particles per step category
	Walkers int

	// Discrete is the discretized function for the step categories
	Discrete cats.Discrete
}

// A Tree is a phylogenetic tree for biogeography.
type Tree struct {
	t     *timetree.Tree
	nodes map[int]*node

	rot      *model.StageRot
	landProb *walkModel

	steps   float64
	dd      cats.Discrete
	walkers int
}

// New creates a new tree by copying the indicated source tree.
func New(t *timetree.Tree, p Param) *Tree {
	states := p.Traits.States()
	landProb := &walkModel{
		stages:     make(map[int64][]stageProb),
		tp:         p.Landscape,
		net:        p.Net,
		movement:   p.Movement,
		settlement: p.Settlement,
		traits:     states,
		key:        p.Keys,
	}

	nt := &Tree{
		t:        t,
		nodes:    make(map[int]*node, len(t.Nodes())),
		rot:      p.Rot,
		landProb: landProb,
		steps:    p.Steps,
		dd:       p.Discrete,
		walkers:  p.Walkers,
	}

	root := &node{
		id: t.Root(),
	}
	nt.nodes[root.id] = root
	root.copySource(nt, p.Stem, p.Stages)

	// Prepare nodes and time stages
	for _, n := range nt.nodes {
		n.setSteps(p.Steps, p.MinSteps, p.MaxSteps, nt.dd.Cats())

		if !nt.t.IsTerm(n.id) {
			continue
		}

		// last terminal stage
		st := n.stages[len(n.stages)-1]

		tx := nt.t.Taxon(n.id)

		rng := p.Ranges.Range(tx)
		var sum float64
		for _, p := range rng {
			sum += p
		}
		obs := p.Traits.Obs(tx)
		sum *= float64(len(obs))

		st.logLike = make([][]float64, len(states))
		for i, t := range states {
			like := make([]float64, nt.landProb.tp.Pixelation().Len())
			isObs := slices.Contains(obs, t)
			for px := range like {
				like[px] = math.Inf(-1)
				if !isObs {
					continue
				}
				if p, ok := rng[px]; ok {
					like[px] = math.Log(p) - math.Log(sum)
				}
			}
			st.logLike[i] = like
		}
	}

	return nt
}

// Conditional returns the conditional likelihood
// for a given node
// at a given time stage
// (in years)
// with a given trait.
// The conditional likelihood is returned as a map of pixels
// to the logLikelihood of the pixels.
func (t *Tree) Conditional(n int, age int64, tr string) map[int]float64 {
	nn, ok := t.nodes[n]
	if !ok {
		return nil
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
		return nil
	}

	ts := nn.stages[i]

	j, ok := slices.BinarySearch(t.landProb.traits, tr)
	if !ok {
		return nil
	}
	cLike := make(map[int]float64, len(ts.logLike[j]))
	for px, p := range ts.logLike[j] {
		if math.IsInf(p, 0) {
			continue
		}
		cLike[px] = p
	}

	return cLike
}

// Discrete returns the discrete distribution
// used for the step categories.
func (t *Tree) Discrete() cats.Discrete {
	return t.dd
}

// DownPass performs the Felsenstein's pruning algorithm
// to estimate the likelihood of the data
// for a tree.
func (t *Tree) DownPass() float64 {
	tmpLike := make([][]float64, len(t.landProb.traits))
	resLike := make([][]float64, len(t.landProb.traits))
	for i := range tmpLike {
		pixSize := t.landProb.tp.Pixelation().Len()
		tmpLike[i] = make([]float64, pixSize)
		resLike[i] = make([]float64, pixSize)
	}

	root := t.nodes[t.t.Root()]
	root.fullDownPass(t, tmpLike, resLike)

	return t.LogLike()
}

// Equator returns the number of pixels in the equator
// of the underlying pixelation.
func (t *Tree) Equator() int {
	return t.landProb.tp.Pixelation().Equator()
}

// IsRoot returns true
// if the indicated node is the root of the tree.
func (t *Tree) IsRoot(id int) bool {
	return t.t.IsRoot(id)
}

// LogLike returns the logLikelihood of the whole reconstruction
// in the most basal stem time stage node.
func (t *Tree) LogLike() float64 {
	root := t.nodes[t.t.Root()]
	rs := root.stages[0]

	max := math.Inf(-1)
	for _, tr := range rs.logLike {
		for _, l := range tr {
			if l > max {
				max = l
			}
		}
	}

	var sum float64
	for _, tr := range rs.logLike {
		for _, l := range tr {
			sum += math.Exp(l - max)
		}
	}
	return math.Log(sum) + max
}

// Name returns the name of the tree.
func (t *Tree) Name() string {
	return t.t.Name()
}

// Nodes returns an slice with IDs
// of the nodes of the tree.
func (t *Tree) Nodes() []int {
	return t.t.Nodes()
}

// NumCats returns the number of categories
// used during search.
func (t *Tree) NumCats() int {
	return len(t.dd.Cats())
}

// Path returns the path from a stochastic mapping
// or a simulation
// of a given node,
// at a given time stage
// (in years),
// for a given particle.
func (t *Tree) Path(particle, n int, age int64) Path {
	nn, ok := t.nodes[n]
	if !ok {
		return Path{From: -1}
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
		return Path{From: -1}
	}
	st := nn.stages[i]

	if particle >= len(st.paths) {
		return Path{From: -1}
	}
	p := st.paths[particle]
	path := make([]int, len(p.Path))
	copy(path, p.Path)
	return Path{
		From:       p.From,
		To:         p.To,
		TraitStart: p.TraitStart,
		TraitEnd:   p.TraitEnd,
		Cat:        p.Cat,
		Path:       path,
	}
}

// Pixels returns the number of pixels in the underlying pixelation.
func (t *Tree) Pixels() int {
	return t.landProb.tp.Pixelation().Len()
}

// SetConditional sets the conditional likelihood
// (in logLike units)
// of a node at a given time stage.
func (t *Tree) SetConditional(n int, age int64, logLike map[string]map[int]float64) {

	nn, ok := t.nodes[n]
	if !ok {
		return
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
		return
	}

	ts := nn.stages[i]
	if ts.logLike == nil {
		ts.logLike = make([][]float64, len(t.landProb.traits))
		for i := range ts.logLike {
			like := make([]float64, t.landProb.tp.Pixelation().Len())
			for px := range like {
				like[px] = math.Inf(-1)
			}
			ts.logLike[i] = like
		}
	}

	for i, tr := range t.landProb.traits {
		trLike, ok := logLike[tr]
		if !ok {
			continue
		}
		for px, v := range trLike {
			ts.logLike[i][px] = v
		}
	}
}

// Stages returns the age of the time stages of a node
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

// Steps returns the base number of steps
func (t *Tree) Steps() float64 {
	return t.steps
}

// Traits returns the names of the traits defined for the terminals
// of a tree.
func (t *Tree) Traits() []string {
	tr := make([]string, len(t.landProb.traits))
	copy(tr, t.landProb.traits)
	return tr
}

// A Node is a node in a phylogenetic tree.
type node struct {
	id     int
	stages []*timeStage
}

func (n *node) copySource(t *Tree, stem int64, stages []int64) {
	children := t.t.Children(n.id)
	for _, c := range children {
		nc := &node{
			id: c,
		}
		nc.copySource(t, stem, stages)
		t.nodes[nc.id] = nc
	}

	nAge := t.t.Age(n.id)

	// post-split
	prev := nAge + stem
	if !t.t.IsRoot(n.id) || stem > 0 {
		if !t.t.IsRoot(n.id) {
			prev = t.t.Age(t.t.Parent(n.id))
		}
		n.stages = append(n.stages, &timeStage{
			node: n,
			age:  prev,
		})
	}

	// add time stages
	for i := len(stages) - 1; i >= 0; i-- {
		a := stages[i]
		if a >= prev {
			continue
		}
		if a <= nAge {
			break
		}
		ts := &timeStage{
			node:     n,
			age:      a,
			duration: float64(prev-a) / timestage.MillionYears,
		}
		n.stages = append(n.stages, ts)
		prev = a
	}

	// at split or a terminal
	ts := &timeStage{
		node:     n,
		isTerm:   t.t.IsTerm(n.id),
		age:      nAge,
		duration: float64(prev-nAge) / timestage.MillionYears,
	}
	n.stages = append(n.stages, ts)
}

func (n *node) setSteps(steps float64, min, max int, cats []float64) {
	for _, ts := range n.stages {
		if ts.duration == 0 {
			continue
		}
		ts.steps = make([]int, 0, len(cats))

		for _, c := range cats {
			s := int(math.Round(steps * ts.duration * c))
			if s < min {
				s = min
			}
			if s > max {
				s = max
			}
			ts.steps = append(ts.steps, s)
		}
	}
}

// A TimeStage is a branch segment at a given time stage.
type timeStage struct {
	node   *node
	isTerm bool

	age      int64
	duration float64

	// logLikelihood of each trait-pixel
	logLike [][]float64

	// path store stochastic map paths
	paths []*Path

	steps []int
}
