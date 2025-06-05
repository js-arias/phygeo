// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walk

import (
	"math"
	"sync"
)

func (n *node) fullDownPass(t *Tree, tmpLike, resLike [][]float64) {
	for _, c := range t.t.Children(n.id) {
		nc := t.nodes[c]
		nc.fullDownPass(t, tmpLike, resLike)
	}
	n.conditional(t, tmpLike, resLike)
}

func (n *node) conditional(t *Tree, tmpLike, resLike [][]float64) {
	if !t.t.IsTerm(n.id) {
		// In an split node
		// the conditional likelihood is the product
		// of the conditional likelihoods of each descendant
		desc := t.t.Children(n.id)
		ts := n.stages[len(n.stages)-1]
		age := t.landProb.tp.ClosestStageAge(ts.age)
		t.landProb.prepareStage(age)

		// check for the minimum value
		// as we use sampling,
		// it is possible that some pixels are "unassigned"
		// with an infinity value
		var min float64
		for _, d := range desc {
			c := t.nodes[d]
			for _, tr := range c.stages[0].logLike {
				for _, l := range tr {
					if math.IsInf(l, 0) {
						continue
					}
					if l < min {
						min = l
					}
				}
			}
		}
		min -= 50000 // add an small, very unlikely value
		logLike := make([][]float64, len(t.landProb.traits))
		for i := range logLike {
			stage := t.landProb.stage(age, i)
			logLike[i] = make([]float64, t.landProb.tp.Pixelation().Len())
			for j, d := range desc {
				c := t.nodes[d]
				if j == 0 {
					for px, l := range c.stages[0].logLike[i] {
						if stage.prior[px] == 0 {
							logLike[i][px] = math.Inf(-1)
							continue
						}
						if math.IsInf(l, 0) {
							logLike[i][px] = min
							continue
						}
						logLike[i][px] = l
					}
					continue
				}
				for px, l := range c.stages[0].logLike[i] {
					if stage.prior[px] == 0 {
						continue
					}
					if math.IsInf(l, 0) {
						l = min
					}
					logLike[i][px] += l
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

		repeat := true
		for tm := 1; repeat; tm++ {
			repeat = false
			logLike := next.conditional(t, tmpLike, resLike, tm)
			// Rotate if there is an stage change
			if nextAge != age {
				rot := t.rot.YoungToOld(nextAge)
				like := make([][]float64, len(logLike))
				for i := range like {
					l := make([]float64, t.landProb.tp.Pixelation().Len())
					if i == 0 {
						for px := range l {
							l[px] = math.Inf(-1)
						}
						like[i] = l
						continue
					}
					copy(l, like[0])
					like[i] = l
				}
				rotation(rot.Rot, logLike, like)
				ts.logLike = like
				repeat = !hasSampled(ts.logLike)
				continue
			}

			like := make([][]float64, len(logLike))
			for i := range like {
				like[i] = make([]float64, len(logLike[i]))
				copy(like[i], logLike[i])
			}
			ts.logLike = like
			repeat = !hasSampled(ts.logLike)
		}
	}

	if t.t.IsRoot(n.id) {
		// set pixel priors
		rs := n.stages[0]
		age := t.landProb.tp.ClosestStageAge(rs.age)
		for i, tr := range rs.logLike {
			stage := t.landProb.stage(age, i)
			var sum float64
			for _, p := range stage.prior {
				sum += p
			}
			logSum := math.Log(sum)
			for px := range tr {
				rs.logLike[i][px] += math.Log(stage.prior[px]) - logSum
			}
		}
	}
}

// pixel blocks
const pixBlocks = 500

// Conditional calculates the conditional likelihood
// of a time stage.
func (ts *timeStage) conditional(t *Tree, tmpLike, resLike [][]float64, times int) [][]float64 {
	age := t.landProb.tp.ClosestStageAge(ts.age)
	numPix := t.landProb.tp.Pixelation().Len()

	t.landProb.prepareStage(age)
	max := scaleLogPix(ts.logLike, tmpLike)

	var wg sync.WaitGroup
	for i := 0; i < numPix; i += pixBlocks {
		wg.Add(1)
		e := min(i+pixBlocks, numPix)
		go func(start, end int) {
			likeChan <- likeChanType{
				start:     start,
				end:       end,
				like:      resLike,
				scaleProb: tmpLike,
				maxLn:     max,
				w:         t.landProb,
				age:       age,
				steps:     ts.steps,
				walkers:   t.walkers,
				times:     times,
				wg:        &wg,
			}
		}(i, e)
	}
	wg.Wait()

	return resLike
}

func scaleLogPix(like, scale [][]float64) float64 {
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

func hasSampled(logLike [][]float64) bool {
	for _, t := range logLike {
		for _, l := range t {
			if l > math.Inf(-1) {
				return true
			}
		}
	}
	return false
}
