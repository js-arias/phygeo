// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package stages implements a command to manage
// the time stages defined in the project.
package stages

import (
	"fmt"
	"os"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/phygeo/timestage"
)

var Command = &command.Command{
	Usage: `stages [--add <value>]
	[--each <value>] [--from <value>] [--to <value>]
	[-f|--file <file>] <project>`,
	Short: "manage time stages",
	Long: `
Command stages manage the time stages defined for a PhyGeo project.

The argument of the command is the name of the project file.

By default, the command will print the time stages (in million years) defined
for the project.

In most cases, the time stages are just the time stages defined by the
underlying dynamic geography models. But it is possible that different time
stages are defined (for example, if the tree is too young for the timescale of
the dynamic geography model, as is the case of phylogeography). To add
additional time stages, use the flag --add, with a value in million years. To
add multiple time stages, use the flag --each with a value in million years,
and time stages will be added sequentially between the oldest age (defined by
the flag --from; the default is the oldest defined stage) and the youngest age
(defined by the flag --to; the default is the present).

If the flag --file or -f is defined, the time stages will be stored in the
indicated file. If at least a stage is added and no stage file is defined, the
default file name will be 'stages.tab'.

The file for the time stages is just a tab-delimited file without a header, in
which the first column contains the time stages in years. Here is an example:

	# time stages
	0
	5000000
	10000000
	100000000
	200000000
	300000000
	`,
	SetFlags: setFlags,
	Run:      run,
}

var addFlag float64
var eachFlag float64
var fromFlag float64
var toFlag float64
var stageFile string

func setFlags(c *command.Command) {
	c.Flags().Float64Var(&addFlag, "add", -1, "")
	c.Flags().Float64Var(&eachFlag, "each", -1, "")
	c.Flags().Float64Var(&fromFlag, "from", -1, "")
	c.Flags().Float64Var(&toFlag, "to", -1, "")
	c.Flags().StringVar(&stageFile, "file", "", "")
	c.Flags().StringVar(&stageFile, "f", "", "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}

	p, err := project.Read(args[0])
	if err != nil {
		return err
	}

	stages := timestage.New()
	if err := readTimeStages(p, stages); err != nil {
		return err
	}

	stF := p.Path(project.Stages)
	if stF == "" {
		stF = "stages.tab"
	}
	write := false

	if addFlag >= 0 {
		write = true
		add := int64(addFlag * timestage.MillionYears)
		stages.AddStage(add)
	} else if eachFlag > 0 {
		write = true
		each := int64(eachFlag * timestage.MillionYears)
		st := stages.Stages()
		from := st[len(st)-1]
		if fromFlag >= 0 {
			from = int64(fromFlag * timestage.MillionYears)
		}
		var to int64
		if toFlag >= 0 {
			to = int64(toFlag * timestage.MillionYears)
		}
		for a := from; a >= to; a -= each {
			stages.AddStage(a)
		}
	}

	if !write {
		for _, a := range stages.Stages() {
			fmt.Fprintf(c.Stdout(), "%.6f\n", float64(a)/timestage.MillionYears)
		}
		return nil
	}

	if stageFile != "" {
		stF = stageFile
	}
	if err := writeStages(stF, stages); err != nil {
		return err
	}

	p.Add(project.Stages, stF)
	if err := p.Write(args[0]); err != nil {
		return err
	}
	return nil
}

func readTimeStages(p *project.Project, stages timestage.Stages) (err error) {
	var pix *earth.Pixelation

	rotF := p.Path(project.GeoMotion)
	if rotF != "" {
		pix, err = readRotation(rotF, stages)
		if err != nil {
			return err
		}
	}

	lsF := p.Path(project.Landscape)
	if lsF != "" {
		if err = readLandscape(lsF, pix, stages); err != nil {
			return err
		}
	}

	stF := p.Path(project.Stages)
	if stF == "" {
		return nil
	}
	f, err := os.Open(stF)
	if err != nil {
		return err
	}
	defer f.Close()

	st, err := timestage.Read(f)
	if err != nil {
		return fmt.Errorf("on file %q: %v", stF, err)
	}
	stages.Add(st)

	return nil
}

func readRotation(name string, st timestage.Stages) (*earth.Pixelation, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rot, err := model.ReadTotal(f, nil, false)
	if err != nil {
		return nil, fmt.Errorf("on file %q: %v", name, err)
	}
	pix := rot.Pixelation()

	st.Add(rot)
	return pix, nil
}

func readLandscape(name string, pix *earth.Pixelation, st timestage.Stages) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()

	tp, err := model.ReadTimePix(f, pix)
	if err != nil {
		return fmt.Errorf("on file %q: %v", name, err)
	}

	st.Add(tp)
	return nil
}

func writeStages(name string, stages timestage.Stages) (err error) {
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

	if err := stages.Write(f); err != nil {
		return err
	}
	return nil
}
