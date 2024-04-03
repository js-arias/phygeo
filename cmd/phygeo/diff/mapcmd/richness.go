// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package mapcmd

import "github.com/js-arias/earth/model"

func richnessOnTime(landscape *model.TimePix) (map[int64]*recStage, error) {
	rt, err := getRec(inputFile, landscape)
	if err != nil {
		return nil, err
	}

	stages := make(map[int64]*recStage)
	for _, t := range rt {
		for _, n := range t.nodes {
			for _, s := range n.stages {
				// only use exact time stages
				age := landscape.ClosestStageAge(s.age)
				if age != s.age {
					continue
				}

				st, ok := stages[age]
				if !ok {
					st = &recStage{
						age: age,
						rec: make(map[int]float64),
					}
					stages[age] = st
				}

				for px, p := range s.rec {
					st.rec[px] += p
				}
			}
		}
	}

	// scale values
	for _, st := range stages {
		var max float64
		for _, p := range st.rec {
			if p > max {
				max = p
			}
		}

		for px, p := range st.rec {
			st.rec[px] = p / max
		}
	}

	return stages, nil
}
