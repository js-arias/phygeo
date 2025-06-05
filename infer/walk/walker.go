// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walk

import (
	"math"
	"math/rand/v2"
)

func runPixLike() {
	for c := range likeChan {
		for t := range c.like {
			for px := c.start; px < c.end; px++ {
				logLike := simPixLike(c.w, c.scaleProb, c.steps, c.age, px, t, c.walkers, c.times)
				c.like[t][px] = logLike + c.maxLn
			}
		}
		c.wg.Done()
	}
}

func simPixLike(w *walkModel, scaledProb [][]float64, steps []int, age int64, px, t, walkers, times int) float64 {
	stage := w.stage(age, t)
	if stage.prior[px] == 0 {
		return math.Inf(-1)
	}

	var sum float64
	for _, step := range steps {
		for range walkers {
			d, p := walk(w, age, px, t, step)
			sum += p * scaledProb[t][d]
		}
	}
	return math.Log(sum) - math.Log(float64(len(steps)*walkers*times))
}

func walk(w *walkModel, age int64, px, t, steps int) (int, float64) {
	stage := w.stage(age, t)
	for range steps {
		n := stage.move[px]
		for {
			nx := rand.IntN(len(n))
			if rand.Float64() < n[nx].prob {
				px = n[nx].id
				break
			}
		}
	}
	return px, stage.prior[px]
}
