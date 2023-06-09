// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package add implements a command to add taxon ranges
// to a PhyGeo project.
package add

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/ranges"
)

var Command = &command.Command{
	Usage: `add [-f|--file <range-file>] [--type <file-type>]
	<project-file> [<range-file>...]`,
	Short: "add taxon ranges to a PhyGeo project",
	Long: `
Command add reads one or more taxon ranges from one or more range files, and
add the ranges to a PhyGeo project. The ranges can be either presence-absence
pixelations, or a continuous range maps.

The first argument of the command is the name of the project file. If no
project exists, a new project will be created.

One or more range files can be given as arguments. If no file is given the
ranges will be read from the standard input. A pixelation model must be
already defined for the project, either a rotation model, or a paleolandscape
model, and the pixelation of the input files must be consistent with that
pixelation model.

By default, only the taxon with ranges defined as presence-absence will be
read. Use the flag --type to define the type of the ranges to be read. The
type can be:

	points	presence-absence taxon ranges
	ranges	continuous range map

By default the range maps will be stored in the range files currently defined
for the project. If the project does not have a range file, a new one will be
created with the name 'points.tab' for presence-absence taxon ranges, or
'ranges.tab' for continuous range maps. A different file name can be defined
with the flag --file or -f. If this flag is used, and there is a range file
already defined, then a new file will be created, and used as the range file
for the added type of range map for the project (previously defined ranges
will be kept).
	`,
	SetFlags: setFlags,
	Run:      run,
}

var outFile string
var typeFlag string

func setFlags(c *command.Command) {
	c.Flags().StringVar(&outFile, "file", "", "")
	c.Flags().StringVar(&outFile, "f", "", "")
	c.Flags().StringVar(&typeFlag, "type", "", "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}
	pFile := args[0]
	p, err := openProject(pFile)
	if err != nil {
		return err
	}

	typeFlag = strings.ToLower(typeFlag)
	if typeFlag == "" {
		typeFlag = string(project.Points)
	}
	switch d := project.Dataset(typeFlag); d {
	case project.Points:
		if err := addPoints(c.Stdin(), p, args[1:]); err != nil {
			return err
		}
	case project.Ranges:
		if err := addRanges(c.Stdin(), p, args[1:]); err != nil {
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

func addPoints(r io.Reader, p *project.Project, files []string) error {
	pix, err := openPixelation(p)
	if err != nil {
		return err
	}

	var coll *ranges.Collection
	if pf := p.Path(project.Points); pf != "" {
		var err error
		coll, err = readCollection(r, pf)
		if err != nil {
			return err
		}
		if eq1, eq2 := pix.Equator(), coll.Pixelation().Equator(); eq1 != eq2 {
			return fmt.Errorf("invalid project file %q: got %d equatorial pixel, want %d", pf, eq2, eq1)
		}
	} else {
		coll = ranges.New(pix)
	}

	if len(files) == 0 {
		files = append(files, "-")
	}
	for _, f := range files {
		c, err := readCollection(r, f)
		if err != nil {
			return err
		}
		cp := c.Pixelation()

		for _, nm := range c.Taxa() {
			if c.Type(nm) != ranges.Points {
				continue
			}
			age := c.Age(nm)
			rng := c.Range(nm)
			for id := range rng {
				pt := cp.ID(id).Point()
				coll.Add(nm, age, pt.Latitude(), pt.Longitude())
			}
		}
	}
	if len(coll.Taxa()) == 0 {
		return nil
	}

	ptsFile := p.Path(project.Points)
	if outFile != "" {
		ptsFile = outFile
	}
	if ptsFile == "" {
		ptsFile = "points.tab"
	}

	if err := writeCollection(ptsFile, coll); err != nil {
		return err
	}
	p.Add(project.Points, ptsFile)
	return nil
}

func addRanges(r io.Reader, p *project.Project, files []string) error {
	pix, err := openPixelation(p)
	if err != nil {
		return err
	}

	var coll *ranges.Collection
	if rf := p.Path(project.Ranges); rf != "" {
		var err error
		coll, err = readCollection(r, rf)
		if err != nil {
			return err
		}
		if eq1, eq2 := pix.Equator(), coll.Pixelation().Equator(); eq1 != eq2 {
			return fmt.Errorf("invalid project file %q: got %d equatorial pixel, want %d", rf, eq2, eq1)
		}
	} else {
		coll = ranges.New(pix)
	}

	if len(files) == 0 {
		files = append(files, "-")
	}
	for _, f := range files {
		c, err := readCollection(r, f)
		if err != nil {
			return err
		}
		if eq1, eq2 := pix.Equator(), c.Pixelation().Equator(); eq1 != eq2 {
			return fmt.Errorf("invalid range file %q: got %d equatorial pixel, want %d", f, eq2, eq1)
		}

		for _, nm := range c.Taxa() {
			if c.Type(nm) != ranges.Range {
				continue
			}
			age := c.Age(nm)
			rng := c.Range(nm)
			coll.Set(nm, age, rng)
		}
	}

	if len(coll.Taxa()) == 0 {
		return nil
	}

	rngFile := p.Path(project.Ranges)
	if outFile != "" {
		rngFile = outFile
	}
	if rngFile == "" {
		rngFile = "ranges.tab"
	}

	if err := writeCollection(rngFile, coll); err != nil {
		return err
	}
	p.Add(project.Ranges, rngFile)
	return nil
}

func openPixelation(p *project.Project) (*earth.Pixelation, error) {
	if path := p.Path(project.Landscape); path != "" {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		tp, err := model.ReadTimePix(f, nil)
		if err != nil {
			return nil, fmt.Errorf("on file %q: %v", path, err)
		}
		return tp.Pixelation(), nil
	}
	if path := p.Path(project.GeoMotion); path != "" {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		tot, err := model.ReadTotal(f, nil, false)
		if err != nil {
			return nil, fmt.Errorf("on file %q: %v", path, err)
		}
		return tot.Pixelation(), nil
	}
	return nil, errors.New("undefined pixelation model")
}

func readCollection(r io.Reader, name string) (*ranges.Collection, error) {
	if name != "-" {
		f, err := os.Open(name)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
	} else {
		name = "stdin"
	}

	coll, err := ranges.ReadTSV(r, nil)
	if err != nil {
		return nil, fmt.Errorf("when reading %q: %v", name, err)
	}

	return coll, nil
}

func writeCollection(name string, coll *ranges.Collection) (err error) {
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	defer func() {
		e := f.Close()
		if e != nil && err == nil {
			err = e
		}
	}()

	if err := coll.TSV(f); err != nil {
		return fmt.Errorf("while writing to %q: %v", name, err)
	}
	return nil
}
