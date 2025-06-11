// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walk

import (
	"math/rand/v2"

	"github.com/js-arias/earth/model"
)

// Rotation rotates a log-prob map using a rotation.
func rotation(rot map[int][]int, rng, dest [][]float64) {
	for i, t := range rng {
		for px, p := range t {
			np := rot[px]

			for _, ip := range np {
				if p > dest[i][ip] {
					dest[i][ip] = p
				}
			}
		}
	}
}

// HasToRot returns true if the next node is in a different time stage.
func hasToRot(rot *model.StageRot, age int64) bool {
	if age == 0 {
		return false
	}
	return rot.ClosestStageAge(age) != rot.ClosestStageAge(age-1)
}

// RotPix rotates a set of pixels to the next time stage.
// If there are multiple destinations,
// it will pick a destination based on the weight of the destination pixel.
func rotPix(rot *model.StageRot, w *walkModel, age int64, pix, tr []int) []int {
	rm := rot.OldToYoung(age)
	if rm == nil {
		return pix
	}

	for i, px := range pix {
		pxs := rm.Rot[px]
		if len(pxs) == 1 {
			pix[i] = pxs[0]
			continue
		}

		stage := w.stage(age, tr[i])
		var max float64
		for _, dp := range pxs {
			if stage.prior[dp] > max {
				max = stage.prior[dp]
			}
		}

		if max == 0 {
			// If all pixels are invalid
			// pick one at random.
			pix[i] = pxs[rand.IntN(len(pxs))]
			continue
		}

		for {
			dp := pxs[rand.IntN(len(pxs))]
			accept := stage.prior[dp] / max
			if rand.Float64() < accept {
				pix[i] = dp
				break
			}
		}
	}
	return pix
}
