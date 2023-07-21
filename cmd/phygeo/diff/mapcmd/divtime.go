// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package mapcmd

import (
	"encoding/csv"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/stat/dist"
	"github.com/js-arias/earth/stat/pixprob"
	"github.com/js-arias/timetree"
)

func diversificationOnTime(name, tf string, tot *model.Total, landscape *model.TimePix, keys *pixKey, norm dist.Normal, pp pixprob.Pixel, contour image.Image) error {
	tc, err := readTreeFile(tf)
	if err != nil {
		return err
	}

	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()

	splits, err := readSplits(f, landscape, tc)
	if err != nil {
		return fmt.Errorf("on input file %q: %v", name, err)
	}

	sc := make(chan stageChan, numCPU*2)
	for i := 0; i < numCPU; i++ {
		go procStage(sc)
	}

	errChan := make(chan error)
	doneChan := make(chan struct{})
	var wg sync.WaitGroup

	stages := landscape.Stages()
	var ws bool
	for i := len(stages) - 1; i >= 0; i-- {
		s, ok := splits[stages[i]]
		if ok {
			ws = true
		}
		if !ws {
			continue
		}

		if !ok {
			s = &recStage{
				age:       stages[i],
				cAge:      stages[i],
				rec:       make(map[int]float64),
				landscape: landscape,
			}
		}

		// the age is in million years
		age := float64(s.age) / 1_000_000
		suf := fmt.Sprintf("-divtime-%.3f", age)
		s.step = 360 / float64(colsFlag)
		s.keys = keys
		s.contour = contour
		wg.Add(1)
		sc <- stageChan{
			rs:        s,
			out:       outputPre + suf + ".png",
			err:       errChan,
			wg:        &wg,
			norm:      norm,
			pp:        pp,
			landscape: landscape,
			tot:       tot,
		}
	}

	go func() {
		wg.Wait()
		close(doneChan)
	}()

	select {
	case err := <-errChan:
		return err
	case <-doneChan:
	}

	return nil
}

func readSplits(r io.Reader, landscape *model.TimePix, tc *timetree.Collection) (map[int64]*recStage, error) {
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

	splits := make(map[int64]*recStage)
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
		t := tc.Tree(tn)
		if t == nil {
			continue
		}

		f = "node"
		id, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}

		if t.IsTerm(id) {
			continue
		}

		f = "age"
		age, err := strconv.ParseInt(row[fields[f]], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}

		if t.Age(id) != age {
			continue
		}
		age = landscape.ClosestStageAge(age)

		if l := landscape.Stage(age); l == nil {
			continue
		}

		st, ok := splits[age]
		if !ok {
			st = &recStage{
				age:       age,
				cAge:      age,
				rec:       make(map[int]float64),
				landscape: landscape,
			}
			splits[age] = st
		}

		f = "to"
		px, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if px >= landscape.Pixelation().Len() {
			return nil, fmt.Errorf("on row %d: field %q: invalid pixel value %d", ln, f, px)
		}

		st.rec[px]++
		if v := st.rec[px]; v > st.max {
			st.max = v
		}
	}

	if len(splits) == 0 {
		return nil, fmt.Errorf("while reading data: %v", io.EOF)
	}

	return splits, nil
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
