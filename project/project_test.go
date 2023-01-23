// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package project_test

import (
	"os"
	"reflect"
	"testing"

	"github.com/js-arias/phygeo/project"
)

func TestProject(t *testing.T) {
	p := project.New()

	sets := []struct {
		set  project.Dataset
		path string
	}{
		{project.Trees, "trees.tab"},
	}

	for _, s := range sets {
		p.Add(s.set, s.path)
	}
	testProject(t, p)

	name := "tmp-project-for-test.tab"
	defer os.Remove(name)

	if err := p.Write(name); err != nil {
		t.Fatalf("error when writing data: %v", err)
	}

	np, err := project.Read(name)
	if err != nil {
		t.Fatalf("error when reading data: %v", err)
	}
	testProject(t, np)
}

func testProject(t testing.TB, p *project.Project) {
	t.Helper()

	sets := []struct {
		set  project.Dataset
		path string
	}{
		{project.Trees, "trees.tab"},
	}

	for _, s := range sets {
		if path := p.Path(s.set); path != s.path {
			t.Errorf("set %s: got path %q, want %q", s.set, path, s.path)
		}
	}
	datasets := []project.Dataset{
		project.Trees,
	}
	if ls := p.Sets(); !reflect.DeepEqual(ls, datasets) {
		t.Errorf("sets: got %v, want %v", ls, datasets)
	}
}
