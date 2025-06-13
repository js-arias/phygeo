// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package pixels implements a command to print
// the list of pixels of a taxon range in a PhyGeo project.
package pixels

import (
	"encoding/csv"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/phygeo/timestage"
)

var Command = &command.Command{
	Usage: `pixels --taxon <string>
	[-m|--model <model-type>]
	<project-file>`,
	Short: "print the pixels of a taxon",
	Long: `
Command pixels reads the geographic range of a particular taxon in a PhyGeo
project and print the pixels of that taxon in the standard output.

The argument of the command is the name of the project file.

The flag --taxon is required and indicates the name of the taxon to be
examined.

By default, it will assume that the project is for a random walk. Valid values
are:

	walk	a random walk (the default)
	diff	a diffusion model
	`,
	SetFlags: setFlags,
	Run:      run,
}

var modelFlag string
var taxonName string

func setFlags(c *command.Command) {
	c.Flags().StringVar(&modelFlag, "model", "walk", "")
	c.Flags().StringVar(&modelFlag, "m", "walk", "")
	c.Flags().StringVar(&taxonName, "taxon", "", "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}

	if taxonName == "" {
		return c.UsageError("expecting flag --taxon")
	}

	p, err := project.Read(args[0])
	if err != nil {
		return err
	}

	rf := p.Path(project.Ranges)
	if rf == "" {
		return fmt.Errorf("taxon %q not in project %q", taxonName, p.Name())
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
		return fmt.Errorf("taxon %q not in project %q", taxonName, p.Name())
	}

	if !coll.HasTaxon(taxonName) {
		return fmt.Errorf("taxon %q not in project %q", taxonName, p.Name())
	}

	w := csv.NewWriter(c.Stdout())
	w.Comma = '\t'
	w.UseCRLF = true

	rng := coll.Range(taxonName)
	age := landscape.ClosestStageAge(coll.Age(taxonName))
	lsc := landscape.Stage(age)
	ageTax := strconv.FormatFloat(float64(coll.Age(taxonName))/timestage.MillionYears, 'f', 6, 64)

	modelFlag = strings.ToLower(modelFlag)
	switch modelFlag {
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

		w.Write([]string{"age", "trait", "pixel", "lon", "lat", "label", "weight", "prob"})

		obs := tr.Obs(taxonName)
		pxs := make([]int, 0, len(rng))
		for px := range rng {
			pxs = append(pxs, px)
		}
		slices.Sort(pxs)
		for _, t := range obs {
			for _, px := range pxs {
				p := rng[px]
				v := lsc[px]
				lb := keys.Label(v)
				weight := st.Weight(t, lb)
				pt := landscape.Pixelation().ID(px).Point()
				row := []string{
					ageTax,
					t,
					strconv.Itoa(px),
					strconv.FormatFloat(pt.Longitude(), 'f', 6, 64),
					strconv.FormatFloat(pt.Latitude(), 'f', 6, 64),
					lb,
					strconv.FormatFloat(weight, 'f', 6, 64),
					strconv.FormatFloat(p, 'f', 6, 64),
				}
				w.Write(row)
			}
		}
		w.Flush()
	case "diff":
		pw, err := p.PixWeight()
		if err != nil {
			return err
		}

		w.Write([]string{"age", "pixel", "lon", "lat", "label", "weight", "prob"})

		pxs := make([]int, 0, len(rng))
		for px := range rng {
			pxs = append(pxs, px)
		}
		slices.Sort(pxs)
		for _, px := range pxs {
			p := rng[px]
			v := lsc[px]
			weight := pw.Weight(v)
			pt := landscape.Pixelation().ID(px).Point()
			row := []string{
				ageTax,
				strconv.Itoa(px),
				strconv.FormatFloat(pt.Longitude(), 'f', 6, 64),
				strconv.FormatFloat(pt.Latitude(), 'f', 6, 64),
				strconv.Itoa(v),
				strconv.FormatFloat(weight, 'f', 6, 64),
				strconv.FormatFloat(p, 'f', 6, 64),
			}
			w.Write(row)
		}
	default:
		return fmt.Errorf("unknown model %q", modelFlag)
	}
	return nil
}
