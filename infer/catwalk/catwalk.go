// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package catwalk searches for the empirical values
// of the settlement probability that fulfills
// the expected lambda values
package catwalk

import (
	"fmt"
	"math"
	"slices"

	"github.com/js-arias/earth"
	"github.com/js-arias/earth/stat/dist"
	"github.com/js-arias/phygeo/cats"
)

// Cats search for the settlement values
// for each category.
func Cats(pix *earth.Pixelation, net earth.Network, lambda float64, steps int, dd cats.Discrete) []float64 {
	cats := dd.Cats()
	sett := make([]float64, len(cats))

	sv := make(chan float64)
	for _, c := range cats {
		go func(c float64) {
			l := lambda * c
			sv <- Settlement(pix, net, l, steps)
		}(c)
	}
	for i := range sett {
		sett[i] = <-sv
	}
	close(sv)
	slices.Sort(sett)
	return sett
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

func walkProb(pix *earth.Pixelation, net earth.Network, steps int, sett float64) float64 {
	move := 1 - sett
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
			mv := move / float64(len(n)-1)
			for _, x := range n {
				if x == px {
					curr[px] += prev[px] * sett
					continue
				}
				curr[x] += prev[px] * mv
			}
		}
	}
	return curr[0]
}

func getBest(first, min, max, step float64, pix *earth.Pixelation, net earth.Network, numSteps int) float64 {
	best := min
	dist := 2.0
	for v := min + step; v < max; v += step {
		wp := walkProb(pix, net, numSteps, v)
		d := math.Abs(first - wp)
		if d < dist {
			dist = d
			best = v
		}
		fmt.Printf("%.6f |%.6f - %.6f| =  %.12f\n", v, wp, first, d)
	}
	return best
}
