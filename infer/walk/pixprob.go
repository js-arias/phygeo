// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walk

import "slices"

type pixProbVal struct {
	trait int
	id    int
	p     float64
	sum   float64
}

func pixCDF(prob [][]float64, tr, sz int) map[int]float64 {
	pp := make([]pixProbVal, 0, sz*len(prob))
	for i := range prob {
		for px, p := range prob[i] {
			if p == 0 {
				continue
			}
			pp = append(pp, pixProbVal{
				trait: i,
				id:    px,
				p:     p,
			})
		}
	}
	slices.SortFunc(pp, func(a, b pixProbVal) int {
		if b.p < a.p {
			return -1
		}
		if b.p > a.p {
			return 1
		}
		return 0
	})

	var sum float64
	for i := range pp {
		pp[i].sum = sum
		sum += pp[i].p
	}

	cdf := make(map[int]float64, sz)
	for _, v := range pp {
		if v.trait != tr {
			continue
		}
		cdf[v.id] = 1 - v.sum/sum
	}
	return cdf
}

func pixRawMarginal(prob [][]float64, tr, sz int) map[int]float64 {
	var sum float64
	for i := range prob {
		for _, p := range prob[i] {
			sum += p
		}
	}

	raw := make(map[int]float64, sz)
	for px, p := range prob[tr] {
		if p == 0 {
			continue
		}
		raw[px] = p / sum
	}
	return raw
}
