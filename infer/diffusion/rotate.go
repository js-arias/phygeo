// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package diffusion

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
