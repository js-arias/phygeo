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
	"github.com/js-arias/timetree"
)

var Command = &command.Command{
	Usage: "add [-f|--file <tree-file>] <project-file> [<tree-file>...]",
	Short: "add phylogenetic trees to a PhyGeo project",
	Long: `
Command add read one or more trees from one or more tree files, and add the
trees to a PhyGeo project. The trees must be time calibrated trees.

The first argument of the command is the name of the project file. If no
project file exists, a new project will be created.

One or more tree files can be given as arguments. If no file is given the
tress will be read from the standard input.

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

func setFlags(c *command.Command) {
	c.Flags().StringVar(&treeFile, "file", "", "")
	c.Flags().StringVar(&treeFile, "f", "", "")
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
	for _, a := range args {
		fn := a
		if fn == "-" {
			fn = ""
			a = "stdin"
		}
		nc, err := readTreeFile(c.Stdin(), fn)
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
	if err := p.Write(pFile); err != nil {
		return err
	}

	return nil
}

func openProject(name string) (*project.Project, error) {
	p, err := project.Read(name)
	if errors.Is(err, os.ErrNotExist) {
		return project.New(), nil
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
