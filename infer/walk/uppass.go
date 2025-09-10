// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walk

import "sync"

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
		// at the root the marginal is the scaled
		// At the root add the pixel priors
		rs := n.stages[0]

		marg := make([][]float64, len(t.landProb.traits))
		for i := range marg {
			marg[i] = make([]float64, t.landProb.tp.Pixelation().Len())
		}
		scaleLogProb(rs.logLike, marg)
		rs.marginal = marg
	}

	// internodes
	tmpMarg := make([][]float64, len(t.landProb.traits))
	tmpStart := make([][]float64, len(t.landProb.traits))
	for i := range tmpMarg {
		tmpMarg[i] = make([]float64, t.landProb.tp.Pixelation().Len())
		tmpStart[i] = make([]float64, t.landProb.tp.Pixelation().Len())
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
			rotation(rot.Rot, prev.marginal, tmpStart)
		} else {
			for i := range tmpStart {
				copy(tmpStart[i], prev.marginal[i])
			}
		}
		ts.calcMarginal(t, tmpStart, tmpMarg)
	}

	if !t.t.IsTerm(n.id) {
		// In a split node
		// copy the marginals in the descendants
		ts := n.stages[len(n.stages)-1]
		for _, d := range t.t.Children(n.id) {
			c := t.nodes[d]
			cs := c.stages[0]
			marg := make([][]float64, len(t.landProb.traits))
			for i := range marg {
				marg[i] = make([]float64, t.landProb.tp.Pixelation().Len())
				copy(marg[i], ts.marginal[i])
			}
			cs.marginal = marg
		}
	}
}

// Marginal calculates the marginal reconstruction
// of a time stage.
func (ts *timeStage) calcMarginal(t *Tree, start, tmpMarg [][]float64) {
	resMarg := make([][]float64, len(t.landProb.traits))
	for i := range resMarg {
		resMarg[i] = make([]float64, t.landProb.tp.Pixelation().Len())
	}

	age := t.landProb.tp.ClosestStageAge(ts.age)
	t.landProb.prepareStage(age)
	scaleLogProb(ts.logLike, tmpMarg)

	answer := make(chan likeChanAnswer)
	go func() {
		for i := range t.landProb.traits {
			margChan <- margChanType{
				start:  start[i],
				end:    tmpMarg[i],
				raw:    resMarg[i],
				w:      t.landProb,
				age:    age,
				tr:     i,
				steps:  ts.steps,
				answer: answer,
			}
		}
	}()

	var max float64
	for range t.landProb.traits {
		a := <-answer
		resMarg[a.tr] = a.rawLike
		for _, p := range resMarg[a.tr] {
			if p > max {
				max = p
			}
		}
	}
	close(answer)

	// scale the values
	for i := range resMarg {
		for px, p := range resMarg[i] {
			resMarg[i][px] = p / max
		}
	}
	ts.marginal = resMarg
}
