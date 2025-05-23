// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package keys implements a command to manage
// pixel key values defined for a project.
package keys

import (
	"errors"
	"fmt"
	"image/color"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/js-arias/blind"
	"github.com/js-arias/command"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/pixkey"
	"github.com/js-arias/phygeo/project"
)

var Command = &command.Command{
	Usage: `keys [--add <file>]
	[--set <value>] [--gray <value>] [--label <value>]
	 <project-file>`,
	Short: "manage pixel key values",
	Long: `
Command key manages pixel key values used in the time pixelation of a PhyGeo
project. The keys contains the labels assigned to the pixel values, and their
colors.

The argument of the command is the name of the project file.

By default, the command will print the currently defined pixel keys into the
standard output. If the flag --add is defined, the indicated file will be used
as the key file of the project. If the added file does not exists it will
create a new file with random colors for each value defined in the landscape
model.

If the flag --set is defined, it will set the color of a pixel value. The
sintaxis of the definition is:

	"<value>=<red>,<green>,<blue>"

Always use the quotations, as comma can have a different meaning in your OS.
The color values are in RGB and should be between 0 and 255.

If the flag --gray is defined, it will set the gray color of a pixel value.
The sintaxis of the definition is:

	"<value>=<color-value>"

Always use the quotations. The color value is an integer between 0 (black) and
255 (white).

If the flag --label is defined, it will set the the label of a pixel value.
The sintaxis of the definition is:

	"<value>=<label>"

Always use quotations.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var grayFlag string
var keysFile string
var labelFlag string
var setFlag string

func setFlags(c *command.Command) {
	c.Flags().StringVar(&grayFlag, "gray", "", "")
	c.Flags().StringVar(&keysFile, "add", "", "")
	c.Flags().StringVar(&labelFlag, "label", "", "")
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

	// Landscape should be already defined.
	landscape, err := p.Landscape(nil)
	if err != nil {
		return err
	}

	if keysFile != "" {
		if err := validKeyFile(landscape); err != nil {
			return err
		}
		p.Add(project.Keys, keysFile)
		if err := p.Write(); err != nil {
			return err
		}
		return nil
	}

	setArgs := false
	if setFlag != "" {
		setArgs = true
	}
	if grayFlag != "" {
		setArgs = true
	}
	if labelFlag != "" {
		setArgs = true
	}

	if !setArgs {
		if err := report(c.Stdout(), p, landscape); err != nil {
			return err
		}
		return nil
	}

	var keys *pixkey.PixKey
	kf := p.Path(project.Keys)
	if kf == "" {
		keys = buildRandomKey(landscape)
	} else {
		keys, err = p.Keys()
		if err != nil {
			return err
		}
	}

	if setFlag != "" {
		v, cc, err := getKeyColor()
		if err != nil {
			return err
		}
		keys.SetColor(cc, v)
	}
	if grayFlag != "" {
		v, cc, err := getKeyGray()
		if err != nil {
			return err
		}
		keys.SetGray(cc, v)
	}
	if labelFlag != "" {
		v, l, err := getKeyLabel()
		if err != nil {
			return err
		}
		keys.SetLabel(v, l)
	}

	if kf == "" {
		kf = defKeyFileName(args[0])
		if err := writeKeyFile(kf, keys); err != nil {
			return err
		}
		p.Add(project.Keys, kf)
		if err := p.Write(); err != nil {
			return err
		}
		return nil
	}
	if err := writeKeyFile(kf, keys); err != nil {
		return err
	}
	return nil
}

func report(w io.Writer, p *project.Project, landscape *model.TimePix) error {
	k, err := p.Keys()
	if err != nil {
		return err
	}

	val := make(map[int]bool)
	for _, age := range landscape.Stages() {
		s := landscape.Stage(age)
		for _, v := range s {
			val[v] = true
		}
	}
	val[0] = true

	noLand := make(map[int]bool)
	for _, v := range k.Keys() {
		if val[v] {
			continue
		}
		noLand[v] = true
		val[v] = true
	}

	pv := make([]int, 0, len(val))
	for v := range val {
		pv = append(pv, v)
	}
	slices.Sort(pv)

	for _, v := range pv {
		l := k.Label(v)
		if l == "" {
			l = "undefined"
		}
		c, _ := k.Color(v)
		r, g, b, _ := c.RGBA()
		cv := fmt.Sprintf("%d,%d,%d", uint8(r), uint8(g), uint8(b))

		gv := "-"
		if k.HasGrayScale() {
			c, _ := k.Gray(v)
			r, _, _, _ := c.RGBA()
			gv = fmt.Sprintf("%d", uint8(r))
		}

		nl := ""
		if noLand[v] {
			nl = "\tnot in landscape"
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s%s\n", v, l, cv, gv, nl)
	}

	return nil
}

func validKeyFile(landscape *model.TimePix) error {
	f, err := os.Open(keysFile)
	if errors.Is(err, os.ErrNotExist) {
		keys := buildRandomKey(landscape)
		if err := writeKeyFile(keysFile, keys); err != nil {
			return err
		}
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := pixkey.Read(f); err != nil {
		return fmt.Errorf("on file %q: %v", keysFile, err)
	}
	return nil
}

func buildRandomKey(landscape *model.TimePix) *pixkey.PixKey {
	keys := pixkey.New()

	for _, age := range landscape.Stages() {
		s := landscape.Stage(age)
		for _, v := range s {
			if _, ok := keys.Color(v); ok {
				continue
			}
			keys.SetColor(randColor(), v)
			c := uint8(rand.Int())
			keys.SetGray(color.RGBA{c, c, c, 255}, v)
			keys.SetLabel(v, strconv.Itoa(v))
		}
	}
	if _, ok := keys.Color(0); !ok {
		keys.SetColor(randColor(), 0)
		keys.SetGray(color.RGBA{255, 255, 255, 255}, 0)
		keys.SetLabel(0, "0")
	}

	return keys
}

func defKeyFileName(path string) string {
	p := filepath.Base(path)
	i := strings.LastIndex(p, ".")
	return p[:i] + "-keys.tab"
}

func writeKeyFile(name string, keys *pixkey.PixKey) (err error) {
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

	if err := keys.TSV(f); err != nil {
		return fmt.Errorf("while writing %q: %v", name, err)
	}
	return nil
}

func randColor() color.RGBA {
	return blind.Sequential(blind.Iridescent, rand.Float64())
}

func getKeyColor() (key int, c color.Color, err error) {
	s := strings.Split(setFlag, "=")
	if len(s) < 2 {
		return 0, color.RGBA{}, fmt.Errorf("invalid --set value: %q", setFlag)
	}
	key, err = strconv.Atoi(s[0])
	if err != nil {
		return 0, color.RGBA{}, fmt.Errorf("invalid --set value: %q: %v", setFlag, err)
	}

	vals := strings.Split(s[1], ",")
	if len(vals) < 3 {
		return 0, color.RGBA{}, fmt.Errorf("invalid --set value: %q", setFlag)
	}

	r, err := strconv.Atoi(vals[0])
	if err != nil {
		return 0, color.RGBA{}, fmt.Errorf("invalid --set value: %q: %v", setFlag, err)
	}
	g, err := strconv.Atoi(vals[1])
	if err != nil {
		return 0, color.RGBA{}, fmt.Errorf("invalid --set value: %q: %v", setFlag, err)
	}
	b, err := strconv.Atoi(vals[2])
	if err != nil {
		return 0, color.RGBA{}, fmt.Errorf("invalid --set value: %q: %v", setFlag, err)
	}

	return key, color.RGBA{uint8(r), uint8(g), uint8(b), 255}, nil
}

func getKeyGray() (key int, c color.Color, err error) {
	s := strings.Split(grayFlag, "=")
	if len(s) < 2 {
		return 0, color.RGBA{}, fmt.Errorf("invalid --gray value: %q", grayFlag)
	}
	key, err = strconv.Atoi(s[0])
	if err != nil {
		return 0, color.RGBA{}, fmt.Errorf("invalid --gray value: %q: %v", grayFlag, err)
	}

	g, err := strconv.Atoi(s[1])
	if err != nil {
		return 0, color.RGBA{}, fmt.Errorf("invalid --gray value: %q: %v", grayFlag, err)
	}

	return key, color.RGBA{uint8(g), uint8(g), uint8(g), 255}, nil
}

func getKeyLabel() (key int, l string, err error) {
	s := strings.Split(labelFlag, "=")
	if len(s) < 2 {
		return 0, "", fmt.Errorf("invalid --label value: %q", labelFlag)
	}
	key, err = strconv.Atoi(s[0])
	if err != nil {
		return 0, "", fmt.Errorf("invalid --label value: %q: %v", labelFlag, err)
	}
	l = strings.Join(s[1:], " ")

	return key, l, nil
}
