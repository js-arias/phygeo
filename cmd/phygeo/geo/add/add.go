// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package add implements a command to add a paleogeographic reconstruction model
// to a PhyGeo project.
package add

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/js-arias/command"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/phygeo/project"
)

var Command = &command.Command{
	Usage: "add --type <file-type> <project-file> <model-file>",
	Short: "add a paleogeographic reconstruction model",
	Long: `
Command add adds the path of a paleogeographic reconstruction model to a
PhyGeo project. The model can be a rotation model, or a time pixelation.

The first argument of the command is the name of the project file. If no
project exists, a new project will be created.

The second argument is the valid path of a model file. If there is a model
already defined in the project, its path will be replaced by the path of the
added file. The model can be either a rotation model (pixels locations in
time), or a time pixelation (pixel values on time). Both kind of models must
be compatible, i.e. based on the same underlying pixelation, and have the same
time stages.

The type of the added model must be explicitly defined using the flag --type
with one of the following values:

	geomod	for a rotation model
	timepix	for a time pixelation
	`,
	SetFlags: setFlags,
	Run:      run,
}

var typeFlag string

func setFlags(c *command.Command) {
	c.Flags().StringVar(&typeFlag, "type", "", "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}
	if len(args) < 2 {
		return c.UsageError("expecting model file")
	}
	if typeFlag == "" {
		return c.UsageError("flag --type undefined")
	}

	pFile := args[0]
	p, err := openProject(pFile)
	if err != nil {
		return err
	}

	typeFlag = strings.ToLower(typeFlag)
	switch d := project.Dataset(typeFlag); d {
	case project.GeoMod:
		if err := addGeoMod(p, args[1]); err != nil {
			return err
		}
	case project.TimePix:
		if err := addTimePix(p, args[1]); err != nil {
			return err
		}
	default:
		msg := fmt.Sprintf("flag --type: unknown value %q", typeFlag)
		return c.UsageError(msg)
	}

	if err := p.Write(pFile); err != nil {
		return err
	}

	return nil
}

func openProject(name string) (*project.Project, error) {
	p, err := project.Read(name)
	if errors.Is(err, os.ErrNotExist) {
		return project.New(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("unable ot open project %q: %v", name, err)
	}
	return p, nil
}

func addGeoMod(p *project.Project, path string) error {
	tot, err := readTotal(path)
	if err != nil {
		return err
	}

	tpPath := p.Path(project.TimePix)
	if tpPath == "" {
		p.Add(project.GeoMod, path)
		return nil
	}

	tp, err := readTimePix(tpPath)
	if err != nil {
		return fmt.Errorf("while reading TimePix: %v", err)
	}

	if eq1, eq2 := tot.Pixelation().Equator(), tp.Pixelation().Equator(); eq1 != eq2 {
		return fmt.Errorf("geomod file %q: got %d equatorial pixels, want %d", path, eq1, eq2)
	}
	if err := cmpStages(tot.Stages(), tp.Stages()); err != nil {
		return fmt.Errorf("geomod file %q: %v", path, err)
	}

	p.Add(project.GeoMod, path)
	return nil
}

func addTimePix(p *project.Project, path string) error {
	tp, err := readTimePix(path)
	if err != nil {
		return err
	}

	mPath := p.Path(project.GeoMod)
	if mPath == "" {
		p.Add(project.TimePix, path)
		return nil
	}

	tot, err := readTotal(mPath)
	if err != nil {
		return fmt.Errorf("while reading GeoModel: %v", err)
	}

	if eq1, eq2 := tp.Pixelation().Equator(), tot.Pixelation().Equator(); eq1 != eq2 {
		return fmt.Errorf("timepix file %q: got %d equatorial pixels, want %d", path, eq1, eq2)
	}
	if err := cmpStages(tp.Stages(), tot.Stages()); err != nil {
		return fmt.Errorf("timepix file %q: %v", path, err)
	}

	p.Add(project.TimePix, path)
	return nil
}

func readTimePix(name string) (*model.TimePix, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tp, err := model.ReadTimePix(f, nil)
	if err != nil {
		return nil, fmt.Errorf("on file %q: %v", name, err)
	}

	return tp, nil
}

func readTotal(name string) (*model.Total, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tot, err := model.ReadTotal(f, nil, false)
	if err != nil {
		return nil, fmt.Errorf("on file %q: %v", name, err)
	}

	return tot, nil
}

func cmpStages(st1, st2 []int64) error {
	if len(st1) > len(st2) {
		st1 = st1[:len(st2)]
	}
	if len(st2) > len(st1) {
		st2 = st2[:len(st1)]
	}

	if !reflect.DeepEqual(st1, st2) {
		return fmt.Errorf("got %v stages, want %v", st1, st2)
	}
	return nil
}
