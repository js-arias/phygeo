// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// PhyGeo is a tool for phylogenetic biogeography analysis.
package main

import (
	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/cmd/phygeo/diff"
	"github.com/js-arias/phygeo/cmd/phygeo/geo"
	"github.com/js-arias/phygeo/cmd/phygeo/rangecmd"
	"github.com/js-arias/phygeo/cmd/phygeo/tree"
)

var app = &command.Command{
	Usage: "phygeo <command> [<argument>...]",
	Short: "a tool for phylogenetic biogeography analysis",
}

func init() {
	app.Add(geo.Command)
	app.Add(diff.Command)
	app.Add(rangecmd.Command)
	app.Add(tree.Command)
}

func main() {
	app.Main()
}
