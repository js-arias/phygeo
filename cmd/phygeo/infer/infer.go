// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package infer is a metapackage for commands
// that dealt with the biogeographic inference.
package infer

import (
	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/cmd/phygeo/infer/difflike"
	"github.com/js-arias/phygeo/cmd/phygeo/infer/mapcmd"
)

var Command = &command.Command{
	Usage: "infer <command> [<argument>...]",
	Short: "commands for biogeographic inference",
}

func init() {
	Command.Add(difflike.Command)
	Command.Add(mapcmd.Command)
}
