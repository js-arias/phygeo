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
PhyGeo project. The model can be a plate motion model, or a paleolandscape.

The first argument of the command is the name of the project file. If no
project exists, a new project will be created.

The second argument is the valid path of a model file. If there is a model
already defined in the project, its path will be replaced by the path of the
added file. The model can be either a plate motion model (pixels locations in
time), or a landscape pixelation (pixel values on time). Both kind of models
must be compatible, i.e. based on the same underlying pixelation, and have the
same time stages.

The type of the added model must be explicitly defined using the flag --type
with one of the following values:

	geomotion	for a plate motion model
	landscape	for a landscape model
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
	case project.GeoMotion:
		if err := addGeoMotion(p, args[1]); err != nil {
			return err
		}
	case project.Landscape:
		if err := addLandscape(p, args[1]); err != nil {
			return err
		}
	default:
		msg := fmt.Sprintf("flag --type: unknown value %q", typeFlag)
		return c.UsageError(msg)
	}

	if err := p.Write(); err != nil {
		return err
	}

	return nil
}

func openProject(name string) (*project.Project, error) {
	p, err := project.Read(name)
	if errors.Is(err, os.ErrNotExist) {
		p := project.New()
		p.SetName(name)
		return p, nil
	}
	if err != nil {
		return nil, fmt.Errorf("unable ot open project %q: %v", name, err)
	}
	return p, nil
}

func addGeoMotion(p *project.Project, path string) error {
	tot, err := readTotal(path)
	if err != nil {
		return err
	}

	tpPath := p.Path(project.Landscape)
	if tpPath == "" {
		p.Add(project.GeoMotion, path)
		return nil
	}

	tp, err := readLandscape(tpPath)
	if err != nil {
		return fmt.Errorf("while reading Landscape: %v", err)
	}

	if eq1, eq2 := tot.Pixelation().Equator(), tp.Pixelation().Equator(); eq1 != eq2 {
		return fmt.Errorf("geomotion file %q: got %d equatorial pixels, want %d", path, eq1, eq2)
	}
	if err := cmpStages(tot.Stages(), tp.Stages()); err != nil {
		return fmt.Errorf("geomotion file %q: %v", path, err)
	}

	p.Add(project.GeoMotion, path)
	return nil
}

func addLandscape(p *project.Project, path string) error {
	tp, err := readLandscape(path)
	if err != nil {
		return err
	}

	mPath := p.Path(project.GeoMotion)
	if mPath == "" {
		p.Add(project.Landscape, path)
		return nil
	}

	tot, err := readTotal(mPath)
	if err != nil {
		return fmt.Errorf("while reading GeoMotion: %v", err)
	}

	if eq1, eq2 := tp.Pixelation().Equator(), tot.Pixelation().Equator(); eq1 != eq2 {
		return fmt.Errorf("landscape file %q: got %d equatorial pixels, want %d", path, eq1, eq2)
	}
	if err := cmpStages(tp.Stages(), tot.Stages()); err != nil {
		return fmt.Errorf("landscape file %q: %v", path, err)
	}

	p.Add(project.Landscape, path)
	return nil
}

func readLandscape(name string) (*model.TimePix, error) {
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
