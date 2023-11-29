// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package pixkey implements a simple color key
// for landscape pixelations.
package pixkey

import (
	"encoding/csv"
	"errors"
	"fmt"
	"image/color"
	"io"
	"os"
	"strconv"
	"strings"
)

// PixKey stores the color values
// for a pixel value.
type PixKey struct {
	color map[int]color.Color
	gray  map[int]uint8
}

// Color returns the color associated with a given value.
// If no color is defined for the value,
// it will return transparent black.
func (pk *PixKey) Color(v int) (color.Color, bool) {
	c, ok := pk.color[v]
	if !ok {
		return color.RGBA{0, 0, 0, 0}, false
	}
	return c, true
}

// HasGrayScale returns true if a gray scale is defined
// for the keys.
func (pk *PixKey) HasGrayScale() bool {
	return len(pk.gray) > 0
}

// Gray returns the gray color associated with a given value.
// If no color is defined for the value,
// it will return transparent black.
func (pk *PixKey) Gray(v int) (color.Color, bool) {
	g, ok := pk.gray[v]
	if !ok {
		return color.RGBA{0, 0, 0, 0}, false
	}
	return color.RGBA{g, g, g, 255}, true
}

// Read reads a key file used to define the colors
// for pixel values in a time pixelation.
//
// A key file is a tab-delimited file
// with the following required columns:
//
//	-key	the value used as identifier
//	-color	an RGB value separated by commas,
//		for example "125,132,148".
//
// Optionally it can contain the following columns:
//
//	-gray:  for a gray scale value
//
// Any other columns, will be ignored.
// Here is an example of a key file:
//
//	key	color	gray	comment
//	0	0, 26, 51	0	deep ocean
//	1	0, 84, 119	10	oceanic plateaus
//	2	68, 167, 196	20	continental shelf
//	3	251, 236, 93	90	lowlands
//	4	255, 165, 0	100	highlands
//	5	229, 229, 224	50	ice sheets
func Read(name string) (*PixKey, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.Comma = '\t'
	r.Comment = '#'

	head, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("while reading header: %v", err)
	}
	fields := make(map[string]int, len(head))
	for i, h := range head {
		h = strings.ToLower(h)
		fields[h] = i
	}
	for _, h := range []string{"key", "color"} {
		if _, ok := fields[h]; !ok {
			return nil, fmt.Errorf("expecting field %q", h)
		}
	}

	pk := &PixKey{
		color: make(map[int]color.Color),
		gray:  make(map[int]uint8),
	}

	for {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		ln, _ := r.FieldPos(0)
		if err != nil {
			return nil, fmt.Errorf("on row %d: %v", ln, err)
		}

		f := "key"
		k, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}

		f = "color"
		val := strings.Split(row[fields[f]], ",")
		if len(val) != 3 {
			return nil, fmt.Errorf("on row %d: field %q: found %d values, want 3", ln, f, len(val))
		}

		red, err := strconv.Atoi(strings.TrimSpace(val[0]))
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q [red value]: %v", ln, f, err)
		}
		if red > 255 {
			return nil, fmt.Errorf("on row %d: field %q [red value]: invalid value %d", ln, f, red)
		}
		green, err := strconv.Atoi(strings.TrimSpace(val[1]))
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q [green value]: %v", ln, f, err)
		}
		if green > 255 {
			return nil, fmt.Errorf("on row %d: field %q [green value]: invalid value %d", ln, f, green)
		}
		blue, err := strconv.Atoi(strings.TrimSpace(val[2]))
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q [blue value]: %v", ln, f, err)
		}
		if blue > 255 {
			return nil, fmt.Errorf("on row %d: field %q [blue value]: invalid value %d", ln, f, blue)
		}

		c := color.RGBA{uint8(red), uint8(green), uint8(blue), 255}
		pk.color[k] = c

		f = "gray"
		if _, ok := fields[f]; !ok {
			continue
		}
		gray, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if gray > 255 {
			return nil, fmt.Errorf("on row %d: field %q: invalid value %d", ln, f, gray)
		}

		pk.gray[k] = uint8(gray)
	}
	return pk, nil
}
