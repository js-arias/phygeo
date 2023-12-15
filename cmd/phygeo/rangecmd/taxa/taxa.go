// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package terms implements a command to print
// the list of taxa in a PhyGeo project
// with defined distribution ranges.
package taxa

import (
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/js-arias/command"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/stat/pixprob"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/ranges"
	"github.com/js-arias/timetree"
)

var Command = &command.Command{
	Usage: "taxa [--count] [--ranges] [--val] <project-file>",
	Short: "print a list of taxa with distribution ranges",
	Long: `
Command taxa reads the geographic ranges from a PhyGeo project and print the
name of the taxa in the standard output.

The argument of the command is the name of the project file.

By default the taxa of the points (presence-absence pixels) dataset will be
printed. If the flag --ranges is set, it will print the taxa of the continuous
distribution range file.

If the flag --count is defined, the number of valid and total amount of pixels
in the range will be printed in front of each taxon name that is defined in at
least one tree. To be valid a pixel must be defined over a landscape value
with a prior probability greater than zero.

If the flag --val is defined, and all the taxa has valid records, the command
will finish silently. Otherwise, any invalid taxon (a taxon without valid
records) will be reported. To be valid, a taxon must have, at least, one
valid pixel.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var countFlag bool
var rangeFlag bool
var valFlag bool

func setFlags(c *command.Command) {
	c.Flags().BoolVar(&countFlag, "count", false, "")
	c.Flags().BoolVar(&rangeFlag, "ranges", false, "")
	c.Flags().BoolVar(&valFlag, "val", false, "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}

	p, err := project.Read(args[0])
	if err != nil {
		return err
	}

	rf := p.Path(project.Points)
	if rangeFlag {
		rf = p.Path(project.Ranges)
	}
	if rf == "" {
		return nil
	}

	coll, err := readRanges(rf)
	if err != nil {
		return err
	}
	if coll == nil {
		return nil
	}

	if valFlag || countFlag {
		lsf := p.Path(project.Landscape)
		if lsf == "" {
			msg := fmt.Sprintf("paleolandscape not defined in project %q", args[0])
			return c.UsageError(msg)
		}
		landscape, err := readLandscape(lsf)
		if err != nil {
			return err
		}

		ppF := p.Path(project.PixPrior)
		if ppF == "" {
			msg := fmt.Sprintf("pixel priors not defined in project %q", args[0])
			return c.UsageError(msg)
		}
		pp, err := readPriors(ppF)
		if err != nil {
			return err
		}

		tf := p.Path(project.Trees)
		if tf == "" {
			msg := fmt.Sprintf("tree file not defined in project %q", args[0])
			return c.UsageError(msg)
		}

		ls, err := makeTermList(tf)
		if err != nil {
			return nil
		}

		valCount(c.Stdout(), ls, coll, landscape, pp)
		return nil
	}

	ls := coll.Taxa()
	for _, tax := range ls {
		fmt.Fprintf(c.Stdout(), "%s\n", tax)
	}

	return nil
}

func readRanges(name string) (*ranges.Collection, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	coll, err := ranges.ReadTSV(f, nil)
	if err != nil {
		return nil, fmt.Errorf("when reading %q: %v", name, err)
	}

	return coll, nil
}

func readLandscape(name string) (*model.TimePix, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tp, err := model.ReadTimePix(f, nil)
	if err != nil {
		return nil, fmt.Errorf("on file %q: %v", name, err)
	}

	return tp, nil
}

func readPriors(name string) (pixprob.Pixel, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	pp, err := pixprob.ReadTSV(f)
	if err != nil {
		return nil, fmt.Errorf("when reading %q: %v", name, err)
	}

	return pp, nil
}

func makeTermList(name string) ([]string, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	c, err := timetree.ReadTSV(f)
	if err != nil {
		return nil, fmt.Errorf("while reading file %q: %v", name, err)
	}

	ls := c.Names()
	terms := make(map[string]bool)

	for _, tn := range ls {
		t := c.Tree(tn)
		if t == nil {
			continue
		}
		for _, tax := range t.Terms() {
			terms[tax] = true
		}
	}

	termList := make([]string, 0, len(terms))
	for tax := range terms {
		termList = append(termList, tax)
	}
	slices.Sort(termList)

	return termList, nil
}

func valCount(w io.Writer, ls []string, coll *ranges.Collection, tp *model.TimePix, pp pixprob.Pixel) {
	for _, tax := range ls {
		if !coll.HasTaxon(tax) {
			if valFlag {
				fmt.Fprintf(w, "INVALID TAXON: no records: %s\n", tax)
				continue
			}
			fmt.Fprintf(w, "%s\t%d\t%d\n", tax, 0, 0)
			continue
		}

		rng := coll.Range(tax)
		age := tp.ClosestStageAge(coll.Age(tax))
		lsc := tp.Stage(age)
		val := 0
		for px := range rng {
			v := lsc[px]
			prior := pp.Prior(v)
			if prior > 0 {
				val++
			}
		}

		if valFlag {
			if val == 0 {
				fmt.Fprintf(w, "INVALID TAXON: no valid pixels: %s: %d pixels\n", tax, len(rng))
			}
			continue
		}

		fmt.Fprintf(w, "%s\t%d\t%d\n", tax, val, len(rng))
	}
}
