// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walker_test

import (
	"math"
	"testing"

	"github.com/js-arias/earth"
	"github.com/js-arias/phygeo/infer/walker"
)

func TestSettlement(t *testing.T) {
	pix := earth.NewPixelation(120)
	net := earth.NewNetwork(pix)

	got := walker.Settlement(pix, net, 100.0, pix.Equator())
	want := 0.943266
	if math.Abs(got-want) > 0.0001 {
		t.Errorf("lambda %.6f: got %.6f, want %.6f [diff = %.6f]", 100.0, got, want, math.Abs(got-want))
	}
}
