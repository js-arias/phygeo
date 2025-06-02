// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package walk is a metapackage for commands
// that dealt with random walk models.
package walk

import (
	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/cmd/phygeo/walk/move"
	"github.com/js-arias/phygeo/cmd/phygeo/walk/settle"
	"github.com/js-arias/phygeo/cmd/phygeo/walk/traits"
)

var Command = &command.Command{
	Usage: "walk <command> [<argument>...]",
	Short: "commands for biogeographic inference with random walks",
}

func init() {
	Command.Add(move.Command)
	Command.Add(settle.Command)
	Command.Add(traits.Command)
}
