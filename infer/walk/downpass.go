// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walk

import (
	"math"
	"sync"
)

func (n *node) fullDownPass(t *Tree) {
	var wg sync.WaitGroup
	for _, c := range t.t.Children(n.id) {
		wg.Add(1)
		go func(c int) {
			nc := t.nodes[c]
			nc.fullDownPass(t)
			wg.Done()
		}(c)
	}
	wg.Wait()
	n.conditional(t)
}

func (n *node) conditional(t *Tree) {
	tmpLike := make([][][]float64, len(t.landProb))
	for i := range tmpLike {
		tmpLike[i] = make([][]float64, len(t.landProb[i].traits))
		for j := range tmpLike[i] {
			tmpLike[i][j] = make([]float64, t.tp.Pixelation().Len())
		}
	}

	if !t.t.IsTerm(n.id) {
		// In a split node
		// the conditional likelihood is the product
		// of the conditional likelihoods of each descendant
		logLike := make([][][]float64, len(t.landProb))
		for i := range logLike {
			logLike[i] = make([][]float64, len(t.landProb[i].traits))
			for j := range logLike[i] {
				logLike[i][j] = make([]float64, t.tp.Pixelation().Len())
			}
		}

		logNumCats := math.Log(float64(len(t.landProb)))
		desc := t.t.Children(n.id)
		for _, d := range desc {
			// first we need to add the categories
			c := t.nodes[d].stages[0]
			max := scaleLogProb(tmpLike, c.logLike)
			for c := range tmpLike {
				if c == 0 {
					continue
				}
				for tr := range tmpLike[c] {
					for px, p := range tmpLike[c][tr] {
						tmpLike[0][tr][px] += p
					}
				}
			}

			// then multiply the values of each descendant
			// as all categories should have the same conditionals
			// we only use the first category
			for tr := range tmpLike[0] {
				for px, p := range tmpLike[0][tr] {
					logLike[0][tr][px] += math.Log(p) + max - logNumCats
				}
			}
		}

		// now we copy the values of the first cat in all categories
		for c := range logLike {
			if c == 0 {
				continue
			}
			for tr := range logLike[c] {
				copy(logLike[c][tr], logLike[0][tr])
			}
		}

		ts := n.stages[len(n.stages)-1]
		ts.logLike = logLike
	}

	// internodes
	for i := len(n.stages) - 2; i >= 0; i-- {
		ts := n.stages[i]
		age := t.rot.ClosestStageAge(ts.age)
		next := n.stages[i+1]
		nextAge := t.rot.ClosestStageAge(next.age)

		logLike := next.conditional(t, tmpLike)
		if nextAge != age {
			// rotate if there is an stage change
			rot := t.rot.YoungToOld(nextAge)
			for c := range logLike {
				for tr := range logLike[c] {
					copy(tmpLike[c][tr], logLike[c][tr])
					for px := range logLike[c][tr] {
						logLike[c][tr][px] = math.Inf(-1)
					}
				}
			}
			rotation(rot.Rot, logLike, tmpLike)
		}
		ts.logLike = logLike
	}

	if t.t.IsRoot(n.id) {
		// At the root add the pixel priors
		// and divide by the number of categories
		rs := n.stages[0]
		age := t.tp.ClosestStageAge(rs.age)
		logNumCats := math.Log(float64(len(t.landProb)))
		for c := range rs.logLike {
			for tr := range rs.logLike[c] {
				stage := t.landProb[c].stage(age, tr)
				for px := range rs.logLike[c][tr] {
					pp := stage.prior[px]
					if pp == 0 {
						// remove un-settable pixels
						rs.logLike[c][tr][px] = math.Inf(-1)
						continue
					}
					rs.logLike[c][tr][px] += math.Log(pp) - logNumCats
				}
			}
		}
	}
}

// Conditional calculates the conditional likelihood
// of a time stage.
func (ts *timeStage) conditional(t *Tree, tmpLike [][][]float64) [][][]float64 {
	resLike := make([][][]float64, len(t.landProb))
	for i := range resLike {
		resLike[i] = make([][]float64, len(t.landProb[i].traits))
		for j := range resLike[i] {
			resLike[i][j] = make([]float64, t.tp.Pixelation().Len())
		}
	}

	age := t.tp.ClosestStageAge(ts.age)
	for i := range t.landProb {
		t.landProb[i].prepareStage(age)
	}
	max := scaleLogProb(tmpLike, ts.logLike)

	answer := make(chan likeChanAnswer)
	go func() {
		for i := range t.landProb {
			likeChan <- likeChanType{
				like:   tmpLike[i],
				raw:    resLike[i],
				w:      t.landProb[i],
				age:    age,
				cat:    i,
				steps:  ts.steps,
				answer: answer,
			}
		}
	}()

	for range t.landProb {
		a := <-answer
		resLike[a.cat] = a.rawLike
		for tr := range resLike[a.cat] {
			stage := t.landProb[a.cat].stage(age, tr)
			for px, p := range resLike[a.cat][tr] {
				pp := stage.prior[px]
				if pp == 0 {
					// remove un-settable pixels
					resLike[a.cat][tr][px] = math.Inf(-1)
					continue
				}
				resLike[a.cat][tr][px] = math.Log(p) + max
			}
		}
	}
	close(answer)
	return resLike
}

func scaleLogProb(dst, src [][][]float64) float64 {
	max := math.Inf(-1)
	for _, c := range src {
		for _, t := range c {
			for _, l := range t {
				if l > max {
					max = l
				}
			}
		}
	}

	// scale the values
	for c := range src {
		for t := range src[c] {
			for px, l := range src[c][t] {
				dst[c][t][px] = math.Exp(l - max)
			}
		}
	}

	return max
}
