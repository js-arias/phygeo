// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walk

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
