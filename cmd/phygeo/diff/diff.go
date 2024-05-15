// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package diff is a metapackage for commands
// that dealt with the biogeographic inference
// using a diffusion model.
package diff

import (
	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/cmd/phygeo/diff/freq"
	"github.com/js-arias/phygeo/cmd/phygeo/diff/integrate"
	"github.com/js-arias/phygeo/cmd/phygeo/diff/like"
	"github.com/js-arias/phygeo/cmd/phygeo/diff/mapcmd"
	"github.com/js-arias/phygeo/cmd/phygeo/diff/ml"
	"github.com/js-arias/phygeo/cmd/phygeo/diff/particles"
	"github.com/js-arias/phygeo/cmd/phygeo/diff/speed"
)

var Command = &command.Command{
	Usage: "diff <command> [<argument>...]",
	Short: "commands for biogeographic inference with diffusion",
}

func init() {
	Command.Add(freq.Command)
	Command.Add(integrate.Command)
	Command.Add(like.Command)
	Command.Add(mapcmd.Command)
	Command.Add(ml.Command)
	Command.Add(particles.Command)
	Command.Add(speed.Command)

	// help topics
	Command.Add(pixProbGuide)
	Command.Add(stochasticMappingGuide)
}

var pixProbGuide = &command.Command{
	Usage: "pix-prob-files",
	Short: "pixel probability files",
	Long: `
Pixel probability files are used in PhyGeo to store the particular probability
of a pixel in a node at a given time stage. In PhyGeo, the main usage of this
kind of file is as an input to build the reconstruction maps.

A pixel probability file is a tab-delimited file with the following columns:

	-tree     the name of the tree	
	-node     the ID of the node
	-age      the age of the time stage, in years
	-type     the type of the stored probability. It can be "log-like" for
	          log-likelihood values (for example, the output of the
	          "diff like" command), "freq" for the raw frequency of a
	          pixel, or "kde" for the smoothed frequency of a pixel (both
	          can be produced by the output of the "diff freq" command).
	-equator  the number of pixels in the equator of the pixelation
	-pixel    the ID of the pixel (from the pixelation)
	-value    the probability value of the pixel.

The file can also include the following columns:

	-lambda  in the log-like files, this column indicates the lambda value
	         used for the likelihood calculations.

Here are some example files:

	# logLike file
	tree	node	age	type	lambda	equator	pixel	value
	vireya	0	18249000	log-like	100.0	120	0	-426.6
	vireya	0	18249000	log-like	100.0	120	1	-426.9
	vireya	0	18249000	log-like	100.0	120	2	-427.8
	vireya	0	18249000	log-like	100.0	120	3	-427.3

	# freq file
	tree	node	age	type	equator	pixel	value
	vireya	1	15000000	freq	120	873	0.004000
	vireya	1	15000000	freq	120	874	0.004000
	vireya	1	15000000	freq	120	875	0.002000
	vireya	1	15000000	freq	120	876	0.003000

	# kde file
	vireya	2	15000000	kde	120	1609	0.035273
	vireya	2	15000000	kde	120	1610	0.075713
	vireya	2	15000000	kde	120	1611	0.162439
	vireya	2	15000000	kde	120	1612	0.337214
	vireya	2	15000000	kde	120	1613	0.255504
	`,
}

var stochasticMappingGuide = &command.Command{
	Usage: "mapping",
	Short: "about stochastic mapping",
	Long: `
In PhyGeo, stochastic mapping is used to estimate probable biogeographic
histories given a likelihood estimation. To perform stochastic mapping, use
the command "diff particles". Commands such as "diff freq" and "diff speed"
use the results of stochastic mapping.

The main procedure of stochastic mapping consists of simulating a set of
particles. The particle at the root was selected at random from the likelihood
estimation at the root. Then, at each time stage, the next location of the
particle is selected using its location at the previous stage, the diffusion
concentration, and the conditional likelihood estimated in the down-pass. At
split nodes, each descendant starts in the same location. See Arias
(doi:10.1101/2023.11.16.567427) for more details.

The results of stochastic mapping are stored in a tab-delimited file with the
following columns:

	-tree      the name of the tree
	-particle  the ID of a particular simulation
	-node      the ID of the node
	-age       the age of the time stage, in years
	-lambda    the lambda value used for the likelihood calculations.
	-equator   the number of pixels in the equator for the pixelation used
	           in the simulations.
	-from      the ID of the pixel location at the start of the time stage.
	-to        the ID of the pixel location at the end of the time stage.

Here is an example of a stochastic-mapping file:

	# stochastic mapping
	tree	particle	node	age	lambda	equator	from	to
	vireya	0	0	16590000	100.0	120	970	1175
	vireya	0	1	15000000	100.0	120	1175	1075
	vireya	0	1	10000000	100.0	120	1075	1074
	vireya	0	2	15000000	100.0	120	1175	1503
	vireya	0	2	12300000	100.0	120	1503	2094
	vireya	0	3	10000000	100.0	120	2094	2095

Nielsen (Syst. Biol. 51:729, 2002) proposed stochastic mapping based on Yang
et al. (Genetics 141:1641, 1995). Dupin et al. (J. Biogeogr. 44: 887, 2017)
implemented it for phylogenetic biogeography.
	`,
}
