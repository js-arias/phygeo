// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package model_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/js-arias/phygeo/infer/model"
)

func TestIO(t *testing.T) {
	mp := newMP()

	var buf bytes.Buffer
	if err := mp.TSV(&buf); err != nil {
		t.Fatalf("when writing TSV: %v", err)
	}
	rp, err := model.Read(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("when reading TSV: %v", err)
	}

	testModel(t, rp, mp)
}

func TestCopy(t *testing.T) {
	mp := newMP()
	cp := mp.Copy()
	testModel(t, cp, mp)
}

func newMP() *model.Model {
	mp := model.New()
	mp.Add("lambda", model.Walk, 1, 100)
	mp.Add("steps", model.Walk, 0, 120)
	mp.Add("lognormal", model.Rate, 2, 1)
	mp.Add("cats", model.Rate, 0, 9)
	mp.Add("land:ocean", model.Mov, 3, 1)
	mp.Add("land:oceanic plateaus", model.Mov, 3, 1)
	mp.Add("land:lands", model.Mov, 0, 1)
	mp.Add("land:ocean", model.Sett, 0, 0)
	mp.Add("land:oceanic plateaus", model.Sett, 0, 0.0001)
	mp.Add("land:lands", model.Sett, 0, 1)

	mp.SetMax("lognormal:sigma", model.Rate, 2)
	mp.SetMax("land:ocean", model.Mov, 1)
	mp.SetMax("land:oceanic plateaus", model.Mov, 1)

	return mp
}

func testModel(t testing.TB, mp, want *model.Model) {
	t.Helper()

	tg := mp.Types()
	tw := want.Types()
	if !reflect.DeepEqual(tg, tw) {
		t.Errorf("types: got %v, want %v", tg, tw)
	}

	for _, tp := range tw {
		ng := mp.Names(tp)
		nw := want.Names(tp)
		if !reflect.DeepEqual(ng, nw) {
			t.Errorf("names: type %q: got %v, want %v", tp, ng, nw)
		}
		for _, n := range nw {
			IDg := mp.ID(n, tp)
			IDw := want.ID(n, tp)
			if IDg != IDw {
				t.Errorf("ID: variable %q, type %q: got %d, want %d", n, tp, IDg, IDw)
			}

			vg := mp.Val(n, tp)
			vw := want.Val(n, tp)
			if vg != vw {
				t.Errorf("Value: variable %q, type %q: got %.6f, want %.6f", n, tp, vg, vw)
			}

			mg := mp.Max(n, tp)
			mw := want.Max(n, tp)
			if mg != mw {
				t.Errorf("Max: variable %q, type %q: got %.6f, want %.6f", n, tp, mg, mw)
			}
		}
	}
	IDsg := mp.IDs()
	IDsw := want.IDs()
	if !reflect.DeepEqual(IDsg, IDsw) {
		t.Errorf("IDs: got %v, want %v", IDsg, IDsw)
	}
}
