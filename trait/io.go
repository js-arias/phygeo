// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package trait

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ReadTSV reads a set of state observations for a trait
// in a set of taxa
// from a TSV file.
//
// The TSV file must contain the following fields:
//
//   - taxon, the taxonomic name of the taxon
//   - trait, the name of the observed trait state
//
// Here is an example file:
//
//	taxon	trait
//	Acer campbellii	temperate
//	Acer campbellii	tropical
//	Acer erythranthum	tropical
//	Acer platanoides	temperate
//	Acer saccharinum	temperate
func ReadTSV(r io.Reader) (*Data, error) {
	tab := csv.NewReader(r)
	tab.Comma = '\t'
	tab.Comment = '#'

	head, err := tab.Read()
	if err != nil {
		return nil, fmt.Errorf("while reading header: %v", err)
	}
	fields := make(map[string]int, len(head))
	for i, h := range head {
		h = strings.ToLower(h)
		fields[h] = i
	}
	for _, h := range []string{"taxon", "trait"} {
		if _, ok := fields[h]; !ok {
			return nil, fmt.Errorf("expecting field %q", h)
		}
	}

	d := New()
	for {
		row, err := tab.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		ln, _ := tab.FieldPos(0)
		if err != nil {
			return nil, fmt.Errorf("on row %d: %v", ln, err)
		}

		f := "taxon"
		tax := row[fields[f]]
		tax = canon(tax)
		if tax == "" {
			continue
		}

		f = "trait"
		obs := row[fields[f]]
		obs = strings.Join(strings.Fields(strings.ToLower(obs)), " ")
		if obs == "" {
			continue
		}

		d.Add(tax, obs)
	}
	return d, nil
}

// TSV writes traits as a TSV file.
func (d *Data) TSV(w io.Writer) error {
	tab := csv.NewWriter(w)
	tab.Comma = '\t'
	tab.UseCRLF = true

	// header
	header := []string{"taxon", "trait"}
	if err := tab.Write(header); err != nil {
		return fmt.Errorf("unable to write header: %v", err)
	}

	taxa := d.Taxa()
	for _, tx := range taxa {
		obs := d.Obs(tx)
		for _, s := range obs {
			row := []string{
				tx,
				s,
			}
			if err := tab.Write(row); err != nil {
				return fmt.Errorf("when writing data: %v", err)
			}
		}
	}

	tab.Flush()
	if err := tab.Error(); err != nil {
		return fmt.Errorf("when writing data: %v", err)
	}
	return nil
}
