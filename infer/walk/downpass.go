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
	if !t.t.IsTerm(n.id) {

		// In a split node
		// the conditional likelihood is the product
		// of the conditional likelihoods of each descendant
		desc := t.t.Children(n.id)
		ts := n.stages[len(n.stages)-1]
		logLike := make([][]float64, len(t.landProb.traits))
		for i := range logLike {
			logLike[i] = make([]float64, t.landProb.tp.Pixelation().Len())
			for _, d := range desc {
				c := t.nodes[d]
				for px, l := range c.stages[0].logLike[i] {
					logLike[i][px] += l
				}
			}
		}
		ts.logLike = logLike
	}

	// internodes
	tmpLike := make([][]float64, len(t.landProb.traits))
	for i := range tmpLike {
		tmpLike[i] = make([]float64, t.landProb.tp.Pixelation().Len())
	}
	for i := len(n.stages) - 2; i >= 0; i-- {
		ts := n.stages[i]
		age := t.rot.ClosestStageAge(ts.age)
		next := n.stages[i+1]
		nextAge := t.rot.ClosestStageAge(next.age)

		logLike := next.conditional(t, tmpLike)
		if nextAge != age {
			// rotate if there is an stage change
			rot := t.rot.YoungToOld(nextAge)
			for i := range logLike {
				copy(tmpLike[i], logLike[i])
				for px := range logLike[i] {
					logLike[i][px] = math.Inf(-1)
				}
			}
			rotation(rot.Rot, tmpLike, logLike)
		}
		ts.logLike = logLike
	}

	if t.t.IsRoot(n.id) {
		// At the root add the pixel priors
		rs := n.stages[0]
		age := t.landProb.tp.ClosestStageAge(rs.age)
		for i := range rs.logLike {
			stage := t.landProb.stage(age, i)
			var sum float64
			for px := range rs.logLike[i] {
				var pp float64
				for _, nx := range stage.move[px] {
					if nx.id == px {
						pp = nx.prob
						break
					}
				}
				sum += pp
				if pp == 0 {
					// remove un-settable pixels
					rs.logLike[i][px] = math.Inf(-1)
					continue
				}
				rs.logLike[i][px] += math.Log(pp)
			}
			logSum := math.Log(sum)
			for px := range rs.logLike[i] {
				rs.logLike[i][px] -= logSum
			}
		}
	}
}

// Conditional calculates the conditional likelihood
// of a time stage.
func (ts *timeStage) conditional(t *Tree, tmpLike [][]float64) [][]float64 {
	resLike := make([][]float64, len(t.landProb.traits))
	for i := range resLike {
		resLike[i] = make([]float64, t.landProb.tp.Pixelation().Len())
	}

	age := t.landProb.tp.ClosestStageAge(ts.age)

	t.landProb.prepareStage(age)
	max := scaleLogProb(ts.logLike, tmpLike)

	answer := make(chan likeChanAnswer)
	go func() {
		for i := range t.landProb.traits {
			likeChan <- likeChanType{
				like:   tmpLike[i],
				raw:    resLike[i],
				w:      t.landProb,
				age:    age,
				tr:     i,
				steps:  ts.steps,
				answer: answer,
			}
		}
	}()

	logNumCats := math.Log(float64(len(ts.steps)))
	for range t.landProb.traits {
		a := <-answer
		resLike[a.tr] = a.rawLike
		stage := t.landProb.stage(age, a.tr)
		for px, p := range resLike[a.tr] {
			var pp float64
			for _, nx := range stage.move[px] {
				if nx.id == px {
					pp = nx.prob
					break
				}
			}
			if pp == 0 {
				// remove un-settable pixels
				resLike[a.tr][px] = math.Inf(-1)
				continue
			}
			resLike[a.tr][px] = math.Log(p) + max - logNumCats
		}
	}
	close(answer)
	return resLike
}

func scaleLogProb(like, scale [][]float64) float64 {
	max := math.Inf(-1)
	for _, t := range like {
		for _, l := range t {
			if l > max {
				max = l
			}
		}
	}

	// scale the values
	for i, t := range like {
		for j, l := range t {
			scale[i][j] = math.Exp(l - max)
		}
	}

	return max
}
