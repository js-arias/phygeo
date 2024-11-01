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
	"github.com/js-arias/earth/stat/pixweight"
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
	lnLike := make([]float64, data.pix.Len())
	for c := range likeChan {
		for i := c.start; i < c.end; i++ {
			px := r[i].px
			logLike := calcPixLike(data, px, lnLike)
			r[i].logLike = logLike
		}
		wg.Done()
	}
}

func calcPixLike(c likePixData, pix int, lnLike []float64) float64 {
	var sum, scale float64
	for _, cL := range c.like {
		dist := c.dm.At(pix, cL.px)
		p := c.pdf.ScaledProbRingDist(dist)
		scale += p * cL.weight
		sum += p * cL.like
	}

	if sum > 0 {
		return math.Log(sum) + c.max - math.Log(scale)
	}

	// pixels are quite far away
	scale = 0
	lnLike = lnLike[:0]
	maxLn := -math.MaxFloat64
	for _, cL := range c.like {
		dist := c.dm.At(pix, cL.px)
		p := c.pdf.LogProbRingDist(dist) + cL.logLike
		scale += c.pdf.ProbRingDist(dist) * cL.weight
		if p > maxLn {
			maxLn = p
		}
		lnLike = append(lnLike, p)
	}

	sum = 0
	for _, p := range lnLike {
		sum += math.Exp(p - maxLn)
	}
	return math.Log(sum) + maxLn - math.Log(scale)
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
		var logLike map[int]float64
		for i, d := range desc {
			c := t.nodes[d]
			if i == 0 {
				logLike = make(map[int]float64, len(c.stages[0].logLike))
			}
			for px, p := range c.stages[0].logLike {
				logLike[px] += p
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
		rs.logLike = addWeights(rs.logLike, t.pw, tp)
	}
}

// LikePix stores the conditional likelihood of a pixel.
type likePix struct {
	px      int     // Pixel ID
	like    float64 // conditional likelihood
	logLike float64
	weight  float64 // pixel weight
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
	endLike, max := prepareLogLikePix(ts.logLike, t.pw, stage, pixTmp)

	// reset result slice
	resTmp = resTmp[:0]
	for px := range stage {
		// skip pixels with 0 weight
		if t.pw.Weight(stage[px]) == 0 {
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

func addWeights(logLike map[int]float64, weight pixweight.Pixel, tp map[int]int) map[int]float64 {
	add := make(map[int]float64, len(logLike))
	for px, p := range logLike {
		v := tp[px]
		if pw := weight.Weight(v); pw == 0 {
			continue
		}
		add[px] = p + weight.LogWeight(v)
	}

	return add
}

// PrepareLogLikePix takes a map of pixels and conditional likelihoods,
// add the weight of each pixel
// and return an array with the pixels and its normalized (non-log) conditional likelihoods,
// and the normalization factor (in log form).
func prepareLogLikePix(logLike map[int]float64, weight pixweight.Pixel, tp map[int]int, lp []likePix) ([]likePix, float64) {
	max := -math.MaxFloat64
	lp = lp[:0]

	for px, v := range tp {
		pw := weight.Weight(v)
		if pw == 0 {
			continue
		}

		p, ok := logLike[px]
		if !ok {
			p = -math.MaxFloat64
		} else {
			p += weight.LogWeight(v)
		}
		lp = append(lp, likePix{
			px:      px,
			like:    p,
			logLike: p,
			weight:  pw,
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
