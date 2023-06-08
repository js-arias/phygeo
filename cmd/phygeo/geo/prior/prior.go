// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package prior implements a command to manage
// pixel priors defined for a project.
package prior

import (
	"fmt"
	"io"
	"os"

	"github.com/js-arias/command"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/stat/pixprob"
	"github.com/js-arias/phygeo/project"
	"golang.org/x/exp/slices"
)

var Command = &command.Command{
	Usage: "prior [--add <file>] <project-file>",
	Short: "manage pixel priors",
	Long: `
Command prior manage pixel priors defined for a PhyGeo project.

The argument of the command is the name of the project file.

By default, the command will print the currently defined pixel priors into the
standard output. If the flag --add is defined, the indicated file will be used
as the pixel prior of the project.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var priorFile string

func setFlags(c *command.Command) {
	c.Flags().StringVar(&priorFile, "add", "", "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}

	p, err := project.Read(args[0])
	if err != nil {
		return err
	}

	if priorFile != "" {
		if _, err := readPriorFile(priorFile); err != nil {
			return err
		}
		p.Add(project.PixPrior, priorFile)
		if err := p.Write(args[0]); err != nil {
			return err
		}
		return nil
	}

	ppF := p.Path(project.PixPrior)
	if ppF == "" {
		return fmt.Errorf("pixel prior undefined for project %q", args[0])
	}

	pp, err := readPriorFile(p.Path(project.PixPrior))
	if err != nil {
		return err
	}
	if tp := p.Path(project.Landscape); tp != "" {
		if err := reportWithLandscape(c.Stdout(), tp, pp); err != nil {
			return err
		}
		return nil
	}

	for _, v := range pp.Values() {
		fmt.Fprintf(c.Stdout(), "%d\t%.6f\n", v, pp.Prior(v))
	}

	return nil
}

func reportWithLandscape(w io.Writer, name string, pp pixprob.Pixel) error {
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

	pv := make([]int, 0, len(val))
	for v := range val {
		pv = append(pv, v)
	}
	slices.Sort(pv)

	for _, v := range pv {
		fmt.Fprintf(w, "%d\t%.6f\n", v, pp.Prior(v))
	}

	return nil
}

func readPriorFile(name string) (pixprob.Pixel, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	pp, err := pixprob.ReadTSV(f)
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
