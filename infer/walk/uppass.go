// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walk

import (
	"math"
	"sync"
)

func (n *node) fullUpPass(t *Tree) {
	n.marginal(t)
	var wg sync.WaitGroup
	for _, c := range t.t.Children(n.id) {
		wg.Add(1)
		go func(c int) {
			nc := t.nodes[c]
			nc.fullUpPass(t)
			wg.Done()
		}(c)
	}
	wg.Wait()
}

func (n *node) marginal(t *Tree) {
	if t.t.IsRoot(n.id) {
		// At the root the marginal is the scaled likelihood
		// (we already have add the pixel priors in the down-pass)
		rs := n.stages[0]

		marg := make([][][]float64, len(t.landProb))
		for i := range marg {
			marg[i] = make([][]float64, len(t.landProb[i].traits))
			for j := range marg[i] {
				marg[i][j] = make([]float64, t.tp.Pixelation().Len())
			}
		}
		normalizeLogProb(marg, rs.logLike)
		rs.marginal = marg
	}

	// internodes
	tmpEnd := make([][][]float64, len(t.landProb))
	tmpStart := make([][][]float64, len(t.landProb))
	for i := range tmpEnd {
		tmpEnd[i] = make([][]float64, len(t.landProb[i].traits))
		tmpStart[i] = make([][]float64, len(t.landProb[i].traits))
		for j := range tmpEnd[i] {
			tmpEnd[i][j] = make([]float64, t.tp.Pixelation().Len())
			tmpStart[i][j] = make([]float64, t.tp.Pixelation().Len())
		}
	}

	// Get the weights of each category
	// using the already assigned weights
	// in the first stage
	catWeights := make([]float64, len(t.landProb))
	fs := n.stages[0]
	for i := range catWeights {
		for j := range fs.marginal[i] {
			for _, p := range fs.marginal[i][j] {
				catWeights[i] += p
			}
		}
	}

	// the first stage was already updated
	for i := 1; i < len(n.stages); i++ {
		ts := n.stages[i]
		age := t.rot.ClosestStageAge(ts.age)
		prev := n.stages[i-1]
		prevAge := t.rot.ClosestStageAge(prev.age)

		if prevAge != age {
			// rotate if there is an state change
			rot := t.rot.OldToYoung(prevAge)
			rotation(rot.Rot, tmpStart, prev.marginal)
		} else {
			for j := range tmpStart {
				for k := range tmpStart[j] {
					copy(tmpStart[j][k], prev.marginal[j][k])
				}
			}
		}
		ts.calcMarginal(t, tmpStart, tmpEnd, catWeights)
	}

	if !t.t.IsTerm(n.id) {
		// In a split node
		// copy the marginals in the descendants.

		// First,
		// we add the categories in the split.
		// As the marginal is already normalized
		// we now that the sum will add to one.
		// We add all the categories in the first category
		ts := n.stages[len(n.stages)-1]
		for j := range tmpEnd[0] {
			for px := range tmpEnd[0][j] {
				tmpEnd[0][j][px] = 0
			}
		}
		for i := range ts.marginal {
			for j := range ts.marginal[i] {
				for px, p := range ts.marginal[i][j] {
					tmpEnd[0][j][px] += p
				}
			}
		}

		// Now copy the marginals in each descendant.
		for _, d := range t.t.Children(n.id) {
			resMarg := make([][][]float64, len(t.landProb))
			for i := range resMarg {
				resMarg[i] = make([][]float64, len(t.landProb[i].traits))
				for j := range resMarg[i] {
					resMarg[i][j] = make([]float64, t.tp.Pixelation().Len())
				}
			}
			c := t.nodes[d]
			cs := c.stages[0]

			// To get the marginal of each pixel at each category
			// we weight the marginal of the pixel at the split
			// with the conditional for each category for that pixel.
			normalizeLogProb(tmpStart, cs.logLike)
			for j := range tmpEnd[0] {
				for px, p := range tmpEnd[0][j] {
					// local conditionals of the category
					var sum float64
					for i := range tmpStart {
						sum += tmpStart[i][j][px]
					}
					if sum == 0 {
						continue
					}
					for i := range resMarg {
						resMarg[i][j][px] = p * tmpStart[i][j][px] / sum
					}
				}
			}
			cs.marginal = resMarg
		}
	}
}

// Marginal calculates the marginal reconstruction
// of a time stage.
func (ts *timeStage) calcMarginal(t *Tree, start, end [][][]float64, weightCat []float64) {
	resMarg := make([][][]float64, len(t.landProb))
	for i := range resMarg {
		resMarg[i] = make([][]float64, len(t.landProb[i].traits))
		for j := range resMarg[i] {
			resMarg[i][j] = make([]float64, t.tp.Pixelation().Len())
		}
	}

	age := t.tp.ClosestStageAge(ts.age)
	for i := range t.landProb {
		t.landProb[i].prepareStage(age)
	}

	answer := make(chan likeChanAnswer)
	go func() {
		for i := range t.landProb {
			margChan <- likeChanType{
				like:   start[i],
				raw:    resMarg[i],
				w:      t.landProb[i],
				age:    age,
				cat:    i,
				steps:  ts.steps,
				answer: answer,
			}
		}
	}()

	normalizeLogProbByCat(end, ts.logLike)
	sum := make([]float64, len(t.landProb))
	for range t.landProb {
		a := <-answer
		resMarg[a.cat] = a.rawLike
		for tr := range resMarg[a.cat] {
			for px, p := range resMarg[a.cat][tr] {
				p *= end[a.cat][tr][px]
				sum[a.cat] += p
				resMarg[a.cat][tr][px] = p
			}
		}
	}
	close(answer)

	// normalize the values
	for i := range sum {
		for tr := range resMarg[i] {
			for px, p := range resMarg[i][tr] {
				resMarg[i][tr][px] = p * weightCat[i] / sum[i]
			}
		}
	}
	ts.marginal = resMarg
}

func normalizeLogProb(dst, src [][][]float64) {
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
	var sum float64
	for c := range src {
		for t := range src[c] {
			for px, l := range src[c][t] {
				p := math.Exp(l - max)
				dst[c][t][px] = p
				sum += p
			}
		}
	}

	// normalize the values
	for c := range dst {
		for t := range dst[c] {
			for px, p := range dst[c][t] {
				dst[c][t][px] = p / sum
			}
		}
	}
}

func normalizeLogProbByCat(dst, src [][][]float64) {
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
	sum := make([]float64, len(src))
	for c := range sum {
		for t := range src[c] {
			for px, l := range src[c][t] {
				p := math.Exp(l - max)
				dst[c][t][px] = p
				sum[c] += p
			}
		}
	}

	// normalize the values
	for c := range sum {
		for t := range dst[c] {
			for px, p := range dst[c][t] {
				dst[c][t][px] = p / sum[c]
			}
		}
	}
}
