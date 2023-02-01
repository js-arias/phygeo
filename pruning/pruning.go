// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package pruning implements an approximation
// of the Felsenstein's pruning algorithm
// for biogeographic analysis.
package pruning

import (
	"github.com/js-arias/earth/model"
	"github.com/js-arias/timetree"
)

// Param is a collection of parameters
// for the initialization of a tree.
type Param struct {
	// TimePixelation
	TP *model.TimePix

	// Length in years of the stem node
	Stem int64
}

// A Tree os a phylogenetic tree for biogeography.
type Tree struct {
	t     *timetree.Tree
	nodes map[int]*node
}

// New creates a new tree by copying the indicated source tree.
func New(t *timetree.Tree, p Param) *Tree {
	nt := &Tree{
		t:     t,
		nodes: make(map[int]*node, len(t.Nodes())),
	}
	root := &node{
		id: t.Root(),
	}
	nt.nodes[root.id] = root
	root.copySource(nt, p.TP, p.Stem)

	return nt
}

// Name returns the name of the tree.
func (t *Tree) Name() string {
	return t.t.Name()
}

// A Node is a node in a phylogenetic tree.
type node struct {
	id     int
	stages []*timeStage
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
	for a := tp.CloserStageAge(prev - 1); a > nAge; a = tp.CloserStageAge(a - 1) {
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

// A TimeStage is a branch segment at a given time stage.
type timeStage struct {
	node   *node
	isTerm bool

	age      int64
	duration float64
}
