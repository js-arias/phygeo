// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package mapcmd implements a command to draw
// the dynamic geography model of a PhyGeo project.
package mapcmd

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/rand/v2"
	"os"

	"github.com/js-arias/blind"
	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/pixkey"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/phygeo/timestage"
)

var Command = &command.Command{
	Usage: `map [-c|--columns <value>]
	[--plates] [--at <age>] [--key <key-file>]
	[-o|--output <file-prefix>] <project-file>`,
	Short: "draw a map of the paleogeographic model",
	Long: `
Command map reads the paleogeographic model from a PhyGeo project and draws it
as a png image using a plate carrée projection.

The argument of the command is the name of the project file.

By default, it will draw the landscape model; use the flag --plates to draw
the plates of the plate motion model.

By default the image will be 3600 pixels wide; use the flag --columns, or -c,
to define a different number of image columns.

By default, all time stages will be produced. Use the flag --at to define a
particular time stage to be drawn (in million years).

By default, the pixel values in a landscape model and the plates in the plate
motion model will be colored at random. Use the flag --key to define a file
with the colors used for the landscape values.

By default, the output files will be prefixed as 'landscape' or 'plates' for
the landscape or the plate motion models, respectively. To set a different
prefix name, use the flag --output or -o. The name of the file will be in the
form '<prefix>-<age>.png' with the age in million years.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var plates bool
var colsFlag int
var atFlag float64
var keyFile string
var outPrefix string

func setFlags(c *command.Command) {
	c.Flags().BoolVar(&plates, "plates", false, "")
	c.Flags().IntVar(&colsFlag, "columns", 3600, "")
	c.Flags().IntVar(&colsFlag, "c", 3600, "")
	c.Flags().Float64Var(&atFlag, "at", -1, "")
	c.Flags().StringVar(&keyFile, "key", "", "")
	c.Flags().StringVar(&outPrefix, "output", "", "")
	c.Flags().StringVar(&outPrefix, "o", "", "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}

	p, err := project.Read(args[0])
	if err != nil {
		return err
	}

	if colsFlag%2 != 0 {
		colsFlag++
	}

	// plate motion model
	if plates {
		rec, err := p.GeoMotion(nil)
		if err != nil {
			return err
		}

		var ages []int64
		if atFlag >= 0 {
			ages = []int64{int64(atFlag * timestage.MillionYears)}
		} else {
			ages = rec.Stages()
		}

		pc := makePlatePalette(rec)
		if outPrefix == "" {
			outPrefix = "plates"
		}

		for _, a := range ages {
			name := fmt.Sprintf("%s-%d.png", outPrefix, a/timestage.MillionYears)
			if err := writeImage(name, makePlatesStage(rec, a, pc)); err != nil {
				return err
			}
		}
		return nil
	}

	// paleo-landscape model
	landscape, err := p.Landscape(nil)
	if err != nil {
		return err
	}

	var ages []int64
	if atFlag >= 0 {
		ages = []int64{int64(atFlag * timestage.MillionYears)}
	} else {
		ages = landscape.Stages()
	}

	var keys *pixkey.PixKey
	if keyFile != "" {
		keys, err = pixkey.Read(keyFile)
		if err != nil {
			return err
		}
	} else {
		keys = &pixkey.PixKey{}
		makeLandscapePalette(landscape, ages, keys)
	}

	if outPrefix == "" {
		outPrefix = "landscape"
	}

	for _, a := range ages {
		name := fmt.Sprintf("%s-%d.png", outPrefix, a/timestage.MillionYears)
		if err := writeImage(name, makeLandscapeStage(landscape, a, keys)); err != nil {
			return err
		}
	}
	return nil
}

// A stageModel stores the pixelation of a paleogeographic model.
type stageModel struct {
	step  float64
	color *pixkey.PixKey
	pix   *earth.Pixelation
	vals  map[int]int
}

func (s stageModel) ColorModel() color.Model { return color.RGBAModel }
func (s stageModel) Bounds() image.Rectangle { return image.Rect(0, 0, colsFlag, colsFlag/2) }
func (s stageModel) At(x, y int) color.Color {
	lat := 90 - float64(y)*s.step
	lon := float64(x)*s.step - 180

	pix := s.pix.Pixel(lat, lon).ID()
	c, ok := s.color.Color(s.vals[pix])
	if !ok {
		if plates {
			return color.RGBA{153, 153, 153, 255}
		}
		return color.RGBA{0, 0, 0, 0}
	}
	return c
}

func makePlatesStage(rec *model.Recons, age int64, pc *pixkey.PixKey) stageModel {
	plates := make(map[int]int, rec.Pixelation().Len())

	for _, p := range rec.Plates() {
		sp := rec.PixStage(p, age)
		for _, ids := range sp {
			for _, id := range ids {
				plates[id] = p
			}
		}
	}

	return stageModel{
		step:  360 / float64(colsFlag),
		color: pc,
		pix:   rec.Pixelation(),
		vals:  plates,
	}
}

func makeLandscapeStage(tp *model.TimePix, age int64, keys *pixkey.PixKey) stageModel {
	vals := make(map[int]int, tp.Pixelation().Len())

	for px := 0; px < tp.Pixelation().Len(); px++ {
		v, _ := tp.At(age, px)
		if v == 0 {
			continue
		}
		vals[px] = v
	}

	return stageModel{
		step:  360 / float64(colsFlag),
		color: keys,
		pix:   tp.Pixelation(),
		vals:  vals,
	}
}

func makePlatePalette(rec *model.Recons) *pixkey.PixKey {
	plates := rec.Plates()
	vals := make(map[int]bool)
	keys := &pixkey.PixKey{}

	for _, plate := range plates {
		vals[plate] = true
	}

	for v := range vals {
		keys.SetColor(randColor(), v)
	}
	return keys
}

func makeLandscapePalette(tp *model.TimePix, ages []int64, keys *pixkey.PixKey) {
	vals := make(map[int]bool)
	for _, a := range ages {
		for px := 0; px < tp.Pixelation().Len(); px++ {
			v, _ := tp.At(a, px)
			vals[v] = true
		}
	}
	for v := range vals {
		keys.SetColor(randColor(), v)
	}
}

func randColor() color.RGBA {
	return blind.Sequential(blind.Iridescent, rand.Float64())
}

func writeImage(name string, img image.Image) (err error) {
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

	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("when encoding image file %q: %v", name, err)
	}
	return nil
}
