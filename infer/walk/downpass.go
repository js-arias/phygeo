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
	tmpLike := make([][]float64, len(t.landProb))
	for i := range tmpLike {
		tmpLike[i] = make([]float64, t.tp.Pixelation().Len())
	}

	if !t.t.IsTerm(n.id) {
		logLike := make([][]float64, len(t.landProb))
		for i := range logLike {
			logLike[i] = make([]float64, t.tp.Pixelation().Len())
		}

		ts := n.stages[len(n.stages)-1]
		age := t.tp.ClosestStageAge(ts.age)

		// In a split node
		// the conditional likelihood is the product
		// of the conditional likelihoods of each descendant
		for _, d := range t.t.Children(n.id) {
			c := t.nodes[d].stages[0]
			for tr := range c.logLike {
				for px, p := range c.logLike[tr] {
					logLike[tr][px] += p
				}
			}
		}

		// remove un-settable pixels
		for tr := range logLike {
			stage := t.landProb[tr].StageProb(age)
			for px := range logLike[tr] {
				pp := stage.Settlement[px]
				if pp == 0 {
					logLike[tr][px] = math.Inf(-1)
				}
			}
		}

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
			for tr := range logLike {
				copy(tmpLike[tr], logLike[tr])
				for px := range logLike[tr] {
					logLike[tr][px] = math.Inf(-1)
				}
			}
			rotation(rot.Rot, logLike, tmpLike)
		}
		ts.logLike = logLike
	}

	if t.t.IsRoot(n.id) {
		// Add the pixel priors
		rs := n.stages[0]
		age := t.tp.ClosestStageAge(rs.age)
		for tr := range rs.logLike {
			stage := t.landProb[tr].StageProb(age)
			for px := range rs.logLike[tr] {
				pp := stage.Prior[px]
				if pp == 0 {
					// remove un-settable pixels
					rs.logLike[tr][px] = math.Inf(-1)
					continue
				}
				rs.logLike[tr][px] += math.Log(pp)
			}
		}
	}
}

// Conditional calculates the conditional likelihood
// of a time stage.
func (ts *timeStage) conditional(t *Tree, tmpLike [][]float64) [][]float64 {
	resLike := make([][]float64, len(t.landProb))
	for i := range resLike {
		resLike[i] = make([]float64, t.tp.Pixelation().Len())
	}

	age := t.tp.ClosestStageAge(ts.age)
	max := scaleLogProb(tmpLike, ts.logLike)

	answer := make(chan likeChanAnswer)
	go func() {
		likeChan <- likeChanType{
			like:   tmpLike,
			raw:    resLike,
			w:      t.landProb,
			age:    age,
			steps:  ts.steps,
			answer: answer,
		}
	}()

	a := <-answer
	resLike = a.rawLike
	for tr := range resLike {
		for px, p := range resLike[tr] {
			resLike[tr][px] = math.Log(p) + max
		}
	}
	close(answer)
	return resLike
}

func scaleLogProb(dst, src [][]float64) float64 {
	max := math.Inf(-1)
	for _, t := range src {
		for _, l := range t {
			if l > max {
				max = l
			}
		}
	}

	// scale the values
	for t := range src {
		for px, l := range src[t] {
			dst[t][px] = math.Exp(l - max)
		}
	}

	return max
}
