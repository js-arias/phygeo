// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package ml implements a command to search
// for the maximum likelihood estimation
// of a biogeographic reconstruction.
package ml

import (
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"runtime"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/pixkey"
	"github.com/js-arias/phygeo/infer/catwalk"
	"github.com/js-arias/phygeo/infer/model"
	"github.com/js-arias/phygeo/infer/walk"
	"github.com/js-arias/phygeo/infer/walker"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/phygeo/trait"
	"gonum.org/v1/gonum/optimize"
)

var Command = &command.Command{
	Usage: `ml
	[--cpu <number>]
	[--mult <number>] [--current]
	[--delta <number>] [--eval <number>]
	[--iter <number>]
	[-o|--output <prefix>]
	--model <model-file> <project-file>`,
	Short: "search the maximum likelihood estimate",
	Long: `
Command ml reads a PhyGeo project and a model definition, and search for the
maximum likelihood estimate of the parameters.

The argument of the command is the name of the project file.

The flag --model is required, and is used to set the name of the model
definition. Because the movement weight and lambda are not identifiable, it is
required that at least one movement weight will be fixed at 1.0. The resulting
values will be stored in a file called "model-ml-<tree>.tab". If the flag -o
or --output is defined, the indicated value will be used as prefix of the
output file model.

As lambda values are usually larger compared to other parameters, its
convergence. To reduce that problem, the search parameter is small (closer to
1.0) and internally multiplied by 100. Use the flag --mult to define a
different multiplier value.

By default, the result of a search iteration is considered significant if the
likelihood improvement is above 1e-4. Use the flag --delta to define a
different value.

By default, the search stops after five function evaluations, or the number of
parameters times two, without a significant improvement. Use the flag --eval
to define a different value.

By default, initial values for the parameters will be set at random. Use
--current flag to set the values to the current values as defined in the model
file. If the search is done with more than one iteration, then at each
iteration an slight random change on the current value will be used as the
starting point.

By default five different starting points will be attempted for the parameter
search in each tree. Use the flag --iter to define a different number.

By default, all available CPUs will be used in the processing. Set --cpu flag
to use a different number of CPUs.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var current bool
var numCPU int
var numEval int
var numIter int
var multLambda float64
var deltaFlag float64
var output string
var modelFile string

func setFlags(c *command.Command) {
	c.Flags().BoolVar(&current, "current", false, "")
	c.Flags().IntVar(&numCPU, "cpu", runtime.NumCPU(), "")
	c.Flags().IntVar(&numEval, "eval", 0, "")
	c.Flags().IntVar(&numIter, "iter", 5, "")
	c.Flags().Float64Var(&multLambda, "mult", 100, "")
	c.Flags().Float64Var(&deltaFlag, "delta", 1e-4, "")
	c.Flags().StringVar(&modelFile, "model", "", "")
	c.Flags().StringVar(&output, "output", "", "")
	c.Flags().StringVar(&output, "o", "", "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}
	if modelFile == "" {
		return c.UsageError("--model flag should be defined")
	}
	if deltaFlag <= 0 {
		deltaFlag = 1e-20
	}
	if numIter <= 0 {
		numIter = 1
	}

	p, err := project.Read(args[0])
	if err != nil {
		return err
	}

	tc, err := p.Trees()
	if err != nil {
		return err
	}

	landscape, err := p.Landscape(nil)
	if err != nil {
		return err
	}

	rot, err := p.StageRotation(landscape.Pixelation())
	if err != nil {
		return err
	}
	rot.SetUndefAsFix()

	stages, err := p.Stages(rot, landscape)
	if err != nil {
		return err
	}

	rc, err := p.Ranges(landscape.Pixelation())
	if err != nil {
		return err
	}

	tr, err := p.Traits()
	if err != nil {
		return err
	}

	// check if all terminals have defined ranges
	// and traits
	for _, tn := range tc.Names() {
		t := tc.Tree(tn)
		for _, term := range t.Terms() {
			if !rc.HasTaxon(term) {
				return fmt.Errorf("taxon %q of tree %q has no defined range", term, tn)
			}
			if len(tr.Obs(term)) == 0 {
				return fmt.Errorf("taxon %q of tree %q has no defined trait", term, tn)
			}
		}
	}

	keys, err := p.Keys()
	if err != nil {
		return err
	}

	mp, err := openModel(modelFile)
	if err != nil {
		return err
	}
	if !mp.IsScaled() {
		return fmt.Errorf("at least one movement parameter should be fixed at 1.0")
	}
	paramIDs := getParamIDs(mp)

	net := earth.NewNetwork(landscape.Pixelation())

	walk.StartDown(numCPU, landscape.Pixelation(), len(tr.States()))
	for _, tn := range tc.Names() {
		t := tc.Tree(tn)
		states := tr.States()
		fn := func(x []float64) float64 {
			fm, ok := fromParamToModel(x, mp, paramIDs, tr, keys)
			if !ok {
				return math.Inf(1)
			}

			dd := fm.Relaxed()
			discrete := catwalk.Cats(landscape.Pixelation(), net, fm.Lambda(), int(fm.Steps()), dd)
			mv := fm.Movement(tr, keys)
			st := fm.Settlement(tr, keys)
			landProb := make([]walker.Model, len(discrete))
			for i, c := range discrete {
				lp := walker.New(landscape, net, mv, st, c, states, keys)
				landProb[i] = lp
			}

			param := walk.Param{
				Landscape: landscape,
				Rot:       rot,
				Stages:    stages.Stages(),
				Ranges:    rc,
				Traits:    tr,
				Keys:      keys,
				Walker:    landProb,
				Steps:     int(fm.Steps()),
				Discrete:  discrete,
			}
			wt := walk.New(t, param)
			l := wt.DownPass()
			return -l
		}
		problem := optimize.Problem{
			Func: fn,
		}
		opts := &optimize.Settings{
			Converger: &funcConv{},
			Recorder: &logProgress{
				name:     tn,
				w:        c.Stdout(),
				mp:       mp,
				paramIDs: paramIDs,
			},
			Concurrent: 1,
		}
		bestLike := math.Inf(1)
		bestParam := make([]float64, len(paramIDs))
		for it := 0; it < numIter; it++ {
			fmt.Fprintf(c.Stdout(), "# iteration %d of %d\n", it, numIter)
			initX := make([]float64, len(paramIDs))
			for _, tp := range mp.Types() {
				for _, pn := range mp.Names(tp) {
					id := mp.ID(pn, tp)
					if id == 0 {
						continue
					}
					mx := mp.Max(pn, tp)
					if math.IsInf(mx, 0) {
						mx = 5
					}
					if current {
						v := mp.Val(pn, tp)
						if tp == model.Walk && pn == "lambda" {
							v /= multLambda
						}
						initX[paramIDs[id]] = v
						if it == 0 {
							continue
						}
						v = v + rand.NormFloat64()*v/10
						if v <= 0 || v > mx {
							continue
						}
						initX[paramIDs[id]] = v
					}
					initX[paramIDs[id]] = rand.Float64() * mx
				}
			}

			result, err := optimize.Minimize(problem, initX, opts, nil)
			if err != nil {
				return fmt.Errorf("when searching %q: %v", tn, err)
			}
			if result.Location.F < bestLike {
				bestLike = result.F
				copy(bestParam, result.X)
			}
		}
		best, ok := fromParamToModel(bestParam, mp, paramIDs, tr, keys)
		if !ok {
			return fmt.Errorf("tree %q: unable to converge", tn)
		}
		fmt.Fprintf(c.Stdout(), "# %s: best likelihood -%.6f\n", tn, bestLike)
		if err := saveModel(best, tn); err != nil {
			return fmt.Errorf("tree %q: %v", tn, err)
		}
	}
	walk.EndDown()
	return nil
}

func openModel(name string) (*model.Model, error) {
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

func saveModel(md *model.Model, tn string) (err error) {
	name := "model-ml-" + tn + ".tab"
	if output != "" {
		name = output + "-" + name
	}
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

	if err := md.TSV(f); err != nil {
		return fmt.Errorf("while writing on %q: %v", name, err)
	}
	return nil
}

func getParamIDs(mp *model.Model) map[int]int {
	IDs := mp.IDs()
	pID := make(map[int]int)
	for i, id := range IDs {
		pID[id] = i
	}
	return pID
}

func fromParamToModel(x []float64, mp *model.Model, paramIDs map[int]int, tr *trait.Data, keys *pixkey.PixKey) (*model.Model, bool) {
	fm := mp.Copy()
	for _, tp := range fm.Types() {
		for _, pn := range fm.Names(tp) {
			id := fm.ID(pn, tp)
			if id == 0 {
				continue
			}
			v := x[paramIDs[id]]
			// parameters outside the boundaries are rejected
			mx := fm.Max(pn, tp)
			if v > mx {
				if v-mx > deltaFlag {
					return nil, false
				}
				// if the value is inside the delta
				// we set the value with the maximum
				v = mx
			}
			if v <= 0 {
				return nil, false
			}
			fm.Update(pn, tp, v)
		}
	}
	if id := fm.ID("lambda", model.Walk); id != 0 {
		fm.Update("lambda", model.Walk, x[paramIDs[id]]*multLambda)
	}
	return fm, true
}

type logProgress struct {
	name     string
	w        io.Writer
	mp       *model.Model
	paramIDs map[int]int
	best     float64
}

func (lp *logProgress) Init() error {
	lp.best = math.Inf(1)
	return nil
}
func (lp *logProgress) Record(loc *optimize.Location, op optimize.Operation, stats *optimize.Stats) error {
	if op != optimize.MajorIteration {
		return nil
	}
	// only report improvements
	if loc.F >= lp.best {
		return nil
	}
	lp.best = loc.F

	fmt.Fprintf(lp.w, "# %s -%.6f [", lp.name, loc.F)

	lambdaID := lp.mp.ID("lambda", model.Walk)
	for i, v := range loc.X {
		if lambdaID != 0 && lp.paramIDs[lambdaID] == i {
			v *= multLambda
		}
		if i > 0 {
			fmt.Fprintf(lp.w, " ")
		}
		fmt.Fprintf(lp.w, "%.6f", v)
	}
	fmt.Fprintf(lp.w, "]\n")
	return nil
}

type funcConv struct {
	first   bool
	best    float64
	last    float64
	param   []float64
	iter    int
	maxIter int
}

func (fc *funcConv) Init(dim int) {
	fc.first = true
	fc.best = math.Inf(1)
	fc.last = math.Inf(1)
	fc.iter = 0
	fc.param = make([]float64, dim)
	fc.maxIter = max(dim*2, 5)
	if numEval > 0 {
		fc.maxIter = numEval
	}
}

func (fc *funcConv) Converged(l *optimize.Location) optimize.Status {
	f := l.F
	p := l.X
	if fc.first {
		fc.first = false
		fc.best = f
		fc.last = f
		copy(fc.param, p)
		fc.iter = 0
		return optimize.NotTerminated
	}

	// we ignore infinity values that most of them are generated
	// without a real function evaluation
	if math.IsInf(f, 1) {
		return optimize.NotTerminated
	}

	diff := math.Abs(f - fc.last)
	var maxDiff float64
	for i, cp := range fc.param {
		df := math.Abs(cp - p[i])
		if df > maxDiff {
			maxDiff = df
		}
	}
	if f < fc.best {
		fc.best = f
		copy(fc.param, p)
		if diff > deltaFlag || maxDiff > deltaFlag {
			fc.last = f
			fc.iter = 0
			return optimize.NotTerminated
		}
	}

	fc.iter++
	if fc.iter < fc.maxIter {
		return optimize.NotTerminated
	}
	return optimize.FunctionConvergence
}
