// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package model is a metapackage for commands
// that dealt in model parameters management.
package model

import (
	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/cmd/phygeo/model/newcmd"
	"github.com/js-arias/phygeo/cmd/phygeo/model/param"
	"github.com/js-arias/phygeo/cmd/phygeo/model/val"
)

var Command = &command.Command{
	Usage: "model <command> [<arguments>...]",
	Short: "commands for model parameters",
}

func init() {
	Command.Add(newcmd.Command)
	Command.Add(param.Command)
	Command.Add(val.Command)
}
