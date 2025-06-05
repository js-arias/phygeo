// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walk

// Rotate rotates a log-prob map using a rotation.
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
