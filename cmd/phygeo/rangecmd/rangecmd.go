// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package rangecmd is a metapackage for commands
// that dealt with taxon distribution ranges.
package rangecmd

import (
	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/cmd/phygeo/rangecmd/add"
	"github.com/js-arias/phygeo/cmd/phygeo/rangecmd/taxa"
)

var Command = &command.Command{
	Usage: "range <command> [<argument>...]",
	Short: "commands for geographic distribution ranges",
}

func init() {
	Command.Add(add.Command)
	Command.Add(taxa.Command)
}
