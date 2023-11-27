// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package tree is a metapackage for commands
// that dealt with phylogenetic trees.
package tree

import (
	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/cmd/phygeo/tree/add"
	"github.com/js-arias/phygeo/cmd/phygeo/tree/draw"
	"github.com/js-arias/phygeo/cmd/phygeo/tree/list"
	"github.com/js-arias/phygeo/cmd/phygeo/tree/terms"
)

var Command = &command.Command{
	Usage: "tree <command> [<argument>...]",
	Short: "commands for phylogenetic trees",
}

func init() {
	Command.Add(add.Command)
	Command.Add(draw.Command)
	Command.Add(list.Command)
	Command.Add(terms.Command)
}
