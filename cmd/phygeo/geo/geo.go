// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package geo is a metapackage for commands
// that dealt with paleogeographic reconstruction models.
package geo

import (
	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/cmd/phygeo/geo/add"
)

var Command = &command.Command{
	Usage: "geo <command> [<argument>...]",
	Short: "commands for paleogeographic reconstruction models",
}

func init() {
	Command.Add(add.Command)
}
