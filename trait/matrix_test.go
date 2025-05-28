// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package trait_test

import (
	"bytes"
	"image/color"
	"reflect"
	"strings"
	"testing"

	"github.com/js-arias/earth/pixkey"
	"github.com/js-arias/phygeo/trait"
)

func TestMatrix(t *testing.T) {
	key := newKey()
	d := newData()
	m := newMove(d, key)

	testMatrix(t, "new matrix", m)
}

func TestMatrixTSV(t *testing.T) {
	key := newKey()
	d := newData()
	m := newMove(d, key)

	var w bytes.Buffer
	if err := m.TSV(&w); err != nil {
		t.Fatalf("unable to write TSV data: %v", err)
	}
	t.Logf("output:\n%s\n", w.String())

	r := strings.NewReader(w.String())
	nm := trait.NewMatrix(d, key)
	if err := nm.ReadTSV(r); err != nil {
		t.Fatalf("unable to read TSV data: %v", err)
	}

	testMatrix(t, "matrix tsv", nm)
}

func newMove(d *trait.Data, key *pixkey.PixKey) *trait.Matrix {
	m := trait.NewMatrix(d, key)

	m.Add("temperate", "ocean", 0.01)
	m.Add("temperate", "tundra", 0.1)
	m.Add("temperate", "temperate", 1)
	m.Add("temperate", "tropical", 0.1)
	m.Add("temperate", "glacial", 0.01)

	m.Add("tropical", "ocean", 0.01)
	m.Add("tropical", "tundra", 0.01)
	m.Add("tropical", "temperate", 0.1)
	m.Add("tropical", "tropical", 1)
	m.Add("tropical", "glacial", 0.01)
	return m
}

func newKey() *pixkey.PixKey {
	pk := pixkey.New()

	pk.SetColor(color.RGBA{54, 75, 154, 255}, 0)
	pk.SetColor(color.RGBA{74, 123, 154, 255}, 1)
	pk.SetColor(color.RGBA{254, 218, 139, 255}, 3)
	pk.SetColor(color.RGBA{25, 21, 139, 255}, 4)
	pk.SetColor(color.RGBA{254, 254, 254, 255}, 6)

	pk.SetLabel(0, "ocean")
	pk.SetLabel(1, "tundra")
	pk.SetLabel(3, "temperate")
	pk.SetLabel(4, "tropical")
	pk.SetLabel(6, "glacial")

	return pk
}

func testMatrix(t testing.TB, name string, m *trait.Matrix) {
	t.Helper()

	ws := map[string]map[string]float64{
		"temperate": {
			"ocean":     0.01,
			"tundra":    0.1,
			"temperate": 1,
			"tropical":  0.1,
			"glacial":   0.01,
		},
		"tropical": {
			"ocean":     0.01,
			"tundra":    0.01,
			"temperate": 0.1,
			"tropical":  1,
			"glacial":   0.01,
		},
	}

	for tr, ks := range ws {
		for k, w := range ks {
			if g := m.Weight(tr, k); g != w {
				t.Errorf("%s: weight of %q-%q: got %.6f, want %.6f", name, tr, k, g, w)
			}
		}
	}

	traits := []string{"temperate", "tropical"}
	if g := m.Traits(); !reflect.DeepEqual(g, traits) {
		t.Errorf("%s: states: got %v, want %v", name, g, traits)
	}

	landscape := []string{"glacial", "ocean", "temperate", "tropical", "tundra"}
	if g := m.Landscape(); !reflect.DeepEqual(g, landscape) {
		t.Errorf("%s: landscape: got %v, want %v", name, g, landscape)
	}
}
