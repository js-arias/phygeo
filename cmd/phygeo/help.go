// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package main

import "github.com/js-arias/command"

func init() {
	app.Add(colorKeyGuide)
	app.Add(projectsGuide)
}

var projectsGuide = &command.Command{
	Usage: "projects",
	Short: "about project files",
	Long: `
PhyGeo requires several files to read and process biogeographic data. To
reduce the burden of keeping track of many files, a single project file is
used to hold the reference of all files required in the analysis. This guide
explains the structure of the file, but most of the time, the best and most
secure way to edit or view this file is by using phygeo commands.

A project file is a tab-delimited file with the following fields:

	- dataset  for the kind of file
	- path     for the path of the file

Here is an example file:

	# phygeo project files
	dataset	path
	geomotion	geo-motion.tab
	pixprior	pix-prior.tab
	ranges	ranges.tab
	landscape	landscape.tab
	trees	trees.tab

The valid file types are:

- Plate motion models. Defined by the dataset keyword "geomotion". This file
  contains the plate motion model in the form of a tab-delimited file. The
  recommended way to add a plate motion model is by using the command
  'phygeo geo add'.
- Landscape models. Defined by the dataset keyword "landscape". This file
  contains pixel values at different time stages in the form of a
  tab-delimited file. The recommended way to add a landscape model is by using
  the command 'phygeo geo add'.
- Pixel prior values. Defined by the dataset keyword "pixprior". This file
  contains the values used for the pixel priors in the form of a
  tab-delimited file. The recommended way to add a pixel prior file is by
  using the command 'phygeo geo prior'.
- Time-calibrated trees. Defined by the dataset keyword "trees". This file
  contains one or more trees in the form of a tab-delimited file. The
  recommended way to add a tree file is by using the command
  'phygeo tree add'.
- Presence-absence pixels. Defined by the dataset keyword "points". This file
  contains the pixels that indicate the presence of one or more taxons in the
  form of a tab-delimited file. The recommended way to add a presence-absence
  file is by using the command 'phygeo range add'.
- Geographic distribution ranges. Defined by the dataset keyword "ranges".
  This file contains the distribution range models of one or more taxons in
  the form of a tab-delimited file. The recommended way to add geographic
  range data is by using the command 'phygeo range add'.
	`,
}

var colorKeyGuide = &command.Command{
	Usage: "color-keys",
	Short: "about color keys files",
	Long: `
By default, mapping commands in PhyGeo use a plain gray background. A color
key file can be defined to set colors to the image map, using the project
landscape as the background.

A color key file is a tab-delimited file with the following columns:

	-key    the landscape value used as an identifier
	-color  a RGB value separated by commas, for example, "125,132,148".

Optionally, it can contain the following column:

	-gray   for a gray scale value (using the RGB scale)

Any other columns will be ignored.

Here is an example of a key file:

	key	color	gray	comment
	0	0, 26, 51	0	deep ocean
	1	0, 84, 119	10	oceanic plateaus
	2	68, 167, 196	20	continental shelf
	3	251, 236, 93	90	lowlands
	4	255, 165, 0	100	highlands
	5	229, 229, 224	50	ice sheets
	`,
}
