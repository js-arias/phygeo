// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package weights implements a command to manage
// pixel weights defined for a project.
package weights

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/js-arias/command"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/stat/pixweight"
	"github.com/js-arias/phygeo/project"
)

var Command = &command.Command{
	Usage: "weights [--add <file>] [--set <value>] <project-file>",
	Short: "manage pixel weights",
	Long: `
Command prior manage pixel normalized weights defined for a PhyGeo project.
The weight is a form of normalized prior for the pixel, so the maximum value
is always 1.0.

The argument of the command is the name of the project file.

By default, the command will print the currently defined pixel weights into
the standard output. If the flag --add is defined, the indicated file will be
used as the pixel weights of the project.

If the flag --set is defined, it will set a pixel weight to a raster value.
The sintaxis of the definition is:

	<value>=<probability>

If there is no pixel weights file defined in the project, a new file will be
created using the project file name as a prefix and "-pix-weights.tab" as a
suffix.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var weightsFile string
var setFlag string

func setFlags(c *command.Command) {
	c.Flags().StringVar(&weightsFile, "add", "", "")
	c.Flags().StringVar(&setFlag, "set", "", "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}

	p, err := project.Read(args[0])
	if err != nil {
		return err
	}

	if weightsFile != "" {
		if _, err := readPriorFile(weightsFile); err != nil {
			return err
		}
		p.Add(project.PixWeight, weightsFile)
		if err := p.Write(); err != nil {
			return err
		}
		return nil
	}

	if setFlag != "" {
		pw := pixweight.New()
		pwF := p.Path(project.PixWeight)
		if pwF != "" {
			pw, err = readPriorFile(p.Path(project.PixWeight))
			if err != nil {
				return err
			}
		} else {
			pwF = makePixPriorFileName(args[0])
		}

		k, prob, err := getKeyProb()
		if err != nil {
			return err
		}
		pw.Set(k, prob)

		if err := writePWF(pwF, pw); err != nil {
			return err
		}
		p.Add(project.PixWeight, pwF)
		if err := p.Write(); err != nil {
			return err
		}
		return nil
	}

	pwF := p.Path(project.PixWeight)
	if pwF == "" {
		if tp := p.Path(project.Landscape); tp != "" {
			pw := pixweight.New()
			if err := reportWithLandscape(c.Stderr(), tp, pw); err != nil {
				return err
			}
		}
		return fmt.Errorf("pixel weights undefined for project %q", args[0])
	}

	pw, err := readPriorFile(p.Path(project.PixWeight))
	if err != nil {
		return err
	}
	if tp := p.Path(project.Landscape); tp != "" {
		if err := reportWithLandscape(c.Stdout(), tp, pw); err != nil {
			return err
		}
		return nil
	}

	for _, v := range pw.Values() {
		fmt.Fprintf(c.Stdout(), "%d\t%.6f\n", v, pw.Weight(v))
	}

	return nil
}

func reportWithLandscape(w io.Writer, name string, pw pixweight.Pixel) error {
	tp, err := readLandscape(name)
	if err != nil {
		return err
	}

	val := make(map[int]bool)
	for _, age := range tp.Stages() {
		s := tp.Stage(age)
		for _, v := range s {
			val[v] = true
		}
	}
	val[0] = true

	notLand := make(map[int]bool)
	for _, v := range pw.Values() {
		if val[v] {
			continue
		}
		notLand[v] = true
		val[v] = true
	}

	pv := make([]int, 0, len(val))
	for v := range val {
		pv = append(pv, v)
	}
	slices.Sort(pv)

	for _, v := range pv {
		wt := pw.Weight(v)
		if notLand[v] {
			fmt.Fprintf(w, "%d\t%.6f\tpixel value not in landscape\n", v, wt)
			continue
		}
		if wt == 0 {
			fmt.Fprintf(w, "%d\t%.6f\tpixel weight undefined\n", v, wt)
			continue
		}
		fmt.Fprintf(w, "%d\t%.6f\n", v, wt)
	}

	return nil
}

func readPriorFile(name string) (pixweight.Pixel, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	pp, err := pixweight.ReadTSV(f)
	if err != nil {
		return nil, fmt.Errorf("when reading %q: %v", name, err)
	}

	return pp, nil
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

func makePixPriorFileName(path string) string {
	p := filepath.Base(path)
	i := strings.LastIndex(p, ".")
	return p[:i] + "-pix-prob.tab"
}

func getKeyProb() (key int, prob float64, err error) {
	s := strings.Split(setFlag, "=")
	if len(s) < 2 {
		return 0, 0, fmt.Errorf("invalid --set value: %q", setFlag)
	}
	key, err = strconv.Atoi(s[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid --set value: %q: %v", setFlag, err)
	}
	prob, err = strconv.ParseFloat(s[1], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid --set value: %q: %v", setFlag, err)
	}
	if prob < 0 || prob > 1 {
		return 0, 0, fmt.Errorf("invalid --set value: %q: invalid probability value", setFlag)
	}

	return key, prob, nil
}

func writePWF(name string, pw pixweight.Pixel) (err error) {
	var f *os.File
	f, err = os.Create(name)
	if err != nil {
		return err
	}
	defer func() {
		e := f.Close()
		if e != nil && err == nil {
			err = e
		}
	}()

	if err := pw.TSV(f); err != nil {
		return err
	}
	return nil
}
