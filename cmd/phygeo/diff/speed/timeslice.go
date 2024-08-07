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
	"github.com/js-arias/phygeo/timestage"
	"github.com/js-arias/timetree"
	"golang.org/x/exp/slices"
	"gonum.org/v1/gonum/stat"
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
	name       string
	timeSlices map[int64]*recSlice
}

type recSlice struct {
	age       int64
	sumBrLen  float64
	distances map[int]float64
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
				name:       tn,
				timeSlices: make(map[int64]*recSlice),
			}
			t.addSlices(tv, tp, tv.Root())
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

		f = "age"
		age, err := strconv.ParseInt(row[fields[f]], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		age = tp.ClosestStageAge(age)
		rs := t.timeSlices[age]

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
		rs.distances[pN] += dist
	}
	if len(ts) == 0 {
		return nil, fmt.Errorf("while reading data: %v", io.EOF)
	}
	return ts, nil
}

func (s *treeSlice) addSlices(t *timetree.Tree, tp *model.TimePix, n int) {
	children := t.Children(n)
	for _, c := range children {
		s.addSlices(t, tp, c)
	}

	if t.IsRoot(n) {
		return
	}

	nAge := t.Age(n)
	prev := t.Age(t.Parent(n))

	// add time stages
	for a := tp.ClosestStageAge(prev - 1); a > nAge; a = tp.ClosestStageAge(a - 1) {
		ts, ok := s.timeSlices[a]
		if !ok {
			ts = &recSlice{
				age:       a,
				distances: make(map[int]float64),
			}
			s.timeSlices[a] = ts
		}
		ts.sumBrLen += float64(prev-a) / timestage.MillionYears
		prev = a
	}

	// add the last segment
	age := tp.ClosestStageAge(nAge)
	ts, ok := s.timeSlices[age]
	if !ok {
		ts = &recSlice{
			age:       age,
			distances: make(map[int]float64),
		}
		s.timeSlices[age] = ts
	}
	ts.sumBrLen += float64(prev-nAge) / timestage.MillionYears
}

func writeTimeSlice(w io.Writer, ts map[string]*treeSlice) error {
	tab := csv.NewWriter(w)
	tab.Comma = '\t'
	tab.UseCRLF = true

	if err := tab.Write([]string{"tree", "age", "distance", "d-025", "d-975", "brLen", "speed"}); err != nil {
		return err
	}

	names := make([]string, 0, len(ts))
	for name := range ts {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
		t := ts[name]
		ages := make([]int64, 0, len(t.timeSlices))
		for a := range t.timeSlices {
			ages = append(ages, a)
		}
		slices.Sort(ages)

		for _, a := range ages {
			s := t.timeSlices[a]

			dist := make([]float64, 0, len(s.distances))
			weights := make([]float64, 0, len(s.distances))
			for _, d := range s.distances {
				dist = append(dist, d*earth.Radius/1000)
				weights = append(weights, 1.0)
			}
			slices.Sort(dist)

			d := stat.Quantile(0.5, stat.Empirical, dist, weights)
			sp := d / s.sumBrLen

			row := []string{
				name,
				strconv.FormatInt(a, 10),
				strconv.FormatFloat(d, 'f', 3, 64),
				strconv.FormatFloat(stat.Quantile(0.025, stat.Empirical, dist, weights), 'f', 3, 64),
				strconv.FormatFloat(stat.Quantile(0.975, stat.Empirical, dist, weights), 'f', 3, 64),
				strconv.FormatFloat(s.sumBrLen, 'f', 3, 64),
				strconv.FormatFloat(sp, 'f', 3, 64),
			}
			if err := tab.Write(row); err != nil {
				return err
			}

		}
	}

	tab.Flush()
	if err := tab.Error(); err != nil {
		return err
	}
	return nil
}
