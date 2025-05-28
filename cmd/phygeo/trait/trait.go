// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package trait is a metapackage for commands
// that dealt with trait data models.
package trait

import (
	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/cmd/phygeo/trait/add"
	"github.com/js-arias/phygeo/cmd/phygeo/trait/move"
)

var Command = &command.Command{
	Usage: "trait <command> [<argument>...]",
	Short: "commands for trait models used in random walks",
}

func init() {
	Command.Add(add.Command)
	Command.Add(move.Command)
}
