// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package mapcmd

import (
	"encoding/csv"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/js-arias/blind"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/timetree"
)

func diversificationAges(name, tf, out string, tot *model.Total, landscape *model.TimePix, keys *pixKey, contour image.Image) error {
	tc, err := readTreeFile(tf)
	if err != nil {
		return err
	}

	splitAges, err := openSplitAges(name, tot.Inverse(), tc)
	if err != nil {
		return err
	}
	var max int64
	for _, a := range splitAges {
		if a > max {
			max = a
		}
	}

	sa := &splitAgeMap{
		splitAges: splitAges,
		max:       max,
		landscape: landscape,
		step:      360 / float64(colsFlag),
		keys:      keys,
		contour:   contour,
	}

	if err := writeImage(out, sa); err != nil {
		return err
	}
	return nil
}

type splitAgeMap struct {
	splitAges map[int]int64
	max       int64
	landscape *model.TimePix
	step      float64
	keys      *pixKey

	contour image.Image
}

func (sa *splitAgeMap) ColorModel() color.Model { return color.RGBAModel }
func (sa *splitAgeMap) Bounds() image.Rectangle { return image.Rect(0, 0, colsFlag, colsFlag/2) }
func (sa *splitAgeMap) At(x, y int) color.Color {
	if sa.contour != nil {
		_, _, _, a := sa.contour.At(x, y).RGBA()
		if a > 100 {
			return color.RGBA{A: 255}
		}
	}

	lat := 90 - float64(y)*sa.step
	lon := float64(x)*sa.step - 180

	pix := sa.landscape.Pixelation().Pixel(lat, lon)
	if age, ok := sa.splitAges[pix.ID()]; ok {
		if divAgeFlag == "ancient" {
			return blind.Gradient(float64(age) / float64(sa.max))
		}
		return blind.Gradient(float64(sa.max-age) / float64(sa.max))
	}

	if sa.keys == nil {
		return color.RGBA{211, 211, 211, 255}
	}

	// use current age
	v, _ := sa.landscape.At(0, pix.ID())
	if grayFlag {
		if c, ok := sa.keys.Gray(v); ok {
			return c
		}
	} else {
		if c, ok := sa.keys.Color(v); ok {
			return c
		}
	}

	return color.RGBA{211, 211, 211, 255}
}

func openSplitAges(name string, tot *model.Total, tc *timetree.Collection) (map[int]int64, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	splitAges, err := readSplitAges(f, tot, tc)
	if err != nil {
		return nil, fmt.Errorf("on input file %q: %v", name, err)
	}
	return splitAges, nil
}

func readSplitAges(r io.Reader, tot *model.Total, tc *timetree.Collection) (map[int]int64, error) {
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

	ageSplits := make(map[int]int64)
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
		if divAgeFlag == "ancient" || divAgeFlag == "recent" {
			if t.IsTerm(id) {
				continue
			}
		}

		f = "age"
		age, err := strconv.ParseInt(row[fields[f]], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}

		if divAgeFlag == "ancient" || divAgeFlag == "recent" {
			if t.Age(id) != age {
				continue
			}
		}

		f = "to"
		px, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if px >= tot.Pixelation().Len() {
			return nil, fmt.Errorf("on row %d: field %q: invalid pixel value %d", ln, f, px)
		}

		if divAgeFlag == "ancient" || divAgeFlag == "ancient-all" {
			rot := tot.Rotation(tot.ClosestStageAge(age))
			if rot != nil {
				dst := rot[px]
				for _, rp := range dst {
					max := ageSplits[rp]
					if age > max {
						ageSplits[rp] = age
					}
				}
				continue
			}
			max := ageSplits[px]
			if age > max {
				ageSplits[px] = age
			}
			continue
		}

		rot := tot.Rotation(tot.ClosestStageAge(age))
		if rot != nil {
			dst := rot[px]
			for _, rp := range dst {
				min, ok := ageSplits[rp]
				if !ok {
					ageSplits[rp] = age
					continue
				}
				if age < min {
					ageSplits[rp] = age
				}
			}
			continue
		}
		min, ok := ageSplits[px]
		if !ok {
			ageSplits[px] = age
			continue
		}
		if age < min {
			ageSplits[px] = age
		}
	}

	if len(ageSplits) == 0 {
		return nil, fmt.Errorf("while reading data: %v", io.EOF)
	}
	return ageSplits, nil
}
