// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package newcmd implements a command to create
// a default model
// for a PhyGeo project.
package newcmd

import (
	"os"

	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/infer/model"
	"github.com/js-arias/phygeo/project"
)

var Command = &command.Command{
	Usage: "new <project> <model-file>",
	Short: "create a new default model",
	Long: `
Command new reads a PhyGeo project and build a new default model parameters
for inferences using random walks or diffusion.

The first argument of the command is the name of the project file-

The second argument is the name of the file that will store the new model.

The model will set all settlement weights as 1.00 as a fixed value, and all
movement wights as a different parameter.
	`,
	Run: run,
}

func run(c *command.Command, args []string) (err error) {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}
	if len(args) < 2 {
		return c.UsageError("expecting model file name")
	}

	p, err := project.Read(args[0])
	if err != nil {
		return err
	}

	landscape, err := p.Landscape(nil)
	if err != nil {
		return err
	}

	tr, err := p.Traits()
	if err != nil {
		return err
	}

	keys, err := p.Keys()
	if err != nil {
		return err
	}

	mp := model.Default(landscape.Pixelation(), tr, keys)
	f, err := os.Create(args[1])
	if err != nil {
		return err
	}
	defer func() {
		e := f.Close()
		if err == nil && e != nil {
			err = e
		}
	}()
	if err := mp.TSV(f); err != nil {
		return err
	}
	return nil
}
