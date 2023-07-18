// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package diffusion

import (
	"math"
	"sync"
	"time"

	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"golang.org/x/exp/rand"
	"golang.org/x/exp/slices"
)

func init() {
	rand.Seed(uint64(time.Now().UnixNano()))
}

// Mapping stores the result of an stochastic mapping
// simulation.
type Mapping struct {
	// Name of the tree
	Name string

	// A map of node IDs to a node reconstruction
	mu    sync.Mutex
	nodes map[int]*NodeMap
}

func (m *Mapping) Node(id int) *NodeMap {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.nodes[id]
}

func (m *Mapping) Nodes() []int {
	m.mu.Lock()
	defer m.mu.Unlock()

	nodes := make([]int, 0, len(m.nodes))
	for _, n := range m.nodes {
		nodes = append(nodes, n.ID)
	}
	slices.Sort(nodes)

	return nodes
}

// NodeMap contains the results of an stochastic mapping
// in a particular node.
type NodeMap struct {
	// ID of the node
	ID int

	// Stages is a map of age stages
	// to the pixels at that particular stage age
	// (in million years)
	Stages map[int64]SrcDest
}

// SrcDest contains the pixels of a particle simulation.
type SrcDest struct {
	// ID of the source pixel
	From int

	// ID of the destination pixel
	To int
}

// Simulate performs an stochastic mapping
// simulation.
func (t *Tree) Simulate() *Mapping {
	m := &Mapping{
		Name:  t.Name(),
		nodes: make(map[int]*NodeMap, len(t.nodes)),
	}

	// pick source at the root
	root := t.nodes[t.t.Root()]
	ts := root.stages[0]
	// get a rotation if change in stage
	age := t.rot.ClosestStageAge(ts.age)
	next := t.rot.ClosestStageAge(ts.age - 1)
	var rot *model.Rotation
	var tp map[int]int
	if age != next {
		rot = t.rot.OldToYoung(age)
		tp = t.landscape.Stage(rot.To)
	}

	// get the source pixel probabilities
	pixels := make([]int, 0, len(ts.logLike))
	max := -math.MaxFloat64
	for px, p := range ts.logLike {
		if rot != nil {
			pxs := rot.Rot[px]
			if len(pxs) == 0 {
				continue
			}
		}
		if p > max {
			max = p
		}
		pixels = append(pixels, px)
	}

	// pick a source pixel
	var source int
	for {
		px := pixels[rand.Intn(len(pixels))]
		accept := math.Exp(ts.logLike[px] - max)
		if rand.Float64() < accept {
			source = px
			break
		}
	}

	// rotate if change in stage
	if rot != nil {
		pxs := rot.Rot[source]
		source = pxs[0]
		if len(pxs) > 1 {
			// pick one of the pixels at random
			// based on the prior
			var max float64
			for _, px := range pxs {
				prior := t.pp.Prior(tp[px])
				if prior > max {
					max = prior
				}
			}
			for {
				px := pxs[rand.Intn(len(pxs))]
				accept := t.pp.Prior(tp[px]) / max
				if rand.Float64() < accept {
					source = px
					break
				}
			}
		}
	}

	// make simulation
	root.simulate(t, m, source)

	return m
}

func (n *node) simulate(t *Tree, m *Mapping, source int) {
	nm := &NodeMap{
		ID:     n.id,
		Stages: make(map[int64]SrcDest, len(n.stages)-1),
	}

	for i := 1; i < len(n.stages); i++ {
		ts := n.stages[i]

		var rot *model.Rotation
		if !ts.isTerm {
			age := t.rot.ClosestStageAge(ts.age)
			next := t.rot.ClosestStageAge(ts.age - 1)
			if age != next {
				rot = t.rot.OldToYoung(age)
			}
		}

		sd := ts.simulation(t, rot, source, n.pixTmp)
		nm.Stages[ts.age] = sd
		source = sd.To

		if ts.isTerm {
			continue
		}
		if rot == nil {
			continue
		}

		// rotate if change in stage
		pxs := rot.Rot[source]
		source = pxs[0]
		if len(pxs) > 1 {
			// pick one of the pixels at random
			// based on the prior
			tp := t.landscape.Stage(t.landscape.ClosestStageAge(rot.To))
			var max float64
			for _, px := range pxs {
				prior := t.pp.Prior(tp[px])
				if prior > max {
					max = prior
				}
			}
			for {
				px := pxs[rand.Intn(len(pxs))]
				accept := t.pp.Prior(tp[px]) / max
				if rand.Float64() < accept {
					source = px
					break
				}
			}
		}
	}
	m.mu.Lock()
	m.nodes[n.id] = nm
	m.mu.Unlock()

	children := t.t.Children(n.id)
	if len(children) == 0 {
		return
	}

	var wg sync.WaitGroup
	for _, cID := range children {
		c := t.nodes[cID]
		var d []likePix
		wg.Add(1)
		go func(c *node, d []likePix) {
			c.simulate(t, m, source)
			wg.Done()
		}(c, d)
	}
	wg.Wait()
}

func (ts *timeStage) simulation(t *Tree, rot *model.Rotation, source int, density []likePix) SrcDest {
	tp := t.landscape
	pix := tp.Pixelation()

	tpv := tp.Stage(tp.ClosestStageAge(ts.age))

	// calculates the density for the destination pixels
	density = density[:0]
	max := -math.MaxFloat64

	if t.dm != nil {
		// use distance matrix
		for px, p := range ts.logLike {
			if rot != nil {
				// skip pixels that are invalid in the next stage rotation
				pxs := rot.Rot[px]
				if len(pxs) == 0 {
					continue
				}
			}
			prior, ok := t.logPrior[tpv[px]]
			if !ok {
				continue
			}
			p += ts.pdf.LogProbRingDist(t.dm.At(source, px)) + prior
			density = append(density, likePix{
				px:   px,
				like: p,
			})
			if p > max {
				max = p
			}
		}
	} else {
		pt1 := pix.ID(source).Point()
		for px, p := range ts.logLike {
			if rot != nil {
				// skip pixels that are invalid in the next stage rotation
				pxs := rot.Rot[px]
				if len(pxs) == 0 {
					continue
				}
			}
			prior, ok := t.logPrior[tpv[px]]
			if !ok {
				continue
			}

			pt2 := pix.ID(px).Point()
			dist := earth.Distance(pt1, pt2)
			p += ts.pdf.LogProb(dist) + prior
			density = append(density, likePix{
				px:   px,
				like: p,
			})
			if p > max {
				max = p
			}
		}
	}

	// Pick a random pixel taking into account
	// the density for the destination.
	for {
		i := rand.Intn(len(density))
		accept := math.Exp(density[i].like - max)
		if rand.Float64() < accept {
			return SrcDest{
				From: source,
				To:   density[i].px,
			}
		}
	}
}
