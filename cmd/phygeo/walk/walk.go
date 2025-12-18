// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package walk is a metapackage for commands
// that dealt with random walk models.
package walk

import (
	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/cmd/phygeo/walk/freq"
	"github.com/js-arias/phygeo/cmd/phygeo/walk/lambda"
	"github.com/js-arias/phygeo/cmd/phygeo/walk/like"
	"github.com/js-arias/phygeo/cmd/phygeo/walk/mapcmd"
	"github.com/js-arias/phygeo/cmd/phygeo/walk/move"
	"github.com/js-arias/phygeo/cmd/phygeo/walk/param"
	"github.com/js-arias/phygeo/cmd/phygeo/walk/particles"
	"github.com/js-arias/phygeo/cmd/phygeo/walk/settle"
	"github.com/js-arias/phygeo/cmd/phygeo/walk/speed"
	"github.com/js-arias/phygeo/cmd/phygeo/walk/traits"
)

var Command = &command.Command{
	Usage: "walk <command> [<argument>...]",
	Short: "commands for biogeographic inference with random walks",
}

func init() {
	Command.Add(freq.Command)
	Command.Add(lambda.Command)
	Command.Add(like.Command)
	Command.Add(mapcmd.Command)
	Command.Add(move.Command)
	Command.Add(param.Command)
	Command.Add(particles.Command)
	Command.Add(settle.Command)
	Command.Add(speed.Command)
	Command.Add(traits.Command)
}
