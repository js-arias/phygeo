// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package add implements a command to add taxon ranges
// to a PhyGeo project.
package add

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/gbifer/tsv"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/ranges"
	"github.com/js-arias/timetree"
)

var Command = &command.Command{
	Usage: `add [-f|--file <range-file>]
	[--type <file-type>] [--format <format>]
	[--filter]
	<project-file> [<range-file>...]`,
	Short: "add taxon ranges to a PhyGeo project",
	Long: `
Command add reads one or more taxon ranges from one or more range files, and
add the ranges to a PhyGeo project. The ranges can be either presence-absence
pixelations, or a continuous range maps.

The first argument of the command is the name of the project file. If no
project exists, a new project will be created.

One or more range files can be given as arguments. If no file is given the
ranges will be read from the standard input. A pixelation model must be
already defined for the project, either a rotation model, or a paleolandscape
model, and the pixelation of the input files must be consistent with that
pixelation model.

By default, only the taxon with ranges defined as presence-absence will be
read. Use the flag --type to define the type of the ranges to be read. The
type can be:

	points	presence-absence taxon ranges
	ranges	continuous range map

By default, the command adds presence-absence files using a tab-delimited
files with the pixel IDs. Using the flag --format, it is possible to define a
different file format. Valid formats are:

	phygeo  the default phygeo format
	darwin  DarwinCore format using tab characters as delimiters (e.g.,
	        the files downloaded from GBIF). Parsed fields are "species",
	        "decimalLatitude", and "decimalLongitude".
	pbdb    Tab-delimited files downloaded from PaleoBiology DataBase, the
	        following fields are required: "accepted_name", "lat", and
	        "lng".
	text    a simple tab-delimited file with the following fields:
	        "species", "latitude", and "longitude".
	csv     the same as text, but using commas as delimiters.

In formats different from the PhyGeo format, all entries are assumed to be
geo-referenced at the present time.

By default, all records in the input files will be added. If the flag --filter
is defined and there are trees in the project, then it will add only the
records that match a taxon name in the trees.

By default the range maps will be stored in the range files currently defined
for the project. If the project does not have a range file, a new one will be
created with the name 'points.tab' for presence-absence taxon ranges, or
'ranges.tab' for continuous range maps. A different file name can be defined
with the flag --file or -f. If this flag is used, and there is a range file
already defined, then a new file will be created, and used as the range file
for the added type of range map for the project (previously defined ranges
will be kept).
	`,
	SetFlags: setFlags,
	Run:      run,
}

var format string
var outFile string
var typeFlag string
var filterFlag bool

func setFlags(c *command.Command) {
	c.Flags().StringVar(&outFile, "file", "", "")
	c.Flags().StringVar(&outFile, "f", "", "")
	c.Flags().StringVar(&format, "format", "phygeo", "")
	c.Flags().StringVar(&typeFlag, "type", "", "")
	c.Flags().BoolVar(&filterFlag, "filter", false, "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}
	pFile := args[0]
	p, err := openProject(pFile)
	if err != nil {
		return err
	}

	typeFlag = strings.ToLower(typeFlag)
	if typeFlag == "" {
		typeFlag = string(project.Points)
	}
	switch d := project.Dataset(typeFlag); d {
	case project.Points:
		if err := addPoints(c.Stdin(), p, args[1:]); err != nil {
			return err
		}
	case project.Ranges:
		if err := addRanges(c.Stdin(), p, args[1:]); err != nil {
			return err
		}
	default:
		msg := fmt.Sprintf("flag --type: unknown value %q", typeFlag)
		return c.UsageError(msg)
	}

	if err := p.Write(pFile); err != nil {
		return err
	}

	return nil
}

func openProject(name string) (*project.Project, error) {
	p, err := project.Read(name)
	if errors.Is(err, os.ErrNotExist) {
		return project.New(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("unable ot open project %q: %v", name, err)
	}
	return p, nil
}

func makeFilter(p *project.Project) (map[string]bool, error) {
	tf := p.Path(project.Trees)
	if tf == "" {
		return nil, fmt.Errorf("project without trees")
	}

	f, err := os.Open(tf)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	c, err := timetree.ReadTSV(f)
	if err != nil {
		return nil, fmt.Errorf("while reading file %q: %v", tf, err)
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

func addPoints(r io.Reader, p *project.Project, files []string) error {
	pix, err := openPixelation(p)
	if err != nil {
		return err
	}

	var coll *ranges.Collection
	if pf := p.Path(project.Points); pf != "" {
		var err error
		coll, err = readCollection(r, pf)
		if err != nil {
			return err
		}
		if eq1, eq2 := pix.Equator(), coll.Pixelation().Equator(); eq1 != eq2 {
			return fmt.Errorf("invalid project file %q: got %d equatorial pixel, want %d", pf, eq2, eq1)
		}
	} else {
		coll = ranges.New(pix)
	}

	var filter map[string]bool
	if filterFlag {
		filter, err = makeFilter(p)
		if err != nil {
			return err
		}
	}

	readPtsFunc := readCollection
	switch strings.ToLower(format) {
	case "csv":
		readPtsFunc = func(r io.Reader, name string) (*ranges.Collection, error) {
			return readTextData(r, pix, name, ',')
		}
	case "darwin":
		readPtsFunc = func(r io.Reader, name string) (*ranges.Collection, error) {
			return readGBIFData(r, pix, name)
		}
	case "pbdb":
		readPtsFunc = func(r io.Reader, name string) (*ranges.Collection, error) {
			return readPaleoDBData(r, pix, name)
		}
	case "phygeo":
	case "text":
		readPtsFunc = func(r io.Reader, name string) (*ranges.Collection, error) {
			return readTextData(r, pix, name, '\t')
		}
	default:
		return fmt.Errorf("format %q unknown", format)
	}

	if len(files) == 0 {
		files = append(files, "-")
	}
	for _, f := range files {
		c, err := readPtsFunc(r, f)
		if err != nil {
			return err
		}
		cp := c.Pixelation()

		for _, nm := range c.Taxa() {
			if filterFlag {
				if !filter[nm] {
					continue
				}
			}
			if c.Type(nm) != ranges.Points {
				continue
			}
			age := c.Age(nm)
			rng := c.Range(nm)
			for id := range rng {
				pt := cp.ID(id).Point()
				coll.Add(nm, age, pt.Latitude(), pt.Longitude())
			}
		}
	}
	if len(coll.Taxa()) == 0 {
		return nil
	}

	ptsFile := p.Path(project.Points)
	if outFile != "" {
		ptsFile = outFile
	}
	if ptsFile == "" {
		ptsFile = "points.tab"
	}

	if err := writeCollection(ptsFile, coll); err != nil {
		return err
	}
	p.Add(project.Points, ptsFile)
	return nil
}

func addRanges(r io.Reader, p *project.Project, files []string) error {
	pix, err := openPixelation(p)
	if err != nil {
		return err
	}

	var filter map[string]bool
	if filterFlag {
		filter, err = makeFilter(p)
		if err != nil {
			return err
		}
	}

	var coll *ranges.Collection
	if rf := p.Path(project.Ranges); rf != "" {
		var err error
		coll, err = readCollection(r, rf)
		if err != nil {
			return err
		}
		if eq1, eq2 := pix.Equator(), coll.Pixelation().Equator(); eq1 != eq2 {
			return fmt.Errorf("invalid project file %q: got %d equatorial pixel, want %d", rf, eq2, eq1)
		}
	} else {
		coll = ranges.New(pix)
	}

	if len(files) == 0 {
		files = append(files, "-")
	}
	for _, f := range files {
		c, err := readCollection(r, f)
		if err != nil {
			return err
		}
		if eq1, eq2 := pix.Equator(), c.Pixelation().Equator(); eq1 != eq2 {
			return fmt.Errorf("invalid range file %q: got %d equatorial pixel, want %d", f, eq2, eq1)
		}

		for _, nm := range c.Taxa() {
			if filterFlag {
				if !filter[nm] {
					continue
				}
			}
			if c.Type(nm) != ranges.Range {
				continue
			}
			age := c.Age(nm)
			rng := c.Range(nm)
			coll.Set(nm, age, rng)
		}
	}

	if len(coll.Taxa()) == 0 {
		return nil
	}

	rngFile := p.Path(project.Ranges)
	if outFile != "" {
		rngFile = outFile
	}
	if rngFile == "" {
		rngFile = "ranges.tab"
	}

	if err := writeCollection(rngFile, coll); err != nil {
		return err
	}
	p.Add(project.Ranges, rngFile)
	return nil
}

func openPixelation(p *project.Project) (*earth.Pixelation, error) {
	if path := p.Path(project.Landscape); path != "" {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		tp, err := model.ReadTimePix(f, nil)
		if err != nil {
			return nil, fmt.Errorf("on file %q: %v", path, err)
		}
		return tp.Pixelation(), nil
	}
	if path := p.Path(project.GeoMotion); path != "" {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		tot, err := model.ReadTotal(f, nil, false)
		if err != nil {
			return nil, fmt.Errorf("on file %q: %v", path, err)
		}
		return tot.Pixelation(), nil
	}
	return nil, errors.New("undefined pixelation model")
}

func readCollection(r io.Reader, name string) (*ranges.Collection, error) {
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

	coll, err := ranges.ReadTSV(r, nil)
	if err != nil {
		return nil, fmt.Errorf("when reading %q: %v", name, err)
	}

	return coll, nil
}

var textHeaderFields = []string{
	"species",
	"latitude",
	"longitude",
}

func readTextData(r io.Reader, pix *earth.Pixelation, name string, comma rune) (*ranges.Collection, error) {
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

	in := csv.NewReader(r)
	in.Comma = comma
	in.Comment = '#'

	head, err := in.Read()
	if err != nil {
		return nil, fmt.Errorf("on file %q: while reading header: %v", name, err)
	}
	fields := make(map[string]int, len(head))
	for i, h := range head {
		h = strings.ToLower(h)
		fields[h] = i
	}
	for _, h := range textHeaderFields {
		if _, ok := fields[h]; !ok {
			return nil, fmt.Errorf("on file %q: expecting field %q", name, h)
		}
	}

	coll := ranges.New(pix)
	for {
		row, err := in.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		ln, _ := in.FieldPos(0)
		if err != nil {
			return nil, fmt.Errorf("on file %q: row %d: %v", name, ln, err)
		}

		f := "species"
		tax := row[fields[f]]

		f = "latitude"
		lat, err := strconv.ParseFloat(row[fields[f]], 64)
		if err != nil {
			return nil, fmt.Errorf("on file %q: row %d: field %q: %v", name, ln, f, err)
		}
		if lat < -90 || lat > 90 {
			return nil, fmt.Errorf("on file %q: row %d: field %q: invalid latitude %.6f", name, ln, f, lat)
		}

		f = "longitude"
		lon, err := strconv.ParseFloat(row[fields[f]], 64)
		if err != nil {
			return nil, fmt.Errorf("on file %q: row %d: field %q: %v", name, ln, f, err)
		}
		if lon < -180 || lon > 180 {
			return nil, fmt.Errorf("on file %q: row %d: field %q: invalid longitude %.6f", name, ln, f, lon)
		}

		coll.Add(tax, 0, lat, lon)
	}
	return coll, nil
}

var gbifFields = []string{
	"species",
	"decimallatitude",
	"decimallongitude",
}

func readGBIFData(r io.Reader, pix *earth.Pixelation, name string) (*ranges.Collection, error) {
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

	tab := tsv.NewReader(r)

	head, err := tab.Read()
	if err != nil {
		return nil, fmt.Errorf("on file %q: while reading header: %v", name, err)
	}
	fields := make(map[string]int, len(head))
	for i, h := range head {
		h = strings.ToLower(h)
		fields[h] = i
	}
	for _, h := range gbifFields {
		if _, ok := fields[h]; !ok {
			return nil, fmt.Errorf("on file %q: expecting field %q", name, h)
		}
	}

	coll := ranges.New(pix)
	for {
		row, err := tab.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		ln, _ := tab.FieldPos(0)
		if err != nil {
			return nil, fmt.Errorf("on file %q: row %d: %v", name, ln, err)
		}

		f := "species"
		tax := row[fields[f]]

		f = "decimallatitude"
		lat, err := strconv.ParseFloat(row[fields[f]], 64)
		if err != nil {
			return nil, fmt.Errorf("on file %q: row %d: field %q: %v", name, ln, f, err)
		}
		if lat < -90 || lat > 90 {
			return nil, fmt.Errorf("on file %q: row %d: field %q: invalid latitude %.6f", name, ln, f, lat)
		}

		f = "decimallongitude"
		lon, err := strconv.ParseFloat(row[fields[f]], 64)
		if err != nil {
			return nil, fmt.Errorf("on file %q: row %d: field %q: %v", name, ln, f, err)
		}
		if lon < -180 || lon > 180 {
			return nil, fmt.Errorf("on file %q: row %d: field %q: invalid longitude %.6f", name, ln, f, lon)
		}

		coll.Add(tax, 0, lat, lon)
	}

	return coll, nil
}

var pbdbFields = []string{
	"accepted_name",
	"lat",
	"lng",
}

func readPaleoDBData(r io.Reader, pix *earth.Pixelation, name string) (*ranges.Collection, error) {
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

	br := bufio.NewReader(r)
	metaLines := 0
	for {
		ln, err := br.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("on file %q: %v", name, err)
		}
		metaLines++
		if strings.HasPrefix(ln, "Records:") {
			break
		}
	}

	tab := tsv.NewReader(br)

	head, err := tab.Read()
	if err != nil {
		return nil, fmt.Errorf("on file %q: while reading header: %v", name, err)
	}
	fields := make(map[string]int, len(head))
	for i, h := range head {
		h = strings.ToLower(h)
		fields[h] = i
	}
	for _, h := range pbdbFields {
		if _, ok := fields[h]; !ok {
			return nil, fmt.Errorf("on file %q: expecting field %q", name, h)
		}
	}

	coll := ranges.New(pix)
	for {
		row, err := tab.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		ln, _ := tab.FieldPos(0)
		ln += metaLines
		if err != nil {
			return nil, fmt.Errorf("on file %q: row %d: %v", name, ln, err)
		}

		f := "accepted_name"
		tax := row[fields[f]]

		f = "lat"
		lat, err := strconv.ParseFloat(row[fields[f]], 64)
		if err != nil {
			return nil, fmt.Errorf("on file %q: row %d: field %q: %v", name, ln, f, err)
		}
		if lat < -90 || lat > 90 {
			return nil, fmt.Errorf("on file %q: row %d: field %q: invalid latitude %.6f", name, ln, f, lat)
		}

		f = "lng"
		lon, err := strconv.ParseFloat(row[fields[f]], 64)
		if err != nil {
			return nil, fmt.Errorf("on file %q: row %d: field %q: %v", name, ln, f, err)
		}
		if lon < -180 || lon > 180 {
			return nil, fmt.Errorf("on file %q: row %d: field %q: invalid longitude %.6f", name, ln, f, lon)
		}

		coll.Add(tax, 0, lat, lon)
	}

	return coll, nil
}

func writeCollection(name string, coll *ranges.Collection) (err error) {
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

	if err := coll.TSV(f); err != nil {
		return fmt.Errorf("while writing to %q: %v", name, err)
	}
	return nil
}
