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

	// Lambda is the concentration parameter per million years
	// in 1/radian units
	Lambda float64

	// Length in years of the stem node
	Stem int64

	// Steps is the number of steps per million year
	Steps int

	// Minimum number of steps in a branch.
	MinSteps int

	// Discrete contains the values for the settlement probability
	// for the diffusion categories.
	Discrete []float64

	// Number of particles used for stochastic mapping
	Particles int
}

// A Tree is a phylogenetic tree for biogeography.
type Tree struct {
	t     *timetree.Tree
	nodes map[int]*node

	rot      *model.StageRot
	tp       *model.TimePix
	landProb []*walkModel

	steps     int
	dd        []float64
	particles int
}

// New creates a new tree by copying the indicated source tree.
func New(t *timetree.Tree, p Param) *Tree {
	states := p.Traits.States()
	landProb := make([]*walkModel, len(p.Discrete))
	for i, c := range p.Discrete {
		lp := &walkModel{
			stages:     make(map[int64][]stageProb),
			tp:         p.Landscape,
			net:        p.Net,
			movement:   p.Movement,
			settlement: p.Settlement,
			settProb:   c,
			traits:     states,
			key:        p.Keys,
		}
		landProb[i] = lp
	}

	nt := &Tree{
		t:         t,
		nodes:     make(map[int]*node, len(t.Nodes())),
		rot:       p.Rot,
		tp:        p.Landscape,
		landProb:  landProb,
		steps:     p.Steps,
		dd:        p.Discrete,
		particles: p.Particles,
	}

	root := &node{
		id: t.Root(),
	}
	nt.nodes[root.id] = root
	root.copySource(nt, p.Stem, p.Stages)

	// Prepare nodes and time stages
	for _, n := range nt.nodes {
		n.setSteps(nt, p.Steps, p.MinSteps)

		// add observed ranges
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

		st.logLike = make([][][]float64, len(nt.dd))

		for c := range st.logLike {
			st.logLike[c] = make([][]float64, len(nt.landProb[c].traits))
			for tr := range st.logLike[c] {
				like := make([]float64, p.Landscape.Pixelation().Len())
				isObs := slices.Contains(obs, states[tr])
				for px := range like {
					like[px] = math.Inf(-1)
					if !isObs {
						continue
					}
					if p, ok := rng[px]; ok {
						like[px] = math.Log(p) - math.Log(sum)
					}
				}
				st.logLike[c][tr] = like
			}
		}
	}
	return nt
}

// Age returns the age of a given node.
func (t *Tree) Age(n int) int64 {
	return t.t.Age(n)
}

// Cats returns the settlement probability
// for each relaxed diffusion category.
func (t *Tree) Cats() []float64 {
	return t.dd
}

// Conditional returns the conditional likelihood
// for a given node
// at a given time stage
// (in years)
// in the given diffusion category
// with a given trait.
// The conditional likelihood is returned as a map of pixels
// to the logLikelihood of the pixels.
func (t *Tree) Conditional(n int, age int64, cat int, tr string) map[int]float64 {
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
	if cat < 0 || cat >= len(t.landProb) {
		return nil
	}

	j, ok := slices.BinarySearch(t.landProb[cat].traits, tr)
	if !ok {
		return nil
	}
	cLike := make(map[int]float64, len(ts.logLike[cat][j]))
	for px, p := range ts.logLike[cat][j] {
		if math.IsInf(p, 0) {
			continue
		}
		cLike[px] = p
	}

	return cLike
}

// DownPass performs the Felsenstein's pruning algorithm
// to estimate the likelihood of the data
// for a tree.
func (t *Tree) DownPass() float64 {
	root := t.nodes[t.t.Root()]
	root.fullDownPass(t)

	return t.LogLike()
}

// Equator returns the number of pixels in the equator
// of the underlying pixelation.
func (t *Tree) Equator() int {
	return t.tp.Pixelation().Equator()
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
	for _, c := range rs.logLike {
		for _, tr := range c {
			for _, l := range tr {
				if l > max {
					max = l
				}
			}
		}
	}

	var sum float64
	for _, c := range rs.logLike {
		for _, tr := range c {
			for _, l := range tr {
				sum += math.Exp(l - max)
			}
		}
	}
	return math.Log(sum) + max
}

// Mapping performs an stochastic mapping.
func (t *Tree) Mapping() {
	root := t.nodes[t.t.Root()]
	root.fullMap(t)
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

// Parent returns the ID of the parent node
// of the indicated node.
func (t *Tree) Parent(n int) int {
	return t.t.Parent(n)
}

// Path returns a particle path
// for a given node
// at a given time stage
// (in years)
// and a given particle.
func (t *Tree) Path(n int, age int64, p int) Path {
	nn, ok := t.nodes[n]
	if !ok {
		return Path{}
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
		return Path{}
	}

	ts := nn.stages[i]
	return ts.paths[p]
}

// Pixels returns the number of pixels in the underlying pixelation.
func (t *Tree) Pixels() int {
	return t.tp.Pixelation().Len()
}

// SetConditional sets the conditional likelihood
// (in logLike units)
// of a node at a given time stage,
// rate category
// and trait.
func (t *Tree) SetConditional(n int, age int64, cat int, tr string, logLike map[int]float64) {
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
	if cat < 0 || cat >= len(t.landProb) {
		return
	}

	j, ok := slices.BinarySearch(t.landProb[cat].traits, tr)
	if !ok {
		return
	}

	// if there are no assigned log Likelihoods,
	// create the arrays
	if ts.logLike == nil {
		tmpLike := make([][][]float64, len(t.landProb))
		for c := range tmpLike {
			tmpLike[c] = make([][]float64, len(t.landProb[c].traits))
			for trv := range tmpLike[c] {
				tmpLike[c][trv] = make([]float64, t.tp.Pixelation().Len())
				for px := range tmpLike[c][trv] {
					tmpLike[c][trv][px] = math.Inf(-1)
				}
			}
		}
		ts.logLike = tmpLike
	}

	for px, p := range logLike {
		ts.logLike[cat][j][px] = p
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

// StageSteps returns the number of steps
// of a given node
// at a given time stage
// (in years).
func (t *Tree) StageSteps(n int, age int64) int {
	nn, ok := t.nodes[n]
	if !ok {
		return 0
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
		return 0
	}

	ts := nn.stages[i]
	return ts.steps
}

// Steps returns the number of steps
// per million years.
func (t *Tree) Steps() int {
	return t.steps
}

// Traits returns the names of the traits defined for the terminals
// of a tree.
func (t *Tree) Traits() []string {
	tr := make([]string, len(t.landProb[0].traits))
	copy(tr, t.landProb[0].traits)
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

func (n *node) setSteps(t *Tree, steps, min int) {
	if t.t.IsTerm(n.id) {
		// for terminal nodes,
		// we check if the length of the branch is enough
		var sum float64
		for _, ts := range n.stages {
			sum += ts.duration
		}

		// if the length of the branch is too small
		// we set the length of the branch to its minimum length
		if st := int(float64(steps) * sum); (min > 0) && (st < min) {
			steps = int(math.Floor(float64(min)/sum)) + 1
		}
	}

	for _, ts := range n.stages {
		if ts.duration == 0 {
			continue
		}
		ts.steps = int((ts.duration) * float64(steps))
	}
}

// A TimeStage is a branch segment at a given time stage.
type timeStage struct {
	node   *node
	isTerm bool

	age      int64
	duration float64

	// conditional logLikelihood of each cat-trait-pixel
	logLike [][][]float64

	steps int

	// paths of the stochastic map particles
	paths []Path
}
