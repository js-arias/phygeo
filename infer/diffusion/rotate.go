// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package diffusion

import (
	"math/rand/v2"

	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/stat/pixweight"
)

// Rotate rotates a log-map using a rotation map.
func rotate(rot map[int][]int, rng map[int]float64) map[int]float64 {
	nr := make(map[int]float64, len(rng))
	for px, p := range rng {
		np := rot[px]

		for _, ip := range np {
			op, ok := nr[ip]
			if !ok {
				nr[ip] = p
				continue
			}

			// if pixel is already assigned kept the best value
			if p > op {
				nr[ip] = p
			}
		}
	}
	return nr
}

// RotPix rotates a pixel at a given age to the next age stage.
// If there are multiple destinations,
// it will pick a destination based on the weight of the destination pixels.
func rotPix(rot *model.StageRot, ts *model.TimePix, pix int, age int64, pw pixweight.Pixel) int {
	rm := rot.OldToYoung(age)
	if rm == nil {
		return pix
	}

	pxs := rm.Rot[pix]
	pix = pxs[0]
	if len(pxs) == 1 {
		return pix
	}

	tp := ts.Stage(ts.ClosestStageAge(age - 1))
	var max float64
	for _, px := range pxs {
		weight := pw.Weight(tp[px])
		if weight > max {
			max = weight
		}
	}

	for {
		px := pxs[rand.IntN(len(pxs))]
		accept := pw.Weight(tp[px]) / max
		if rand.Float64() < accept {
			return px
		}
	}
}
