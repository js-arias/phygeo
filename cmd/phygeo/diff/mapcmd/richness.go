// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package mapcmd

import (
	"fmt"
	"image"
	"sync"

	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/stat"
	"github.com/js-arias/earth/stat/dist"
	"github.com/js-arias/earth/stat/pixprob"
)

type stageRng struct {
	rs *recStage
	wg *sync.WaitGroup

	norm      dist.Normal
	pp        pixprob.Pixel
	landscape *model.TimePix
}

type kdeAnswer struct {
	age int64
	rng map[int]float64
}

func procStageRichness(sc chan stageRng, ans chan kdeAnswer) {
	for sr := range sc {
		rng := stat.KDE(sr.norm, sr.rs.rec, sr.landscape, sr.rs.cAge, sr.pp)
		ans <- kdeAnswer{
			age: sr.rs.cAge,
			rng: rng,
		}
		sr.wg.Done()
	}
}

func richnessOnTime(name string, tot *model.Total, landscape *model.TimePix, keys *pixKey, norm dist.Normal, pp pixprob.Pixel, contour image.Image) error {
	rec, err := getRec(name, landscape)
	if err != nil {
		return err
	}

	sc := make(chan stageRng, numCPU*2)
	ac := make(chan kdeAnswer, numCPU*2)
	for i := 0; i < numCPU; i++ {
		go procStageRichness(sc, ac)
	}

	var wg sync.WaitGroup
	for _, t := range rec {
		for _, n := range t.nodes {
			for _, s := range n.stages {
				if s.age != s.cAge {
					continue
				}
				wg.Add(1)
				go func(s *recStage) {
					sc <- stageRng{
						rs:        s,
						wg:        &wg,
						norm:      norm,
						pp:        pp,
						landscape: landscape,
					}
				}(s)
			}
		}
	}
	go func() {
		wg.Wait()
		close(sc)
		close(ac)
	}()

	rt := make(map[int64]*recStage)
	for a := range ac {
		st, ok := rt[a.age]
		if !ok {
			st = &recStage{
				age:       a.age,
				cAge:      a.age,
				rec:       make(map[int]float64),
				landscape: landscape,
			}
			rt[a.age] = st
		}

		for pix, prob := range a.rng {
			if prob < 1-bound {
				continue
			}

			st.rec[pix] += prob
		}
	}

	// We only use the KDE lambda when estimating
	// the node-stage ranges,
	// for the drawing,
	// we use the raw value.
	kdeLambda = 0

	for _, st := range rt {
		// the age is in million years
		age := float64(st.age) / 1_000_000
		suf := fmt.Sprintf("-richness-%.3f", age)
		out := outputPre + suf + ".png"

		st.step = 360 / float64(colsFlag)
		st.keys = keys
		st.contour = contour
		if tot != nil {
			st.tot = tot.Rotation(st.age)
		}

		var max float64
		for _, p := range st.rec {
			if p > max {
				max = p
			}
		}
		st.max = max

		if err := writeImage(out, st); err != nil {
			return err
		}
	}
	return nil
}
