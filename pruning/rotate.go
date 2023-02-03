// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package pruning

import "math"

// Rotate rotates a log-map using a rotation map.
func rotate(rot map[int][]int, rng map[int]float64) map[int]float64 {
	nr := make(map[int]float64, len(rng))
	for px, p := range rng {
		np := rot[px]

		// divides the probability on all destination pixels
		p := p - math.Log(float64(len(np)))
		for _, ip := range np {
			op, ok := nr[ip]
			if !ok {
				nr[ip] = p
				continue
			}

			max := op
			if p > max {
				max = p
			}
			sum := math.Exp(p-max) + math.Exp(op-max)
			nr[ip] = math.Log(sum) + max
		}
	}
	return nr
}
