// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package diff is a metapackage for commands
// that dealt with the biogeographic inference
// using a diffusion model.
package diff

import (
	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/cmd/phygeo/diff/integrate"
	"github.com/js-arias/phygeo/cmd/phygeo/diff/like"
	"github.com/js-arias/phygeo/cmd/phygeo/diff/mapcmd"
	"github.com/js-arias/phygeo/cmd/phygeo/diff/ml"
	"github.com/js-arias/phygeo/cmd/phygeo/diff/speed"
)

var Command = &command.Command{
	Usage: "diff <command> [<argument>...]",
	Short: "commands for biogeographic inference with diffusion",
}

func init() {
	Command.Add(integrate.Command)
	Command.Add(like.Command)
	Command.Add(mapcmd.Command)
	Command.Add(ml.Command)
	Command.Add(speed.Command)
}
