// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package rangecmd is a metapackage for commands
// that dealt with taxon distribution ranges.
package rangecmd

import (
	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/cmd/phygeo/rangecmd/add"
	"github.com/js-arias/phygeo/cmd/phygeo/rangecmd/kde"
	"github.com/js-arias/phygeo/cmd/phygeo/rangecmd/mapcmd"
	"github.com/js-arias/phygeo/cmd/phygeo/rangecmd/remove"
	"github.com/js-arias/phygeo/cmd/phygeo/rangecmd/rotate"
	"github.com/js-arias/phygeo/cmd/phygeo/rangecmd/taxa"
)

var Command = &command.Command{
	Usage: "range <command> [<argument>...]",
	Short: "commands for geographic distribution ranges",
}

func init() {
	Command.Add(add.Command)
	Command.Add(kde.Command)
	Command.Add(mapcmd.Command)
	Command.Add(remove.Command)
	Command.Add(rotate.Command)
	Command.Add(taxa.Command)

	// help guides
	Command.Add(rangeFilesGuide)
}

var rangeFilesGuide = &command.Command{
	Usage: "range-files",
	Short: "about distribution range files",
	Long: `
There are two ways in which distribution range data is stored in PhyGeo. In
the first form, the distribution range is expressed as presence-absence
pixels. Another way to see it is as a rasterization of the collection
localities of the specimen records. The second form, the distribution range,
is a continuous range map in which each pixel stores a scaled density that
indicates the relative likelihood of the species presence. In traditional
range maps, all densities are equal, whereas in distribution models, the
densities might be different.

In PhyGeo, both kinds of distribution ranges are kept in a single file, but
for each taxon only a single type will be used.

The recommended way to interact with distribution range files is with the
commands in "phygeo range". Type "phygeo range" to see the distribution range
commands, and "phygeo help range <command>" to learn more about a command.

A distribution range file is a tab-delimited file with the following columns:

	-taxon    the name of the taxon
	-type     the type of the range model. Can be "points" (for
	          presence-absence pixelation), or "range" (for a pixelated
	          range map).
	-age      for the age stage of the pixels (in years)
	-equator  for the number of pixels in the equator
	-pixel    the ID of a pixel (from the pixelation)
	-density  the density for the presence at that pixel

Here is an example file:

	# pixel presences
	taxon	type	age	equator	pixel	density
	Brontostoma discus	points	0	360	17319	1.000000
	Brontostoma discus	points	0	360	19117	1.000000
	E. lunensis	range	230000000	360	34661	0.200000
	E. lunensis	range	230000000	360	34662	0.500000
	E. lunensis	range	230000000	360	34663	1.000000
	E. lunensis	range	230000000	360	34664	0.500000
	E. lunensis	range	230000000	360	34665	0.200000
	Rhododendron ericoides	points	0	360	18588	1.000000
	Rhododendron ericoides	points	0	360	19305	1.000000
	Rhododendron ericoides	points	0	360	19308	1.000000

In a PhyGeo project, the file that contains the geographic distribution range
data is indicated with the "ranges" keyword.
	`,
}
