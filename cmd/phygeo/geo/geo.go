// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package geo is a metapackage for commands
// that dealt with paleogeographic reconstruction models.
package geo

import (
	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/cmd/phygeo/geo/add"
	"github.com/js-arias/phygeo/cmd/phygeo/geo/prior"
)

var Command = &command.Command{
	Usage: "geo <command> [<argument>...]",
	Short: "commands for paleogeographic reconstruction models",
}

func init() {
	Command.Add(add.Command)
	Command.Add(prior.Command)

	// help guides
	Command.Add(landscapeModelGuide)
	Command.Add(motionModelGuide)
	Command.Add(pixelPriorGuide)
}

var landscapeModelGuide = &command.Command{
	Usage: "landscape",
	Short: "about landscape models",
	Long: `
A landscape model (or paleolandscape model) stores pixel values at different
times. These values are usually associated with a particular landscape feature
(for example, 1 is associated with shallow seas, while 5 is associated with
high-altitude mountains). In PhyGeo, the landscape models are stored as
tab-delimited files.

There are two uses for the landscape models in PhyGeo. The first, and most
important, is for the analysis. The landscape features are associated with
pixel priors. Therefore, the landscape feature modifies the probability of
assigning a pixel as part of the ancestral range. In the second usage, mapping
commands use feature values to assign background colors.

The landscape models are closely associated with plate motion models. In
PhyGeo, both models should have the same spatial and temporal resolution. They
are kept separated for flexibility; for example, you can have two different
landscape models using the same plate motion model.

Landscape models are taken as given in PhyGeo. If you want to work with
landscape models, the recommended way is to use the tools 'plates' or
'platesgui' available at: <https://github.com/js-arias/earth>. Here is a
repository with a collection of landscape models:
<https://github.com/js-arias/geomodels>.

A landscape model is a tab-delimited file with the following columns:

	-equator      for the size of the pixelation (the number of pixels in
	              the equatorial ring).
	-age          the time stage, in years.	
	-stage-pixel  the ID of a pixel at the indicated time stage.
	-value         an integer value associated with a landscape feature.

Here is an example file:

	equator	age	stage-pixel	value
	360	100000000	19051	1
	360	100000000	19055	2
	360	100000000	19409	1
	360	140000000	20051	1
	360	140000000	20055	2
	360	140000000	20056	3

In a PhyGeo project, the file that contains the landscape model is indicated
with the "landscape" keyword.
	`,
}

var motionModelGuide = &command.Command{
	Usage: "motion-model",
	Short: "about plate motion models",
	Long: `
A plate motion model stores the locations of tectonic features at different
times. While geologists use a vectorial and time-continuous rotation model, in
PhyGeo, a rasterized and time-discrete version of such models is stored as a
tab-delimited file.

Plate motion models are taken as given in PhyGeo. If you want to work with
plate motion models, the recommended way is to use the tool 'plates',
available at: <https://github.com/js-arias/earth>. Here is a repository with a
collection of plate motion models: <https://github.com/js-arias/geomodels>.

A plate motion model file is a tab-delimited file with the following columns:

	-equator      for the size of the pixelation (the number of pixels in
	              the equatorial ring).
	-plate        the ID of the tectonic plate.
	-pixel        the ID of the pixel at the present location (from the
	              pixelation).
	-age          the time stage, in years.
	-stage-pixel  the ID of the pixel at the indicated time stage.
	
Here is an example file:

	equator	plate	pixel	age	stage-pixel
	360	59999	17051	100000000	19051
	360	59999	17051	140000000	20051
	360	59999	17055	100000000	19055
	360	59999	17055	140000000	20055
	360	59999	17055	140000000	20056

In a PhyGeo project, the file that contains the plate motion model is
indicated with the "geomotion" keyword.
	`,
}

var pixelPriorGuide = &command.Command{
	Usage: "pixel-priors",
	Short: "about the pixel prior files",
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
