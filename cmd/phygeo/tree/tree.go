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
	"github.com/js-arias/phygeo/cmd/phygeo/tree/remove"
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
	Command.Add(remove.Command)
	Command.Add(terms.Command)

	// help topics
	Command.Add(treeFilesGuide)
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
