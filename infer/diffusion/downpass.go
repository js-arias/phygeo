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
	pixel int
	pix   *earth.Pixelation
	dm    *earth.DistMat
	like  []likePix
	max   float64
	pdf   dist.Normal
	wg    *sync.WaitGroup
}

type answerChan struct {
	pixel   int
	logLike float64
}

func pixLike(likeChan chan likeChanType, answer chan answerChan) {
	for c := range likeChan {
		var sum float64
		nMax := c.pdf.LogProbRingDist(0)
		max := -math.MaxFloat64
		if c.dm != nil {
			// use the distance matrix
			for _, cL := range c.like {
				dist := c.dm.At(c.pixel, cL.px)
				sum += c.pdf.ScaledProbRingDist(dist) * cL.like
				if sum > 0 {
					continue
				}
				if lp := c.pdf.LogProbRingDist(dist) + cL.logLike; lp > max {
					max = lp
				}
			}
		} else {
			// use raw distance
			pt1 := c.pix.ID(c.pixel).Point()
			for _, cL := range c.like {
				pt2 := c.pix.ID(cL.px).Point()
				dist := earth.Distance(pt1, pt2)
				sum += c.pdf.ScaledProb(dist) * cL.like
				if sum > 0 {
					continue
				}
				if lp := c.pdf.LogProb(dist) + cL.logLike; lp > max {
					max = lp
				}
			}
		}

		if sum > 0 {
			answer <- answerChan{
				pixel:   c.pixel,
				logLike: math.Log(sum) + c.max + nMax,
			}
			c.wg.Done()
			continue
		}

		answer <- answerChan{
			pixel:   c.pixel,
			logLike: max,
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

	pixTmp := make([]likePix, 0, t.landscape.Pixelation().Len())
	n.conditional(t, pixTmp)
}

func (n *node) conditional(t *Tree, pixTmp []likePix) {
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
		logLike := next.conditional(t, age, pixTmp)

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
		rs.logLike = addPrior(rs.logLike, t.pp, tp)
	}
}

// LikePix stores the conditional likelihood of a pixel.
type likePix struct {
	px      int     // Pixel ID
	like    float64 // conditional likelihood
	logLike float64
}

// Conditional calculates the conditional likelihood
// at a time stage.
func (ts *timeStage) conditional(t *Tree, old int64, pixTmp []likePix) map[int]float64 {
	age := t.landscape.ClosestStageAge(ts.age)
	var rot *model.Rotation
	if age != old {
		rot = t.rot.YoungToOld(age)
	}
	stage := t.landscape.Stage(age)

	likeChan := make(chan likeChanType, numCPU*2)
	answer := make(chan answerChan, numCPU*2)
	for i := 0; i < numCPU; i++ {
		go pixLike(likeChan, answer)
	}

	// update descendant log like
	// with the arrival priors
	endLike, max := prepareLogLikePix(ts.logLike, t.pp, stage, pixTmp)

	go func() {
		// send the pixels
		var wg sync.WaitGroup
		for px := range stage {
			// skip pixels with 0 prior
			if t.pp.Prior(stage[px]) == 0 {
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
				pixel: px,
				pix:   t.landscape.Pixelation(),
				dm:    t.dm,
				like:  endLike,
				max:   max,
				pdf:   ts.pdf,
				wg:    &wg,
			}
		}
		wg.Wait()
		close(answer)
	}()

	logLike := make(map[int]float64, len(stage))
	for a := range answer {
		// skip invalid pixels
		if a.pixel < 0 {
			continue
		}
		logLike[a.pixel] = a.logLike
	}
	close(likeChan)

	return logLike
}

func addPrior(logLike map[int]float64, prior pixprob.Pixel, tp map[int]int) map[int]float64 {
	add := make(map[int]float64, len(logLike))
	for px, p := range logLike {
		v := tp[px]
		if pp := prior.Prior(v); pp == 0 {
			continue
		}
		add[px] = p + prior.LogPrior(v)
	}

	return add
}

// PrepareLogLikePix takes a map of pixels and conditional likelihoods,
// add the prior of each pixel
// and return an array with the pixels and its normalized (non-log) conditional likelihoods,
// and the normalization factor (in log form).
func prepareLogLikePix(logLike map[int]float64, prior pixprob.Pixel, tp map[int]int, lp []likePix) ([]likePix, float64) {
	max := -math.MaxFloat64
	lp = lp[:0]
	for px, p := range logLike {
		v := tp[px]
		if pp := prior.Prior(v); pp == 0 {
			continue
		}
		p += prior.LogPrior(v)
		lp = append(lp, likePix{
			px:      px,
			like:    p,
			logLike: p,
		})
		if p > max {
			max = p
		}
	}

	// likelihood standardization
	for i, pv := range lp {
		lp[i].like = math.Exp(pv.like - max)
	}

	return lp, max
}
