// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package pixel implements a command to print
// the information for a pixel in the model.
package pixel

import (
	"fmt"
	"os"
	"strconv"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/vector"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/phygeo/timestage"
)

var Command = &command.Command{
	Usage: `pixel [--id] <project> [<value>...]`,
	Short: "get pixel information",
	Long: `
Command pixel retrieves a pixel location, its landscape features, and its
previous locations for the paleogeographic model defined for a PhyGeo project.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var idFlag bool

func setFlags(c *command.Command) {
	c.Flags().BoolVar(&idFlag, "id", false, "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}

	p, err := project.Read(args[0])
	if err != nil {
		return err
	}

	args = args[1:]
	if !idFlag && len(args) < 2 {
		return c.UsageError("expecting <latitude> <longitude> arguments")
	}

	recF := p.Path(project.GeoMotion)
	if recF == "" {
		msg := fmt.Sprintf("plate motion model not defined in project %q", args[0])
		return c.UsageError(msg)
	}
	rec, err := readRecons(recF)
	if err != nil {
		return err
	}
	pix := rec.Pixelation()

	// paleo-landscape model
	lsf := p.Path(project.Landscape)
	if lsf == "" {
		msg := fmt.Sprintf("landscape not defined in project %q", args[0])
		return c.UsageError(msg)
	}
	landscape, err := readLandscape(lsf, pix)
	if err != nil {
		return err
	}

	var px earth.Pixel
	if idFlag {
		pixID, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid pixel ID %q: %v", args[0], err)
		}
		if pixID < 0 || pixID > pix.Len() {
			return fmt.Errorf("invalid pixel ID %q", args[0])
		}
		px = pix.ID(pixID)
	} else {
		pt, err := vector.ParsePoint(args[0], args[1])
		if err != nil {
			return err
		}
		px = pix.Pixel(pt.Lat, pt.Lon)
	}

	fmt.Fprintf(c.Stdout(), "# pixel %d, lat: %.6f, lon: %.6f\n", px.ID(), px.Point().Latitude(), px.Point().Longitude())
	plL := plates(rec, px.ID())
	fmt.Fprintf(c.Stdout(), "plate\tage\tpixel\tvalue\n")
	if len(plL) == 0 {
		v, _ := landscape.At(0, px.ID())
		fmt.Fprintf(c.Stdout(), "--\t%.6f\t%d\t%d\n", 0.0, px.ID(), v)
		return nil
	}
	for _, pID := range plL {
		for _, a := range rec.Stages() {
			r := rec.PixStage(pID, a)
			if r == nil {
				break
			}
			ax := landscape.ClosestStageAge(a)
			np := r[px.ID()]
			for _, px := range np {
				v, _ := landscape.At(ax, px)
				fmt.Fprintf(c.Stdout(), "%d\t%.6f\t%d\t%d\n", pID, float64(a)/timestage.MillionYears, px, v)
			}
		}
	}

	return nil
}

func readRecons(name string) (*model.Recons, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}

	rec, err := model.ReadReconsTSV(f, nil)
	if err != nil {
		return nil, fmt.Errorf("when reading file %q: %v", name, err)
	}
	return rec, nil
}

func readLandscape(name string, pix *earth.Pixelation) (*model.TimePix, error) {
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

func plates(rec *model.Recons, px int) []int {
	var pl []int
	for _, p := range rec.Plates() {
		for _, id := range rec.Pixels(p) {
			if id == px {
				pl = append(pl, p)
				break
			}
		}
	}

	return pl
}
