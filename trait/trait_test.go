// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package trait_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/js-arias/phygeo/trait"
)

func TestData(t *testing.T) {
	d := newData()

	testData(t, "data", d)
}

func TestTSV(t *testing.T) {
	d := newData()

	var w bytes.Buffer
	if err := d.TSV(&w); err != nil {
		t.Fatalf("unable to write TSV data: %v", err)
	}
	t.Logf("output:\n%s\n", w.String())

	r := strings.NewReader(w.String())
	nd, err := trait.ReadTSV(r)
	if err != nil {
		t.Fatalf("unable to read TSV data: %v", err)
	}

	testData(t, "tsv", nd)
}

func newData() *trait.Data {
	d := trait.New()

	d.Add("Acer platanoides", "temperate")
	d.Add("Acer saccharinum", "temperate")
	d.Add("Acer campbellii", "temperate")
	d.Add("Acer campbellii", "tropical")
	d.Add("Acer erythranthum", "tropical")
	return d
}

func testData(t testing.TB, name string, d *trait.Data) {
	t.Helper()

	taxa := []string{"Acer campbellii", "Acer erythranthum", "Acer platanoides", "Acer saccharinum"}
	if g := d.Taxa(); !reflect.DeepEqual(g, taxa) {
		t.Errorf("%s: taxa: got %v, want %v", name, g, taxa)
	}

	states := []string{"temperate", "tropical"}
	if g := d.States(); !reflect.DeepEqual(g, states) {
		t.Errorf("%s: states: got %v, want %v", name, g, states)
	}

	obs := map[string][]string{
		"Acer campbellii":   {"temperate", "tropical"},
		"Acer erythranthum": {"tropical"},
		"Acer platanoides":  {"temperate"},
		"Acer saccharinum":  {"temperate"},
	}
	for tx, w := range obs {
		if g := d.Obs(tx); !reflect.DeepEqual(g, w) {
			t.Errorf("%s: observations for %q: got %v, want %v", name, tx, g, w)
		}
	}
}
