// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walk

import "math/rand/v2"

// Rotation rotates a logLike field using a rotation.
func rotation(rot map[int][]int, dst, rng [][][]float64) {
	for c := range rng {
		for t := range rng[c] {
			for px, p := range rng[c][t] {
				np := rot[px]

				for _, ip := range np {
					if p > dst[c][t][ip] {
						dst[c][t][ip] = p
					}
				}
			}
		}
	}
}

// RotPixel returns the pixel from a rotation
func rotPixel(rot map[int][]int, px int, prior []float64) int {
	np := rot[px]
	if len(np) == 1 {
		return np[0]
	}

	// If there are multiple destination pixels,
	// pick one at random
	var sum float64
	for _, ip := range np {
		sum += prior[ip]
	}
	if sum == 0 {
		pos := rand.IntN(len(np))
		return (np[pos])
	}

	for {
		pos := rand.IntN(len(np))
		p := prior[np[pos]] / sum
		if rand.Float64() < p {
			return np[pos]
		}
	}
}
