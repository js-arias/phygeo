// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package add implements a command to add taxon traits
// to a PhyGeo project.
package add

import (
	"fmt"
	"io"
	"os"

	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/phygeo/trait"
)

var Command = &command.Command{
	Usage: `add [-f|--file <trait-file>] [--filter]
	<project-file> [<trait-file>...]`,
	Short: "add taxon traits to a PhyGeo project",
	Long: `
Command add reads one or more tab-delimited files with trait data, and add the
trait observation to a PhyGeo project.

The first argument of the command is the name of the project file.

One or more trait files can be given as arguments. If no file is given the
traits will be read from the standard input.

By default, all taxon-trait pairs will be added. If the flag --filter is
defined and there are trees in the project, then it will add only the traits
for the taxon names present in the trees.

By default the trait file will be stored in the trait file currently defined
for the project. If the project does not have a trait file, a new one will be
created with the name 'traits.tab'. A different file name can be defined with
the flag --file or -f. If this flag is used, and there is a trait file already
defined, then the new file will be created, and used as traits file
(previously defined traits will be kept).
	`,
	SetFlags: setFlags,
	Run:      run,
}

var outFile string
var filterFlag bool

func setFlags(c *command.Command) {
	c.Flags().StringVar(&outFile, "file", "", "")
	c.Flags().StringVar(&outFile, "f", "", "")
	c.Flags().BoolVar(&filterFlag, "filter", false, "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}
	p, err := project.Read(args[0])
	if err != nil {
		return err
	}

	d, err := addTraitData(c.Stdin(), p, args[1:])
	if err != nil {
		return err
	}

	tf := p.Path(project.Traits)
	if tf == "" {
		tf = "traits.tab"
	}
	if outFile != "" {
		tf = outFile
	}
	if err := writeTraitData(tf, d); err != nil {
		return err
	}

	if p.Path(project.Traits) != tf {
		p.Add(project.Traits, tf)
		if err := p.Write(); err != nil {
			return err
		}
	}
	return nil
}

func addTraitData(r io.Reader, p *project.Project, files []string) (*trait.Data, error) {
	d := trait.New()

	tf := p.Path(project.Traits)
	if tf != "" {
		var err error
		d, err = p.Traits()
		if err != nil {
			return nil, err
		}
	}

	var filter map[string]bool
	if filterFlag {
		var err error
		filter, err = makeFilter(p)
		if err != nil {
			return nil, err
		}
	}

	if len(files) == 0 {
		files = append(files, "-")
	}
	for _, f := range files {
		td, err := readTraitData(r, f)
		if err != nil {
			return nil, err
		}

		for _, tx := range td.Taxa() {
			if filterFlag {
				if !filter[tx] {
					continue
				}
			}
			obs := td.Obs(tx)
			for _, s := range obs {
				d.Add(tx, s)
			}
		}
	}
	return d, nil
}

func readTraitData(r io.Reader, name string) (*trait.Data, error) {
	if name != "-" {
		f, err := os.Open(name)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
	} else {
		name = "stdin"
	}

	d, err := trait.ReadTSV(r)
	if err != nil {
		return nil, fmt.Errorf("when reading %q: %v", name, err)
	}
	return d, nil
}

func writeTraitData(name string, d *trait.Data) (err error) {
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	defer func() {
		e := f.Close()
		if e != nil && err == nil {
			err = e
		}
	}()

	if err := d.TSV(f); err != nil {
		return fmt.Errorf("while writing %q: %v", name, err)
	}
	return nil
}

func makeFilter(p *project.Project) (map[string]bool, error) {
	c, err := p.Trees()
	if err != nil {
		return nil, err
	}

	terms := make(map[string]bool)
	for _, tn := range c.Names() {
		t := c.Tree(tn)
		if t == nil {
			continue
		}
		for _, tax := range t.Terms() {
			terms[tax] = true
		}
	}

	return terms, nil
}
