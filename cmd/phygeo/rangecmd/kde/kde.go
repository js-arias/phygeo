// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package kde implements a command
// to estimate the range distributions
// using a kernel density estimator.
package kde

import (
	"errors"
	"fmt"
	"os"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/stat"
	"github.com/js-arias/earth/stat/dist"
	"github.com/js-arias/earth/stat/pixprob"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/ranges"
)

var Command = &command.Command{
	Usage: `kde [--lambda <value>] [--bound <value>]
	[-f|--file <file>] <project-file>`,
	Short: "estimate geographic ranges using a KDE",
	Long: `
Command kde reads the point locations from a PhyGeo project and produces new
range maps using a kernel density estimation based on a spherical normal. It
will only add taxa without a defined range map.

The argument of the command is the name of the project file.
	
The flag --lambda defines the concentration parameter of the spherical normal
(equivalent to the kappa parameter in the von Mises-Fisher distribution) in
1/radians^2 units. If no value is defined, it will use the 1/size^2 of a pixel
in the pixelation used for the project.
	
By default, only the pixel at 0.95 of the spherical normal CDF will be used.
Use the flag --bound to set the bound of the normal CDF.
	
By default, the range maps will be stored in the range file currently defined
for the project. A different file name can be defined with the flag --file or
-f. If this flag is used a new file will be created and used as the range file
of the project (previously defined ranges will be kept).
	`,
	SetFlags: setFlags,
	Run:      run,
}

var lambdaFlag float64
var boundFlag float64
var outFile string

func setFlags(c *command.Command) {
	c.Flags().Float64Var(&lambdaFlag, "lambda", 0, "")
	c.Flags().Float64Var(&boundFlag, "bound", 0.95, "")
	c.Flags().StringVar(&outFile, "file", "", "")
	c.Flags().StringVar(&outFile, "f", "", "")
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

	lsf := p.Path(project.Landscape)
	if lsf == "" {
		msg := fmt.Sprintf("landscape not defined in project %q", args[0])
		return c.UsageError(msg)
	}
	landscape, err := readLandscape(lsf)
	if err != nil {
		return err
	}

	ppF := p.Path(project.PixPrior)
	if ppF == "" {
		msg := fmt.Sprintf("pixel priors not defined in project %q", args[0])
		return c.UsageError(msg)
	}
	pp, err := readPriors(ppF)
	if err != nil {
		return err
	}

	var rng *ranges.Collection
	rf := p.Path(project.Ranges)
	if rf != "" {
		rng, err = readRanges(rf)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("%s: undefined range file", pFile)
	}
	if outFile == "" {
		outFile = rf
	}

	if lambdaFlag == 0 {
		angle := earth.ToRad(landscape.Pixelation().Step())
		lambdaFlag = 1 / (angle * angle)
		fmt.Fprintf(c.Stderr(), "# Using lambda value of: %.6f\n", lambdaFlag)
	}
	n := dist.NewNormal(lambdaFlag, landscape.Pixelation())

	for _, tax := range rng.Taxa() {
		if rng.Type(tax) == ranges.Range {
			continue
		}

		px := rng.Range(tax)
		age := rng.Age(tax)
		kde := stat.KDE(n, px, landscape, age, pp)
		taxKDE := make(map[int]float64)
		for pt, p := range kde {
			if p < 1-boundFlag {
				continue
			}
			taxKDE[pt] = p
		}
		rng.Set(tax, age, taxKDE)
	}

	if err := writeCollection(outFile, rng); err != nil {
		return err
	}
	p.Add(project.Ranges, outFile)

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

func readPriors(name string) (pixprob.Pixel, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	pp, err := pixprob.ReadTSV(f)
	if err != nil {
		return nil, fmt.Errorf("when reading %q: %v", name, err)
	}

	return pp, nil
}

func readRanges(name string) (*ranges.Collection, error) {
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
