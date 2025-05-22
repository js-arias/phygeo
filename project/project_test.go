// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package project_test

import (
	"os"
	"reflect"
	"slices"
	"testing"

	"github.com/js-arias/phygeo/project"
)

type setPath struct {
	set  project.Dataset
	path string
}

func TestProject(t *testing.T) {
	p := project.New()

	sets := []setPath{
		{project.GeoMotion, "geo-model.tab"},
		{project.PixWeight, "pix-weights.tab"},
		{project.Ranges, "ranges.tab"},
		{project.Landscape, "landscape.tab"},
		{project.Trees, "trees.tab"},
		{project.Stages, "stages.tab"},
	}

	for _, s := range sets {
		p.Add(s.set, s.path)
	}
	testProject(t, p, sets)

	name := "tmp-project-for-test.tab"
	defer os.Remove(name)

	p.SetName(name)
	if err := p.Write(); err != nil {
		t.Fatalf("error when writing data: %v", err)
	}

	np, err := project.Read(name)
	if err != nil {
		t.Fatalf("error when reading data: %v", err)
	}
	testProject(t, np, sets)
}

func testProject(t testing.TB, p *project.Project, sets []setPath) {
	t.Helper()

	for _, s := range sets {
		if path := p.Path(s.set); path != s.path {
			t.Errorf("set %s: got path %q, want %q", s.set, path, s.path)
		}
	}
	datasets := make([]project.Dataset, 0, len(sets))
	for _, v := range sets {
		datasets = append(datasets, v.set)
	}
	slices.Sort(datasets)

	if ls := p.Sets(); !reflect.DeepEqual(ls, datasets) {
		t.Errorf("sets: got %v, want %v", ls, datasets)
	}
}
