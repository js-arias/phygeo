// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package add implements a command to add trees
// to a PhyGeo project.
package add

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/phygeo/timestage"
	"github.com/js-arias/timetree"
)

var Command = &command.Command{
	Usage: `add [-f|--file <tree-file>]
	[--newick <name>] [--age <value>]
	<project-file> [<tree-file>...]`,
	Short: "add phylogenetic trees to a PhyGeo project",
	Long: `
Command add read one or more trees from one or more tree files, and add the
trees to a PhyGeo project. The trees must be time calibrated trees.

The first argument of the command is the name of the project file. If no
project file exists, a new project will be created.

One or more tree files can be given as arguments. If no file is given the
tress will be read from the standard input.

By default, the input is expected to be in the form of tab-delimited tree
files. To import newick trees (i.e., trees in parenthetical format), use the
flag --newick with a name to be defined for the trees found in the input
files. It is expected that branch lengths were given in million years. By
default, the age of the root will be calculated from the largest branch length
between any terminal and the root. To set a different root age, use the
flag --age, with a value in million years.

By default the trees will be stored in the tree file currently defined for the
project. If the project does not have a tree file, a new one will be created
with the name 'trees.tab'. A different tree file name can be defined using the
flag --file, or -f. If this flag is used, and there is tree file already
defined, then a new file with that name will be created, and used as the tree
file for the project (previously defined trees will be kept).
	`,
	SetFlags: setFlags,
	Run:      run,
}

var treeFile string
var newickName string
var rootAge float64

func setFlags(c *command.Command) {
	c.Flags().StringVar(&treeFile, "file", "", "")
	c.Flags().StringVar(&treeFile, "f", "", "")
	c.Flags().StringVar(&newickName, "newick", "", "")
	c.Flags().Float64Var(&rootAge, "age", 0, "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}
	pFile := args[0]
	p, err := openProject(pFile)
	if err != nil {
		return err
	}

	var tc *timetree.Collection
	if tf := p.Path(project.Trees); tf != "" {
		tc, err = readTreeFile(nil, tf)
		if err != nil {
			return fmt.Errorf("on project %q: %v", tf, err)
		}
	}
	if tc == nil {
		tc = timetree.NewCollection()
	}

	args = args[1:]
	if len(args) == 0 {
		args = append(args, "-")
	}
	for i, a := range args {
		fn := a
		if fn == "-" {
			fn = ""
			a = "stdin"
		}
		var nc *timetree.Collection
		if newickName != "" {
			tn := newickName
			if i > 0 {
				tn = fmt.Sprintf("%s.%d", newickName, i)
			}
			nc, err = readNewick(c.Stdin(), fn, tn)
		} else {
			nc, err = readTreeFile(c.Stdin(), fn)
		}
		if err != nil {
			return err
		}

		for _, tn := range nc.Names() {
			t := nc.Tree(tn)
			if err := tc.Add(t); err != nil {
				return fmt.Errorf("when adding trees from %q: %v", a, err)
			}
		}
	}

	if treeFile == "" {
		treeFile = p.Path(project.Trees)
		if treeFile == "" {
			treeFile = "trees.tab"
		}
	}

	if err := writeTrees(tc); err != nil {
		return err
	}
	p.Add(project.Trees, treeFile)
	if err := p.Write(); err != nil {
		return err
	}

	return nil
}

func openProject(name string) (*project.Project, error) {
	p, err := project.Read(name)
	if errors.Is(err, os.ErrNotExist) {
		p := project.New()
		p.SetName(name)
		return p, nil
	}
	if err != nil {
		return nil, fmt.Errorf("unable ot open project %q: %v", name, err)
	}
	return p, nil
}

func readTreeFile(r io.Reader, name string) (*timetree.Collection, error) {
	if name != "" {
		f, err := os.Open(name)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
	} else {
		name = "stdin"
	}

	c, err := timetree.ReadTSV(r)
	if err != nil {
		return nil, fmt.Errorf("while reading file %q: %v", name, err)
	}
	return c, nil
}

func writeTrees(tc *timetree.Collection) (err error) {
	f, err := os.Create(treeFile)
	if err != nil {
		return err
	}
	defer func() {
		e := f.Close()
		if e != nil && err == nil {
			err = e
		}
	}()

	if err := tc.TSV(f); err != nil {
		return fmt.Errorf("while writing to %q: %v", treeFile, err)
	}
	return nil
}

func readNewick(r io.Reader, newickFile, treeName string) (*timetree.Collection, error) {
	if newickFile != "" {
		f, err := os.Open(newickFile)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
	} else {
		newickFile = "stdin"
	}

	c, err := timetree.Newick(r, treeName, int64(rootAge*timestage.MillionYears))
	if err != nil {
		return nil, fmt.Errorf("while reading file %q: %v", treeFile, err)
	}
	return c, nil
}
