// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package settle implements a command to manage
// a settlement matrix defined for a project.
package settle

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/js-arias/command"
	"github.com/js-arias/earth/pixkey"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/phygeo/trait"
)

var Command = &command.Command{
	Usage: "settle [--add <file>] [--set <value>] <project-file>",
	Short: "manage settlement matrix",
	Long: `
Command settle manages a settlement matrix for a random walk defined for a
PhyGeo project. A settlement matrix contains the settlement weights on a
particular landscape feature (i.e., a pixel raster value) for a given trait.

The argument of the command is the name of the project file.

By default, the command will print the currently defined settlement matrix
weights into the standard input. If the flag --add is defined, the indicated
file will be used as the settlement matrix of the project.

If the flag --set is defined, it will set a settlement weight for a trait-
landscape pair. The sintaxis of the definition is:

	"<trait>,<feature-value>=<weight>"

Always use the quotations.

If there is no settlement matrix file defined in the project, a new file will
be created using the project file name as prefix and "-settlement.tab" as
suffix.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var settleFile string
var setFlag string

func setFlags(c *command.Command) {
	c.Flags().StringVar(&settleFile, "add", "", "")
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

	// landscape keys should be already defined
	keys, err := p.Keys()
	if err != nil {
		return err
	}

	// traits should be already defined
	traits, err := p.Traits()
	if err != nil {
		return err
	}

	if settleFile != "" {
		if err := validSettlementFile(traits, keys); err != nil {
			return err
		}
		p.Add(project.Settlement, settleFile)
		if err := p.Write(); err != nil {
			return err
		}
		return nil
	}

	if setFlag != "" {
		var st *trait.Matrix
		stF := p.Path(project.Settlement)
		if stF == "" {
			st = trait.NewMatrix(traits, keys)
		} else {
			st, err = p.Settlement(traits, keys)
			if err != nil {
				return err
			}
		}

		s, l, w, err := getWeight()
		if err != nil {
			return err
		}
		st.Add(s, l, w)

		if stF == "" {
			stF = defWeightName(args[0])
			if err := writeSettlementFile(stF, st); err != nil {
				return err
			}
			p.Add(project.Settlement, stF)
			if err := p.Write(); err != nil {
				return err
			}
			return nil
		}
		if err := writeSettlementFile(stF, st); err != nil {
			return err
		}
		return nil
	}

	st, err := p.Settlement(traits, keys)
	if err != nil {
		return err
	}

	for _, t := range st.Traits() {
		for _, l := range st.Landscape() {
			fmt.Fprintf(c.Stdout(), "%s\t%s\t%.6f\n", t, l, st.Weight(t, l))
		}
	}

	return nil
}

func validSettlementFile(traits *trait.Data, keys *pixkey.PixKey) error {
	f, err := os.Open(settleFile)
	if err != nil {
		return err
	}
	defer f.Close()

	mv := trait.NewMatrix(traits, keys)
	if err := mv.ReadTSV(f); err != nil {
		return fmt.Errorf("invalid file %q: %v", settleFile, err)
	}
	return nil
}

func defWeightName(path string) string {
	p := filepath.Base(path)
	i := strings.LastIndex(p, ".")
	return p[:i] + "-settlement.tab"
}

func getWeight() (state, landscape string, weight float64, err error) {
	s := strings.Split(setFlag, "=")
	if len(s) < 2 {
		return "", "", 0, fmt.Errorf("invalid --set value: %q", setFlag)
	}

	v := strings.Split(s[0], ",")
	if len(s) < 2 {
		return "", "", 0, fmt.Errorf("invalid --set value: %q", setFlag)
	}

	w, err := strconv.ParseFloat(s[1], 64)
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid --set value: %q: %v", setFlag, err)
	}
	if w < 0 {
		return "", "", 0, fmt.Errorf("invalid --set value: %q: invalid weight value", setFlag)
	}

	return v[0], v[1], w, nil
}

func writeSettlementFile(name string, m *trait.Matrix) (err error) {
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

	if err := m.TSV(f); err != nil {
		return err
	}
	return nil
}
