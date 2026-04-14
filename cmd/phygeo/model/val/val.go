// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package val implements a command to set the value
// of a model variable of a PhyGeo model definition.
package val

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/infer/model"
)

var Command = &command.Command{
	Usage: `val [--max]
	[--type <parameter-type>] [--name <parameter name>]
	<model-file> [<value>]`,
	Short: "set the value of a model parameter",
	Long: `
Command val reads a PhyGeo model definition and set the value of the indicated
parameter of the model.

The first argument of the command is the name of the file with the model
definition.

Without any flag it will print all current values.

The second parameter indicates the new value. This requires the definition of
the --type and --name flags. The flag --type is for the parameter types. Valid
values are:
	walk for the random walk variables
	rate for the rate scalar variables used to relax the random walk
	mov  for movement weights
	sett for settlement weights
The flag --name is for the name of the parameter, and depends on the type of
parameter and the trait values and raster landscapes defined in the project
datasets.

By default, the current value of the parameter-variable will be updated, if
the flag --max is defined, then the value will be set as the maximum value for
the parameter.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var maxFlag bool
var nameFlag string
var typeFlag string

func setFlags(c *command.Command) {
	c.Flags().BoolVar(&maxFlag, "max", false, "")
	c.Flags().StringVar(&nameFlag, "name", "", "")
	c.Flags().StringVar(&typeFlag, "type", "", "")
}

func run(c *command.Command, args []string) (err error) {
	if len(args) < 1 {
		return c.UsageError("expecting model file name")
	}

	mp, err := open(args[0])
	if err != nil {
		return err
	}
	fn := mp.Set
	if maxFlag {
		fn = mp.SetMax
	}

	if nameFlag == "" && typeFlag == "" {
		printVals(c.Stdout(), mp)
		return nil
	}
	if len(args) < 2 {
		return c.UsageError("expecting numerical value")
	}
	v, err := strconv.ParseFloat(args[1], 64)

	fn(nameFlag, model.Type(typeFlag), v)
	f, err := os.Create(args[0])
	if err != nil {
		return err
	}
	defer func() {
		e := f.Close()
		if err == nil && e != nil {
			err = e
		}
	}()
	if err := mp.TSV(f); err != nil {
		return err
	}
	return nil
}

func printVals(w io.Writer, mp *model.Model) {
	for _, tp := range mp.Types() {
		fmt.Fprintf(w, "%s\n", tp)
		for _, pn := range mp.Names(tp) {
			v := mp.Val(pn, tp)
			id := mp.ID(pn, tp)
			if id == 0 {
				fmt.Fprintf(w, "\t%s [fixed] = %.6f\n", pn, v)
				continue
			}
			mx := mp.Max(pn, tp)
			fmt.Fprintf(w, "\t%s [param:%d] = %.6f [max = %.6f]\n", pn, id, v, mx)
		}
	}
}

func open(name string) (*model.Model, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	mp, err := model.Read(f)
	if err != nil {
		return nil, fmt.Errorf("on file %q: %v", name, err)
	}
	return mp, nil
}
