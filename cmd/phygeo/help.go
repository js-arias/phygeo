// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package main

import "github.com/js-arias/command"

func init() {
	app.Add(colorKeyGuide)
	app.Add(pixelPriorGuide)
	app.Add(projectsGuide)
	app.Add(rangeFilesGuide)
	app.Add(treeFilesGuide)
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
- Paleolandscape models. Defined by the dataset keyword "landscape". This file
  contains pixel values at different time stages in the form of a
  tab-delimited file. The recommended way to add a paleolandscape model is by
  using the command 'phygeo geo add'.
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
- Range models. Defined by the dataset keyword "ranges". This file contains
  the distribution range models of one or more taxons in the form of a
  tab-delimited file. The recommended way to add a presence-absence file is by
  using the command 'phygeo range add'.
	`,
}

var pixelPriorGuide = &command.Command{
	Usage: "pixel-priors",
	Short: "about the pixel prior file",
	Long: `
To take into account the landscape, each pixel must have a different prior, so
some pixels will be more likely to be sampled than others based on the
landscape features.

In PhyGeo, such priors can be defined in a file for pixel prior values. The
recommended way to interact with the priors is by using the command
"phygeo geo prior", which can be used to add a pixel prior file, view the
current priors, or set or edit values. See "phygeo help geo add" to learn
more.
  
A pixel prior file is a tab-delimited file with the following fields:
  
	- key    the value used as an identifier in the landscape model must 
                 be an integer.
	- prior  the prior of the pixel, it should be a value between 0 and 1.
  
Here is an example file:

	key	prior	comment
	0	0.000000	deep ocean
	1	0.010000	oceanic plateaus
	2	0.050000	continental shelf
	3	0.950000	lowlands
	4	1.000000	highlands
	5	0.001000	ice sheets

In this case, the comment column will be ignored.

In a PhyGeo project, the file that contains the pixel priors is indicated with
the "pixprior" keyword.
`,
}

var treeFilesGuide = &command.Command{
	Usage: "tree-files",
	Short: "about tree files",
	Long: `
In PhyGeo, phylogenetic trees must be time-calibrated and stored in a
tab-delimited file. The advantage of using a tab-delimited file is that it
would be easier to manipulate trees than in traditional newick files; for
example, it would be easier for commands in PhyGeo, as well as for third-party
applications, to understand the node IDs.

The recommended way to interact with time-calibrated trees in a PhyGeo project
is by using the commands in "phygeo tree".
	
A PhyGeo tree file is a tab-delimited file with the following columns:
	
	-tree    for the name of the tree.
	-node    for the ID of the node.
	-parent  for of ID of the parent node (-1 is used for the root).
	-age     the age of the node (in years).
	-taxon   the taxonomic name of the node.
	
Here is an example file:

	# time calibrated phylogenetic tree
	tree	node	parent	age	taxon
	dinosaurs	0	-1	235000000
	dinosaurs	1	0	230000000	Eoraptor lunensis
	dinosaurs	2	0	170000000
	dinosaurs	3	2	145000000	Ceratosaurus nasicornis
	dinosaurs	4	2	71000000	Carnotaurus sastrei
	
In a PhyGeo project, the file that contains the trees is indicated with the
"trees" keyword.
	`,
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

In PhyGeo, both kinds of distribution ranges are kept separate. By default,
commands that use distribution ranges use the points.

The recommended way to interact with distribution range files is with the
commands in "phygeo range". Type "phygeo range" to see the distribution range
commands, and "phygeo help range <command>" to learn more about a command.

A distribution range file is a tab-delimited file with the following columns:

	- taxon    the name of the taxon
	- type     the type of the range model. Can be "points" (for
	           presence-absence pixelation), or "range" (for a pixelated
	           range map).
	- age      for the age stage of the pixels (in years)
	- equator  for the number of pixels in the equator
	- pixel    the ID of a pixel (from the pixelation)
	- density  the density for the presence at that pixel

Here is an example file for a presence-absence pixels:

	# pixel presences
	taxon	type	age	equator	pixel	density
	Brontostoma discus	points	0	360	17319	1.000000
	Brontostoma discus	points	0	360	19117	1.000000
	Rhododendron ericoides	points	0	360	18588	1.000000
	Rhododendron ericoides	points	0	360	19305	1.000000
	Rhododendron ericoides	points	0	360	19308	1.000000

And an example file for a continuous distribution range map:

	# continuous distribution range models
	taxon	type	age	equator	pixel	density
	E. lunensis	range	230000000	360	34661	0.200000
	E. lunensis	range	230000000	360	34662	0.500000
	E. lunensis	range	230000000	360	34663	1.000000
	E. lunensis	range	230000000	360	34664	0.500000
	E. lunensis	range	230000000	360	34665	0.200000

In a PhyGeo project, the file that contains the presence-absence pixels is
indicated with the "points" keyword, and continuous range files with the
"ranges" keyword.
	`,
}

var colorKeyGuide = &command.Command{
	Usage: "color-keys",
	Short: "about color keys file",
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
