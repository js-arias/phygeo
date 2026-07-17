// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walker

import (
	"math"

	"github.com/js-arias/earth"
	"github.com/js-arias/earth/stat/dist"
)

// Lambda returns the lambda value
// that produce an equivalent spherical normal distribution
// for a wanderlust value,
// and the settlement and movement weights.
func Lambda(pix *earth.Pixelation, net earth.Network, wanderlust, sett, mov float64, steps int) float64 {
	dist := walkProb(pix, net, steps, wanderlust, sett, mov)
	var spread float64
	for _, p := range dist[1:] {
		spread += p
	}
	min := 1.0
	max := 10_000.0
	var best float64
	for st := 100.0; st >= 0.1; st /= 10 {
		best = bestLambda(1-spread, min, max, st, pix)
		min = best - st
		if min < 0 {
			min = 0
		}
		max = best + st
	}
	return best
}

// Settlement returns the settlement probability
// that produce an equivalent distribution
// of the spherical normal with a given lambda value
// discretized in a random walk
// with the given number of steps.
func Settlement(pix *earth.Pixelation, net earth.Network, lambda float64, steps int) float64 {
	sn := dist.NewNormal(lambda, pix)
	first := sn.Prob(0)
	var min float64
	max := 1.0
	var best float64
	for st := 0.1; st > 0.000001; st /= 10 {
		best = getBest(first, min, max, st, pix, net, steps)
		min = best - st
		if min < 0 {
			min = 0
		}
		max = best + st
		if max > 1 {
			max = 1
		}
	}
	return best
}

// Expected returns the expected value
// (in radians)
// and the variance
// (in radians^2)
// of a random walk,
// for a wanderlust value,
// and the homogeneous settlement
// and movement weights.
func Expected(pix *earth.Pixelation, net earth.Network, wanderlust, sett, mov float64, steps int) (exp, v float64) {
	dist := walkProb(pix, net, steps, wanderlust, sett, mov)
	var sumE, sumV float64
	for px, p := range dist {
		d := earth.ToRad(float64(pix.ID(px).Ring()) * pix.Step())
		sumE += d * p
		sumV += d * d * p
	}
	return sumE, sumV
}

func walkProb(pix *earth.Pixelation, net earth.Network, steps int, wanderlust, sett, mov float64) []float64 {
	curr := make([]float64, pix.Len())
	prev := make([]float64, pix.Len())
	curr[0] = 1
	for range steps {
		prev, curr = curr, prev
		for px := range curr {
			curr[px] = 0
		}
		for px := range prev {
			n := net[px]
			mv := wanderlust * mov
			st := 1 - mv
			mv /= float64(len(n) - 1)

			// move probability
			for _, x := range n {
				if x == px {
					continue
				}
				p := prev[px] * mv
				curr[x] += p
			}

			// add settlement
			curr[px] += prev[px] * st * sett
		}
	}
	return curr
}

func getBest(first, min, max, step float64, pix *earth.Pixelation, net earth.Network, numSteps int) float64 {
	best := min
	dist := 2.0
	for v := min + step; v < max; v += step {
		wp := walkProb(pix, net, numSteps, 1-v, 1, 1)[0]
		d := math.Abs(first - wp)
		if d < dist {
			dist = d
			best = v
		}
	}
	return best
}

func bestLambda(sett, min, max, step float64, pix *earth.Pixelation) float64 {
	best := min
	diff := 2.0
	for v := min; v <= max; v += step {
		sn := dist.NewNormal(v, pix)
		first := sn.Prob(0)
		d := math.Abs(first - sett)
		if d < diff {
			diff = d
			best = v
		}
	}
	return best
}
