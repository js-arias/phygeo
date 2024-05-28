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
	start, end int
}

type likeResult struct {
	px      int
	logLike float64
}

type likePixData struct {
	pix *earth.Pixelation
	dm  *earth.DistMat

	like []likePix
	max  float64
	pdf  dist.Normal
}

func pixLike(likeChan chan likeChanType, wg *sync.WaitGroup, data likePixData, r []likeResult) {
	for c := range likeChan {
		for i := c.start; i < c.end; i++ {
			px := r[i].px
			logLike := calcPixLike(data, px)
			r[i].logLike = logLike
		}
		wg.Done()
	}
}

func calcPixLike(c likePixData, pix int) float64 {
	var sum, scale float64
	if c.dm != nil {
		// use the distance matrix
		for _, cL := range c.like {
			dist := c.dm.At(pix, cL.px)
			p := c.pdf.ScaledProbRingDist(dist)
			scale += p * cL.prior
			sum += p * cL.like
		}
	} else {
		// use raw distance
		pt1 := c.pix.ID(pix).Point()
		for _, cL := range c.like {
			pt2 := c.pix.ID(cL.px).Point()
			dist := earth.Distance(pt1, pt2)
			p := c.pdf.ScaledProb(dist)
			scale += p * cL.prior
			sum += p * cL.like
		}
	}

	if sum > 0 {
		return math.Log(sum) + c.max - math.Log(scale)
	}

	// pixels are quite far away
	// use only the maximum likelihood point
	maxLike := -math.MaxFloat64
	logScale := math.Log(scale)
	if c.dm != nil {
		// use the distance matrix
		for _, cL := range c.like {
			dist := c.dm.At(pix, cL.px)
			lp := c.pdf.LogProbRingDist(dist) + cL.logLike - logScale
			if lp > maxLike {
				maxLike = lp
			}
		}
	} else {
		// use raw distance
		pt1 := c.pix.ID(pix).Point()
		for _, cL := range c.like {
			pt2 := c.pix.ID(cL.px).Point()
			dist := earth.Distance(pt1, pt2)
			lp := c.pdf.LogProb(dist) + cL.logLike - logScale
			if lp > maxLike {
				maxLike = lp
			}
		}
	}
	return maxLike
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
	resTmp := make([]likeResult, 0, t.landscape.Pixelation().Len())
	n.conditional(t, pixTmp, resTmp)
}

func (n *node) conditional(t *Tree, pixTmp []likePix, resTmp []likeResult) {
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
		logLike := next.conditional(t, age, pixTmp, resTmp)

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
	prior   float64 // pixel prior
}

// pixel blocks
var pixBlocks = 1000

// Conditional calculates the conditional likelihood
// at a time stage.
func (ts *timeStage) conditional(t *Tree, old int64, pixTmp []likePix, resTmp []likeResult) map[int]float64 {
	age := t.landscape.ClosestStageAge(ts.age)
	var rot *model.Rotation
	if age != old {
		rot = t.rot.YoungToOld(age)
	}
	stage := t.landscape.Stage(age)

	// update descendant log like
	// with the arrival priors
	endLike, max := prepareLogLikePix(ts.logLike, t.pp, stage, pixTmp)

	// reset result slice
	resTmp = resTmp[:0]
	for px := range stage {
		// skip pixels with 0 prior
		if t.pp.Prior(stage[px]) == 0 {
			continue
		}

		// the pixel must be valid at the oldest stage
		if rot != nil {
			if _, ok := rot.Rot[px]; !ok {
				continue
			}
		}

		resTmp = append(resTmp, likeResult{px: px})
	}

	data := likePixData{
		pix:  t.landscape.Pixelation(),
		dm:   t.dm,
		like: endLike,
		max:  max,
		pdf:  ts.pdf,
	}

	// parallel part
	likeChan := make(chan likeChanType, numCPU*2)
	var wg sync.WaitGroup
	for i := 0; i < numCPU; i++ {
		go pixLike(likeChan, &wg, data, resTmp)
	}
	for i := 0; i < len(resTmp); i += pixBlocks {
		wg.Add(1)
		end := i + pixBlocks
		if end > len(resTmp) {
			end = len(resTmp)
		}
		likeChan <- likeChanType{
			start: i,
			end:   end,
		}
	}
	wg.Wait()
	close(likeChan)

	logLike := make(map[int]float64, len(stage))
	for _, r := range resTmp {
		// skip invalid pixels
		if r.px < 0 {
			continue
		}
		logLike[r.px] = r.logLike
	}

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

	for px, v := range tp {
		pp := prior.Prior(v)
		if pp == 0 {
			continue
		}

		p, ok := logLike[px]
		if !ok {
			p = -math.MaxFloat64
		} else {
			p += prior.LogPrior(v)
		}
		lp = append(lp, likePix{
			px:      px,
			like:    p,
			logLike: p,
			prior:   pp,
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
