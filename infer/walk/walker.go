// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walk

import (
	"math"
	"math/rand/v2"

	"github.com/js-arias/earth/model"
)

func runPixLike() {
	for c := range likeChan {
		for t := range c.like {
			for px := c.start; px < c.end; px++ {
				logLike := simPixLike(c.w, c.scaleProb, c.steps, c.age, px, t, c.walkers)
				c.like[t][px] = logLike + c.maxLn
			}
		}
		c.wg.Done()
	}
}

func simPixLike(w *walkModel, scaledProb [][]float64, steps []int, age int64, px, t, walkers int) float64 {
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
	return math.Log(sum) - math.Log(float64(len(steps)*walkers))
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

type pathDensity struct {
	pix   int // end pixel
	trait int // end trait
	cat   int // category
	prob  float64
	path  []int
}

func runSimPath(pathChan chan pathChanType, walkers, numCat, maxStep int) {
	paths := make([]*pathDensity, walkers*numCat)
	tmpPaths := make([]*pathDensity, walkers*numCat)

	for i := range paths {
		p := &pathDensity{
			path: make([]int, 0, maxStep+1),
		}
		paths[i] = p
	}
	for c := range pathChan {
		for pt := c.start; pt < c.end; pt++ {
			px := c.src[pt]
			t := c.t[pt]
			dp, dt, cat, path := simPath(c.w, c.rot, c.density, paths, tmpPaths, c.steps, c.age, px, t, walkers)
			pp := make([]int, len(path))
			copy(pp, path)
			p := &Path{
				From:       px,
				To:         dp,
				TraitStart: c.w.traits[t],
				TraitEnd:   c.w.traits[dt],
				Cat:        cat,
				Path:       pp,
			}
			c.path[pt] = p
		}
		c.wg.Done()
	}
}

func simPath(w *walkModel, rot *model.Rotation, density [][]float64, paths, tmpPaths []*pathDensity, steps []int, age int64, px, t, walkers int) (int, int, int, []int) {
	var max float64
	for max == 0 {
		tmpPaths = tmpPaths[:0]
		for i, step := range steps {
			for j := range walkers {
				pd := paths[i*walkers+j]
				dp, dt, p, path := pathWalk(w, pd.path, age, px, t, step)
				if rot != nil && len(rot.Rot[dp]) == 0 {
					continue
				}
				pd.pix = dp
				pd.trait = dt
				pd.cat = i
				pd.prob = p * density[t][dp]
				pd.path = path
				if pd.prob > max {
					max = pd.prob
				}
				if pd.prob > 0 {
					tmpPaths = append(tmpPaths, pd)
				}
			}
		}
	}
	return pickPath(tmpPaths, max)
}

func pickPath(paths []*pathDensity, max float64) (int, int, int, []int) {
	// scale probabilities
	for _, p := range paths {
		p.prob = p.prob / max
	}

	for {
		i := rand.IntN(len(paths))
		p := paths[i]
		if rand.Float64() < p.prob {
			return p.pix, p.trait, p.cat, p.path
		}
	}
}

func pathWalk(w *walkModel, path []int, age int64, px, t, steps int) (int, int, float64, []int) {
	stage := w.stage(age, t)
	path = path[:0]
	path = append(path, px)
	for range steps {
		n := stage.move[px]
		for {
			nx := rand.IntN(len(n))
			if rand.Float64() < n[nx].prob {
				px = n[nx].id
				path = append(path, px)
				break
			}
		}
	}
	return px, t, stage.prior[px], path
}
