// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package diffusion

import (
	"math"
	"sync"

	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/stat/dist"
	"github.com/js-arias/earth/stat/pixprob"
)

type likeChanType struct {
	pixel   int
	pix     *earth.Pixelation
	dm      *earth.DistMat
	logLike []logLikePix
	pdf     dist.Normal
	wg      *sync.WaitGroup
}

type answerChan struct {
	pixel   int
	logLike float64
}

func pixLike(likeChan chan likeChanType, answer chan answerChan, size int) {
	prob := make([]float64, 0, size)
	for c := range likeChan {
		prob = prob[:0]
		max := -math.MaxFloat64

		if c.dm != nil {
			// use the distance matrix
			for _, pL := range c.logLike {
				p := c.pdf.LogProbRingDist(c.dm.At(c.pixel, pL.px)) + pL.logLike
				prob = append(prob, p)
				if p > max {
					max = p
				}
			}
		} else {
			// use raw distance
			pt1 := c.pix.ID(c.pixel).Point()
			for _, pL := range c.logLike {
				pt2 := c.pix.ID(pL.px).Point()
				dist := earth.Distance(pt1, pt2)
				p := c.pdf.LogProb(dist) + pL.logLike
				prob = append(prob, p)
				if p > max {
					max = p
				}
			}
		}

		var sum float64
		for _, p := range prob {
			sum += math.Exp(p - max)
		}
		pixLike := -math.MaxFloat64
		if sum > 0 {
			pixLike = math.Log(sum) + max
		}
		answer <- answerChan{
			pixel:   c.pixel,
			logLike: pixLike,
		}
		c.wg.Done()
	}
}

var numCPU = 1

// SetCPU sets the number of process
// used for the reconstruction.
func SetCPU(cpu int) {
	numCPU = cpu
}

func (n *node) fullDownPass(t *Tree) {
	for _, c := range t.t.Children(n.id) {
		nc := t.nodes[c]
		nc.fullDownPass(t)
	}

	n.conditional(t)
}

func (n *node) conditional(t *Tree) {
	if !t.t.IsTerm(n.id) {
		// In an split node
		// the conditional likelihood is the product of the
		// conditional likelihoods of each descendant

		desc := t.t.Children(n.id)
		left := t.nodes[desc[0]]
		right := t.nodes[desc[1]]
		logLike := make(map[int]float64, len(left.stages[0].logLike))
		for px, p := range left.stages[0].logLike {
			op, ok := right.stages[0].logLike[px]
			if !ok {
				continue
			}
			logLike[px] = p + op
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
		logLike := next.conditional(t, age)

		// Rotate if there is an stage change
		if nextAge != age {
			rot := t.rot.YoungToOld(nextAge)
			logLike = rotate(rot.Rot, logLike)
		}

		ts.logLike = logLike
	}

	if t.t.IsRoot(n.id) {
		// set the pixels priors at the root
		rs := n.stages[0]
		tp := t.landscape.Stage(t.landscape.ClosestStageAge(rs.age))
		rs.logLike = addPrior(rs.logLike, tp, t.pp)
	}
}

// LogLikePix stores the conditional likelihood of a pixel.
type logLikePix struct {
	px      int     // Pixel ID
	logLike float64 // conditional likelihood
}

// Conditional calculates the conditional likelihood
// at a time stage.
func (ts *timeStage) conditional(t *Tree, old int64) map[int]float64 {
	age := t.landscape.ClosestStageAge(ts.age)
	var rot *model.Rotation
	if age != old {
		rot = t.rot.YoungToOld(age)
	}
	stage := t.landscape.Stage(age)

	likeChan := make(chan likeChanType, numCPU*2)
	answer := make(chan answerChan, numCPU*2)
	for i := 0; i < numCPU; i++ {
		go pixLike(likeChan, answer, t.landscape.Pixelation().Len())
	}

	// update descendant log like
	// with the arrival priors
	endLike := prepareLogLikePix(ts.logLike, stage, t.pp)

	go func() {
		// send the pixels

		var wg sync.WaitGroup
		for px := range stage {
			// skip pixels with 0 prior
			if pp := t.pp.Prior(stage[px]); pp == 0 {
				continue
			}

			// the pixel must be valid at oldest time stage
			if rot != nil {
				if _, ok := rot.Rot[px]; !ok {
					continue
				}
			}

			wg.Add(1)
			likeChan <- likeChanType{
				pixel:   px,
				pix:     t.landscape.Pixelation(),
				dm:      t.dm,
				logLike: endLike,
				pdf:     ts.pdf,
				wg:      &wg,
			}
		}
		wg.Wait()
		close(answer)
	}()

	logLike := make(map[int]float64, len(stage))
	for a := range answer {
		logLike[a.pixel] = a.logLike
	}
	close(likeChan)

	return logLike
}

func addPrior(logLike map[int]float64, tp map[int]int, pp pixprob.Pixel) map[int]float64 {
	logPrior := make(map[int]float64, len(pp.Values()))
	for _, v := range pp.Values() {
		p := pp.Prior(v)
		if p == 0 {
			continue
		}
		logPrior[v] = math.Log(p)
	}

	add := make(map[int]float64, len(logLike))
	for px, p := range logLike {
		prior, ok := logPrior[tp[px]]
		if !ok {
			continue
		}
		add[px] = p + prior
	}

	return add
}

// PrepareLogLikePix takes a map of pixels and conditional likelihoods,
// add the prior of each pixel
// and return an array with the pixels and its updated conditional likelihoods.
func prepareLogLikePix(logLike map[int]float64, tp map[int]int, pp pixprob.Pixel) []logLikePix {
	logPrior := make(map[int]float64, len(pp.Values()))
	for _, v := range pp.Values() {
		p := pp.Prior(v)
		if p == 0 {
			continue
		}
		logPrior[v] = math.Log(p)
	}

	lp := make([]logLikePix, 0, len(logLike))
	for px, p := range logLike {
		prior, ok := logPrior[tp[px]]
		if !ok {
			continue
		}
		lp = append(lp, logLikePix{
			px:      px,
			logLike: p + prior,
		})
	}

	return lp
}
