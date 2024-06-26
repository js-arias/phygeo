// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// PGS is a tool for simulations of biogeographic data
// using spherical diffusion
// and dynamic geography.
package main

import (
	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/cmd/pgs/cmpcmd"
	"github.com/js-arias/phygeo/cmd/pgs/freq"
	"github.com/js-arias/phygeo/cmd/pgs/infer"
	"github.com/js-arias/phygeo/cmd/pgs/sim"
	"github.com/js-arias/phygeo/cmd/pgs/unrot"
)

var app = &command.Command{
	Usage: "pgs <command> [<argument>...]",
	Short: "a tool for simulations of biogeographic data",
}

func init() {
	app.Add(cmpcmd.Command)
	app.Add(freq.Command)
	app.Add(infer.Command)
	app.Add(sim.Command)
	app.Add(unrot.Command)
}

func main() {
	app.Main()
}
