// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package set implements a command to set node ages
// for the trees in PhyGeo project.
package set

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/timetree"
)

var Command = &command.Command{
	Usage: `set [--tozero] [-i|--input <file>]
	<project>`,
	Short: "set ages of the nodes of a tree",
	Long: `
Command set reads the trees from a PhyGeo project and a list of node ages to
update the age of the indicated nodes.

The argument of the command is the name of the project.

The ages of the nodes are read from an input file defined with the --input or
-i flag. The ages file is a TSV file without a header with the following
columns:

    -tree  the name of the tree
    -node  the ID of the node to set
    -age   the age (in million years) of the node

The node ages must be consistent with any other age already defined on the
tree. The changes are made sequentially.


As an usual operation is to set ages of all terminals to 0 (present), the flag
--tozero is provided to automate this action. Note that the flag will set all
terminals in the project.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var toZero bool
var input string

func setFlags(c *command.Command) {
	c.Flags().BoolVar(&toZero, "tozero", false, "")
	c.Flags().StringVar(&input, "input", "", "")
	c.Flags().StringVar(&input, "i", "", "")
}

var changes = false

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}

	p, err := project.Read(args[0])
	if err != nil {
		return err
	}

	tf := p.Path(project.Trees)
	if tf == "" {
		msg := fmt.Sprintf("tree file not defined in project %q", args[0])
		return c.UsageError(msg)
	}

	tc, err := readTreeFile(tf)
	if err != nil {
		return err
	}

	if toZero {
		termsToZero(tc)
	} else {
		setAges(tc)
	}

	if !changes {
		return nil
	}

	if err := writeTrees(tc, tf); err != nil {
		return err
	}
	return nil
}

func termsToZero(c *timetree.Collection) {
	for _, tn := range c.Names() {
		t := c.Tree(tn)
		for _, n := range t.Terms() {
			v, _ := t.TaxNode(n)
			t.Set(v, 0)
		}
	}
	changes = true
}

func readTreeFile(name string) (*timetree.Collection, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	c, err := timetree.ReadTSV(f)
	if err != nil {
		return nil, fmt.Errorf("while reading file %q: %v", name, err)
	}
	return c, nil
}

const millionYears = 1_000_000

func setAges(c *timetree.Collection) error {
	f, err := os.Open(input)
	if err != nil {
		return err
	}
	defer f.Close()

	tab := csv.NewReader(f)
	tab.Comma = '\t'
	tab.Comment = '#'

	fields := map[string]int{
		"tree": 0,
		"node": 1,
		"age":  2,
	}
	for {
		row, err := tab.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		ln, _ := tab.FieldPos(0)
		if err != nil {
			return fmt.Errorf("%q: on row %d: %v", input, ln, err)
		}
		if len(row) < len(fields) {
			return fmt.Errorf("%q: got %d rows, want %d", input, len(row), len(fields))
		}

		f := "tree"
		name := strings.ToLower(strings.Join(strings.Fields(row[fields[f]]), " "))
		if name == "" {
			continue
		}

		t := c.Tree(name)
		if t == nil {
			continue
		}
		f = "node"
		id, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return fmt.Errorf("%q: on row %d: field %q: %v", input, ln, f, err)
		}
		f = "age"
		ageF, err := strconv.ParseFloat(row[fields[f]], 64)
		if err != nil {
			return fmt.Errorf("%q: on row %d: field %q: %v", input, ln, f, err)
		}

		age := int64(ageF * millionYears)
		if err := t.Set(id, age); err != nil {
			return fmt.Errorf("%q: on row %d: %v", input, ln, err)
		}
		changes = true
	}
	return nil
}

func writeTrees(tc *timetree.Collection, treeFile string) (err error) {
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
