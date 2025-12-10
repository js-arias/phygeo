// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package walkparam_test

import (
	"os"
	"reflect"
	"testing"

	"github.com/js-arias/earth"
	"github.com/js-arias/phygeo/walkparam"
)

func TestWalkParam(t *testing.T) {
	pix := earth.NewPixelation(120)

	name := "tmp-walk-parameters-for-test.tab"
	wp := walkparam.New(name, pix)
	testWP(t, wp, nil, name, pix)

	wp.SetCats(11)
	wp.SetSteps(pix.Equator() * 2)
	wp.SetMinSteps(pix.Equator() / 4)
	wp.SetRelaxed("gamma")

	defer os.Remove(name)
	if err := wp.Write(); err != nil {
		t.Fatalf("error when writing data: %v", err)
	}

	np, err := walkparam.Read(name, pix)
	if err != nil {
		t.Fatalf("error when reading data: %v", err)
	}
	testWP(t, np, wp, name, pix)
}

func testWP(t testing.TB, wp, want *walkparam.WP, name string, pix *earth.Pixelation) {
	t.Helper()

	if want == nil {
		want = walkparam.New(name, pix)
	}

	if wp.Name() != want.Name() {
		t.Errorf("name: got %q, want %q", wp.Name(), want.Name())
	}

	if !reflect.DeepEqual(wp.Relaxed([]float64{1}), want.Relaxed([]float64{1})) {
		w := wp.Relaxed([]float64{1})
		g := want.Relaxed([]float64{1})
		t.Errorf("relaxed: got %v, want %v", w, g)
	}
	if wp.Function() != want.Function() {
		t.Errorf("function: got %q, want %q", wp.Function(), want.Function())
	}
	if wp.Cats() != want.Cats() {
		t.Errorf("cats: got %d, want %d", wp.Cats(), want.Cats())
	}

	if wp.Steps() != want.Steps() {
		t.Errorf("steps: got %d, want %d", wp.Steps(), want.Steps())
	}
	if wp.MinSteps() != want.MinSteps() {
		t.Errorf("min-steps: got %d, want %d", wp.MinSteps(), want.MinSteps())
	}
}
