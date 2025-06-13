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
	"slices"
	"strings"

	"github.com/js-arias/command"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/pixkey"
	"github.com/js-arias/earth/stat/pixweight"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/phygeo/trait"
	"github.com/js-arias/ranges"
)

var Command = &command.Command{
	Usage: `taxa [-m|--model <model-type>]
	[--count] [--val]
	<project-file>`,
	Short: "print a list of taxa with distribution ranges",
	Long: `
Command taxa reads the geographic ranges from a PhyGeo project and print the
name of the taxa in the standard output.

The argument of the command is the name of the project file.

If the flag --count is defined, the number of valid (when a walk model is used
the number of pixels with low settlement weights will also be in brackets),
the total amount of pixels, and the type of the range will be printed in front
of each taxon name that is defined in at least one tree. To be valid a pixel
must be defined over a landscape value with a weight greater than zero. A low
weight settlement is a settlement weight below 0.5.

If the flag --val is defined, and all the taxa has valid records, the command
will finish silently. Otherwise, any invalid taxon (a taxon without valid
records) will be reported. To be valid, a taxon must have, at least, one
valid pixel (i.e. a pixel with a weight greater than zero).

By default, it will assume that the project is for a random walk. The model
only makes difference if --count or -val flags are used. Use the flag --model,
or -m, to define a different model. Valid values are:

	walk	a random walk (the default)
	diff	a diffusion model
	`,
	SetFlags: setFlags,
	Run:      run,
}

var countFlag bool
var rangeFlag bool
var valFlag bool
var modelFlag string

func setFlags(c *command.Command) {
	c.Flags().BoolVar(&countFlag, "count", false, "")
	c.Flags().BoolVar(&rangeFlag, "ranges", false, "")
	c.Flags().BoolVar(&valFlag, "val", false, "")
	c.Flags().StringVar(&modelFlag, "model", "walk", "")
	c.Flags().StringVar(&modelFlag, "m", "walk", "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}

	p, err := project.Read(args[0])
	if err != nil {
		return err
	}

	rf := p.Path(project.Ranges)
	if rf == "" {
		if !valFlag {
			return nil
		}

		tf := p.Path(project.Trees)
		if tf == "" {
			return nil
		}

		// all terminal taxa are "invalid":
		// no distribution data defined.
		ls, err := makeTermList(p)
		if err != nil {
			return nil
		}
		for _, tax := range ls {
			fmt.Fprintf(c.Stdout(), "INVALID TAXON: no records: %s\n", tax)
		}
		return nil
	}

	landscape, err := p.Landscape(nil)
	if err != nil {
		return err
	}

	coll, err := p.Ranges(landscape.Pixelation())
	if err != nil {
		return err
	}
	if coll == nil {
		return nil
	}

	if valFlag || countFlag {
		ls, err := makeTermList(p)
		if err != nil {
			return nil
		}

		modelFlag = strings.ToLower(modelFlag)
		switch modelFlag {
		case "diff":
			pw, err := p.PixWeight()
			if err != nil {
				return err
			}

			valCount(c.Stdout(), ls, coll, landscape, pw)
		case "walk":
			keys, err := p.Keys()
			if err != nil {
				return err
			}

			tr, err := p.Traits()
			if err != nil {
				return err
			}

			st, err := p.Settlement(tr, keys)
			if err != nil {
				return err
			}

			valCountWalk(c.Stdout(), ls, coll, landscape, tr, keys, st)
		default:
			return fmt.Errorf("unknown model %q", modelFlag)
		}
		return nil
	}

	ls := coll.Taxa()
	for _, tax := range ls {
		fmt.Fprintf(c.Stdout(), "%s\n", tax)
	}

	return nil
}

func makeTermList(p *project.Project) ([]string, error) {
	c, err := p.Trees()
	if err != nil {
		return nil, err
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

func valCount(w io.Writer, ls []string, coll *ranges.Collection, tp *model.TimePix, pw pixweight.Pixel) {
	for _, tax := range ls {
		if !coll.HasTaxon(tax) {
			if valFlag {
				fmt.Fprintf(w, "INVALID TAXON: no records: %s\n", tax)
				continue
			}
			fmt.Fprintf(w, "%s\t%d\t%d\tNA\n", tax, 0, 0)
			continue
		}

		rng := coll.Range(tax)
		age := tp.ClosestStageAge(coll.Age(tax))
		lsc := tp.Stage(age)
		val := 0
		for px := range rng {
			v := lsc[px]
			weight := pw.Weight(v)
			if weight > 0 {
				val++
			}
		}

		if valFlag {
			if val == 0 {
				fmt.Fprintf(w, "INVALID TAXON: no valid pixels: %s: %d pixels\n", tax, len(rng))
			}
			continue
		}

		fmt.Fprintf(w, "%s\t%d\t%d\t%s\n", tax, val, len(rng), coll.Type(tax))
	}
}

func valCountWalk(w io.Writer, ls []string, coll *ranges.Collection, tp *model.TimePix, tr *trait.Data, key *pixkey.PixKey, sett *trait.Matrix) {
	for _, tax := range ls {
		if !coll.HasTaxon(tax) {
			if valFlag {
				fmt.Fprintf(w, "INVALID TAXON: no records: %s\n", tax)
				continue
			}
			fmt.Fprintf(w, "%s\t%d [%d]\t%d\tNA\n", tax, 0, 0, 0)
			continue
		}

		rng := coll.Range(tax)
		age := tp.ClosestStageAge(coll.Age(tax))
		lsc := tp.Stage(age)
		obs := tr.Obs(tax)
		val := 0
		high := 0
		for px := range rng {
			v := lsc[px]
			var isVal, isHigh bool
			for _, t := range obs {
				w := sett.Weight(t, key.Label(v))
				if w > 0 {
					isVal = true
				}
				if w > 0.5 {
					isHigh = true
				}
			}
			if isVal {
				val++
			}
			if isHigh {
				high++
			}
		}

		if valFlag {
			if val == 0 {
				fmt.Fprintf(w, "INVALID TAXON: no valid pixels: %s: %d pixels\n", tax, len(rng))
				continue
			}
			if high == 0 {
				fmt.Fprintf(w, "WARNING: low probability pixels: %s: %d of %d pixels\n", tax, val, len(rng))
			}
			continue
		}

		fmt.Fprintf(w, "%s\t%d [%d]\t%d\t%s\n", tax, val, high, len(rng), coll.Type(tax))
	}
}
