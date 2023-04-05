// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package speed

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/timetree"
	"golang.org/x/exp/slices"
)

func getTimeSlice(name string, tc *timetree.Collection, tp *model.TimePix) (map[string]*treeSlice, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ts, err := readTimeSlices(f, tc, tp)
	if err != nil {
		return nil, fmt.Errorf("on input file %q: %v", name, err)
	}
	return ts, nil
}

type treeSlice struct {
	name      string
	particles map[int]*partSlice
}

type partSlice struct {
	id int
	ts map[int64]*recSlice
}

type recSlice struct {
	age      int64
	sumSpeed float64
	lineages int
}

func readTimeSlices(r io.Reader, tc *timetree.Collection, tp *model.TimePix) (map[string]*treeSlice, error) {
	tsv := csv.NewReader(r)
	tsv.Comma = '\t'
	tsv.Comment = '#'

	head, err := tsv.Read()
	if err != nil {
		return nil, fmt.Errorf("while reading header: %v", err)
	}
	fields := make(map[string]int, len(head))
	for i, h := range head {
		h = strings.ToLower(h)
		fields[h] = i
	}
	for _, h := range headerFields {
		if _, ok := fields[h]; !ok {
			return nil, fmt.Errorf("expecting field %q", h)
		}
	}

	ts := make(map[string]*treeSlice)
	for {
		row, err := tsv.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		ln, _ := tsv.FieldPos(0)
		if err != nil {
			return nil, fmt.Errorf("on row %d: %v", ln, err)
		}

		f := "tree"
		tn := strings.Join(strings.Fields(row[fields[f]]), " ")
		if tn == "" {
			continue
		}
		tn = strings.ToLower(tn)
		tv := tc.Tree(tn)
		if tv == nil {
			continue
		}
		t, ok := ts[tn]
		if !ok {
			t = &treeSlice{
				name:      tn,
				particles: make(map[int]*partSlice),
			}
			ts[tn] = t
		}

		f = "node"
		id, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}

		// ignore root node
		if tv.IsRoot(id) {
			continue
		}

		f = "particle"
		pN, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		p, ok := t.particles[pN]
		if !ok {
			p = &partSlice{
				id: pN,
				ts: make(map[int64]*recSlice),
			}
			t.particles[pN] = p
		}

		f = "age"
		age, err := strconv.ParseInt(row[fields[f]], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		old, young := tp.Bounds(age)
		if na := tv.Age(tv.Parent(id)); na < old {
			old = na
		}

		rs, ok := p.ts[young]
		if !ok {
			rs = &recSlice{
				age: young,
			}
			p.ts[young] = rs
		}

		f = "from"
		fPx, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if fPx >= tp.Pixelation().Len() {
			return nil, fmt.Errorf("on row %d: field %q: invalid pixel value %d", ln, f, fPx)
		}
		from := tp.Pixelation().ID(fPx).Point()

		f = "to"
		tPx, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if tPx >= tp.Pixelation().Len() {
			return nil, fmt.Errorf("on row %d: field %q: invalid pixel value %d", ln, f, tPx)
		}
		to := tp.Pixelation().ID(tPx).Point()

		dist := earth.Distance(from, to)
		brLen := float64(old-age) / millionYears
		rs.sumSpeed += dist / brLen
		rs.lineages++
	}
	if len(ts) == 0 {
		return nil, fmt.Errorf("while reading data: %v", io.EOF)
	}
	return ts, nil
}

func timeSliceFile(w io.Writer, ts map[string]*treeSlice) error {
	if outputFile != "" {
		f, err := os.Create(outputFile)
		if err != nil {
			return err
		}
		defer func() {
			e := f.Close()
			if e != nil && err == nil {
				err = e
			}
		}()
		w = f
	} else {
		outputFile = "stdout"
	}

	if err := writeTimeSlice(w, ts); err != nil {
		return fmt.Errorf("when writing on file %q: %v", outputFile, err)
	}
	return nil
}

func writeTimeSlice(w io.Writer, ts map[string]*treeSlice) error {
	tab := csv.NewWriter(w)
	tab.Comma = '\t'
	tab.UseCRLF = true

	if err := tab.Write([]string{"tree", "particle", "age", "avg-speed", "lineages"}); err != nil {
		return err
	}

	names := make([]string, 0, len(ts))
	for name := range ts {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
		t := ts[name]

		ps := make([]int, 0, len(t.particles))
		for p := range t.particles {
			ps = append(ps, p)
		}
		slices.Sort(ps)

		for _, pID := range ps {
			p := t.particles[pID]
			ages := make([]int64, 0, len(p.ts))
			for a := range p.ts {
				ages = append(ages, a)
			}
			slices.Sort(ages)

			for _, a := range ages {
				rs := p.ts[a]
				speed := rs.sumSpeed / float64(rs.lineages)

				row := []string{
					name,
					strconv.Itoa(pID),
					strconv.FormatInt(a, 10),
					strconv.FormatFloat(speed, 'f', 6, 64),
					strconv.Itoa(rs.lineages),
				}
				if err := tab.Write(row); err != nil {
					return err
				}
			}
		}
	}

	tab.Flush()
	if err := tab.Error(); err != nil {
		return err
	}
	return nil
}
