// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walk

import (
	"math/rand/v2"
	"sync"

	"github.com/js-arias/earth/model"
)

// Path contains the dispersal path of a particle.
type Path struct {
	// ID of the source pixel
	From int

	// ID of the destination pixel
	To int

	// TraitStart starting trait
	TraitStart string

	// TraitEnd ending trait
	TraitEnd string

	// Number of the category
	Cat int

	// The path in pixels
	Path []int
}

func (n *node) upPass(t *Tree, pathChan chan pathChanType, density [][]float64, src, tr []int, cpu int) {
	n.stMap(t, pathChan, density, src, tr, cpu)
	for _, c := range t.t.Children(n.id) {
		nc := t.nodes[c]
		nc.upPass(t, pathChan, density, src, tr, cpu)
	}
}

func (n *node) stMap(t *Tree, pathChan chan pathChanType, density [][]float64, src, tr []int, cpu int) {
	if t.t.IsRoot(n.id) {
		rs := n.stages[0]
		age := t.rot.ClosestStageAge(rs.age)
		src, tr = pickRootParticles(rs.logLike, density, src, tr, t.rot.OldToYoung(age))

		rs.paths = make([]*Path, len(src))
		for i := range rs.paths {
			px := src[i]
			trV := tr[i]
			p := &Path{
				From:       px,
				To:         px,
				TraitStart: t.landProb.traits[trV],
				TraitEnd:   t.landProb.traits[trV],
			}
			rs.paths[i] = p
		}
		t.landProb.prepareStage(age)
		if hasToRot(t.rot, age) {
			src = rotPix(t.rot, t.landProb, age, src, tr)
		}
		if len(n.stages) == 1 {
			// copy the source in the descendant nodes
			for _, c := range t.t.Children(n.id) {
				nc := t.nodes[c]
				post := nc.stages[0]
				post.paths = make([]*Path, len(src))
				for i := range post.paths {
					px := src[i]
					trV := tr[i]
					p := &Path{
						From:       px,
						To:         px,
						TraitStart: t.landProb.traits[trV],
						TraitEnd:   t.landProb.traits[trV],
					}
					post.paths[i] = p
				}
			}
			return
		}
	} else {
		post := n.stages[0]
		for i, p := range post.paths {
			src[i] = p.To
		}
	}

	for i := 1; i < len(n.stages); i++ {
		ts := n.stages[i]
		ts.stMap(t, pathChan, density, src, tr, cpu)
		for i, p := range ts.paths {
			src[i] = p.To
		}

		if hasToRot(t.rot, ts.age) {
			src = rotPix(t.rot, t.landProb, ts.age, src, tr)
		}
	}

	// copy the source in the descendant nodes
	for _, c := range t.t.Children(n.id) {
		nc := t.nodes[c]
		post := nc.stages[0]
		post.paths = make([]*Path, len(src))
		for i := range post.paths {
			px := src[i]
			trV := tr[i]
			p := &Path{
				From:       px,
				To:         px,
				TraitStart: t.landProb.traits[trV],
				TraitEnd:   t.landProb.traits[trV],
			}
			post.paths[i] = p
		}
	}
}

// StMap calculates the stochastic mapping of a time stage.
func (ts *timeStage) stMap(t *Tree, pathChan chan pathChanType, density [][]float64, src, tr []int, cpu int) {
	age := t.landProb.tp.ClosestStageAge(ts.age)

	t.landProb.prepareStage(age)
	scaleLogPix(ts.logLike, density)

	var rot *model.Rotation
	if hasToRot(t.rot, ts.age) {
		rot = t.rot.OldToYoung(t.rot.ClosestStageAge(ts.age))
	}
	partBlock := len(src)/(2*cpu) + cpu

	ts.paths = make([]*Path, len(src))
	var wg sync.WaitGroup
	for i := 0; i < len(src); i += partBlock {
		wg.Add(1)
		e := min(i+pixBlocks, len(src))
		go func(start, end int) {
			pathChan <- pathChanType{
				start:   start,
				end:     end,
				src:     src,
				t:       tr,
				density: density,
				path:    ts.paths,
				w:       t.landProb,
				rot:     rot,
				age:     age,
				steps:   ts.steps,
				wg:      &wg,
			}
		}(i, e)
	}
	wg.Wait()
}

func pickRootParticles(logLike, density [][]float64, dst, tr []int, rot *model.Rotation) ([]int, []int) {
	scaleLogPix(logLike, density)
	for i := range dst {
		for {
			t := rand.IntN(len(density))
			d := density[t]
			px := rand.IntN(len(d))
			if rot != nil && len(rot.Rot[px]) == 0 {
				continue
			}
			p := d[px]
			if rand.Float64() < p {
				dst[i] = px
				tr[i] = t
				break
			}
		}
	}
	return dst, tr
}
