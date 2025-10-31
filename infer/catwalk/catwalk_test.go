// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package catwalk_test

import (
	"math"
	"testing"

	"github.com/js-arias/earth"
	"github.com/js-arias/phygeo/cats"
	"github.com/js-arias/phygeo/infer/catwalk"
	"gonum.org/v1/gonum/stat/distuv"
)

func TestSettlement(t *testing.T) {
	pix := earth.NewPixelation(120)
	net := earth.NewNetwork(pix)

	got := catwalk.Settlement(pix, net, 100.0, pix.Equator())
	want := 0.943266
	if math.Abs(got-want) > 0.0001 {
		t.Errorf("lambda %.6f: got %.6f, want %.6f [diff = %.6f]", 100.0, got, want, math.Abs(got-want))
	}
}

func TestCats(t *testing.T) {
	pix := earth.NewPixelation(120)
	net := earth.NewNetwork(pix)

	dd := cats.LogNormal{
		Param: distuv.LogNormal{
			Mu:    0,
			Sigma: 1,
		},
		NumCat: 9,
	}
	want := []float64{
		0.753190,
		0.864349,
		0.904382,
		0.927440,
		0.943266,
		0.955249,
		0.965016,
		0.973755,
		0.983596,
	}
	got := catwalk.Cats(pix, net, 100, pix.Equator(), dd)
	cats := dd.Cats()
	for i, g := range got {
		if math.Abs(g-want[i]) > 0.0001 {
			t.Errorf("cat %d: lambda %.6f: got %.6f, want %.6f [diff = %.6f]", i, cats[i]*100.0, g, want[i], math.Abs(g-want[i]))
		}
	}

}
