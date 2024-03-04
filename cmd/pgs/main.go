// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// PGS is a tool for simulations of biogeographic data
// using spherical diffusion
// and dynamic geography.
package main

import (
	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/cmd/pgs/sim"
)

var app = &command.Command{
	Usage: "pgs <command> [<argument>...]",
	Short: "a tool for simulations of biogeographic data",
}

func init() {
	app.Add(sim.Command)
}

func main() {
	app.Main()
}
