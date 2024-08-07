// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package timestage_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/js-arias/phygeo/timestage"
)

type geoModel struct {
	stages []int64
}

func (g geoModel) Stages() []int64 {
	return g.stages
}

func TestStages(t *testing.T) {
	s := timestage.New()

	want := geoModel{
		stages: []int64{
			0,
			5_000_000,
			10_000_000,
			100_000_000,
			200_000_000,
			300_000_000,
			400_000_000,
			500_000_000,
			550_000_000,
		},
	}

	s.Add(want)
	testStages(t, "add", s, want.Stages())

	var buf bytes.Buffer
	if err := s.Write(&buf); err != nil {
		t.Fatalf("unable to write data: %v", err)
	}

	r, err := timestage.Read(&buf)
	if err != nil {
		t.Logf("input data:\n%s\n", buf.String())
		t.Fatalf("unable to read data: %v", err)
	}

	testStages(t, "read", r, want.Stages())
}

func testStages(t testing.TB, name string, s timestage.Stages, want []int64) {
	t.Helper()

	got := s.Stages()
	if len(got) != len(want) {
		t.Errorf("%s length: got %d stages, want %d", name, len(got), len(want))
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: got %v stages, want %v stages", name, got, want)
	}

}

func TestClosestStageAge(t *testing.T) {
	s := timestage.New()

	m := geoModel{
		stages: []int64{
			0,
			5_000_000,
			10_000_000,
			100_000_000,
			200_000_000,
			300_000_000,
			400_000_000,
			500_000_000,
			550_000_000,
		},
	}

	s.Add(m)

	tests := map[string]struct {
		age  int64
		want int64
	}{
		"exact": {
			age:  5_000_000,
			want: 5_000_000,
		},
		"younger": {
			age:  3_000_000,
			want: 0,
		},
		"middle": {
			age:  348_000_000,
			want: 300_000_000,
		},
		"older": {
			age:  555_000_000,
			want: 550_000_000,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := s.ClosestStageAge(test.age)
			if got != test.want {
				t.Errorf("%s: age %d: got %d, want %d", name, test.age, got, test.want)
			}
		})
	}
}
