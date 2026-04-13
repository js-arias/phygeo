// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package param implements a command to set the ID
// of a model parameter in a PhyGeo model.
package param

import (
	"fmt"
	"io"
	"os"

	"github.com/js-arias/command"
	"github.com/js-arias/phygeo/infer/model"
)

var Command = &command.Command{
	Usage: `param [--id <number>]
	[--type <parameter-type>] [--name <parameter name>]
	<model-file>
	`,
	Short: "set the ID of a model parameter",
	Long: `
Command param reads a PhyGeo model definition and sets the ID of a parameter
of the model.

The first argument of the command is the name of the file with the model
definition.

Without any flag it will print all the defined parameter IDs.

If the flag --id is defined, the indicated number will be set as the ID of the
modified parameter. Set the ID of zero, will make the variable to be a fixed
value. If the flag --id is defined, it is required to also define the flags
--type and --name. The flag --type is for the parameter types. Valid values
are:
	walk for the random walk variables
	rate for the rate scalar variables used to relax the random walk
	mov  for movement weights
	sett for settlement weights
The flag --name is for the name of the parameter, and depends on the type of
parameter and the trait values and raster landscapes defined in the project
datasets.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var idFlag int
var nameFlag string
var typeFlag string

func setFlags(c *command.Command) {
	c.Flags().IntVar(&idFlag, "id", -1, "")
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

	if idFlag < 0 {
		printIDs(c.Stdout(), mp)
		return nil
	}

	mp.AsParam(nameFlag, model.Type(typeFlag), idFlag, -1)
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

func printIDs(w io.Writer, mp *model.Model) {
	IDs := mp.IDs()
	IDs = append(IDs, 0)
	for _, id := range IDs {
		if id == 0 {
			fmt.Fprintf(w, "Fixed values:\n")
		} else {
			fmt.Fprintf(w, "ID: %d\n", id)
		}
		for _, tp := range mp.Types() {
			for _, pn := range mp.Names(tp) {
				v := mp.ID(pn, tp)
				if v != id {
					continue
				}
				fmt.Fprintf(w, "\t%s, %s\n", tp, pn)
			}
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
