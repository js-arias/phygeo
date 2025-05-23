// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package project

import (
	"fmt"
	"os"

	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/stat/pixweight"
	"github.com/js-arias/phygeo/timestage"
	"github.com/js-arias/ranges"
	"github.com/js-arias/timetree"
)

// GeoMotion reads a plate motion model file
// as defined in a project.
func (p *Project) GeoMotion(pix *earth.Pixelation) (*model.Recons, error) {
	name := p.Path(GeoMotion)
	if name == "" {
		return nil, fmt.Errorf("plate motion model not defined in project %q", p.name)
	}

	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}

	rec, err := model.ReadReconsTSV(f, pix)
	if err != nil {
		return nil, fmt.Errorf("when reading file %q: %v", name, err)
	}
	return rec, nil
}

// Landscape reads a time pixelation file
// as defined in a project.
func (p *Project) Landscape(pix *earth.Pixelation) (*model.TimePix, error) {
	name := p.Path(Landscape)
	if name == "" {
		return nil, fmt.Errorf("landscape not defined in project %q", p.name)
	}

	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tp, err := model.ReadTimePix(f, pix)
	if err != nil {
		return nil, fmt.Errorf("on file %q: %v", name, err)
	}
	return tp, nil
}

// PixWeight reads a pix weight file
// as defined in a project.
func (p *Project) PixWeight() (pixweight.Pixel, error) {
	name := p.Path(PixWeight)
	if name == "" {
		return nil, fmt.Errorf("pixel weights not defined in project %q", p.name)
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	pw, err := pixweight.ReadTSV(f)
	if err != nil {
		return nil, fmt.Errorf("when reading %q: %v", name, err)
	}
	return pw, nil
}

// Ranges reads a range collection file
// as defined in a project.
func (p *Project) Ranges(pix *earth.Pixelation) (*ranges.Collection, error) {
	name := p.Path(Ranges)
	if name == "" {
		return nil, fmt.Errorf("ranges not defined in project %q", p.name)
	}

	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	coll, err := ranges.ReadTSV(f, nil)
	if err != nil {
		return nil, fmt.Errorf("when reading %q: %v", name, err)
	}
	return coll, nil
}

// StageRotation reads a plate motion model
// as defined in a project,
// in the form of a stage rotations.
func (p *Project) StageRotation(pix *earth.Pixelation) (*model.StageRot, error) {
	name := p.Path(GeoMotion)
	if name == "" {
		return nil, fmt.Errorf("plate motion model not defined in project %q", p.name)
	}

	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rot, err := model.ReadStageRot(f, pix)
	if err != nil {
		return nil, fmt.Errorf("on file %q: %v", name, err)
	}
	return rot, nil
}

// Stages reads a set of stages
// as defined in a project.
func (p *Project) Stages(sts ...timestage.Stager) (timestage.Stages, error) {
	stages := timestage.New()
	for _, s := range sts {
		stages.Add(s)
	}

	name := p.Path(Stages)
	if name == "" {
		return stages, nil
	}

	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	st, err := timestage.Read(f)
	if err != nil {
		return nil, fmt.Errorf("when reading %q: %v", name, err)
	}
	stages.Add(st)

	return stages, nil
}

// TotalRotation reads a plate motion model
// as defined in a project,
// in the form of total rotations.
func (p *Project) TotalRotation(pix *earth.Pixelation, inverse bool) (*model.Total, error) {
	name := p.Path(GeoMotion)
	if name == "" {
		return nil, fmt.Errorf("plate motion model not defined in project %q", p.name)
	}

	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rot, err := model.ReadTotal(f, pix, inverse)
	if err != nil {
		return nil, fmt.Errorf("on file %q: %v", name, err)
	}
	return rot, nil
}

// Trees reads a tree collection file
// as defined in a project.
func (p *Project) Trees() (*timetree.Collection, error) {
	name := p.Path(Trees)
	if name == "" {
		return nil, fmt.Errorf("trees not defined in project %q", p.name)
	}

	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	c, err := timetree.ReadTSV(f)
	if err != nil {
		return nil, fmt.Errorf("while reading file %q: %v", name, err)
	}
	return c, nil

}
