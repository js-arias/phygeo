// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package speed

import (
	"fmt"
	"image/color"

	"github.com/js-arias/earth"
	"github.com/js-arias/phygeo/timestage"
	"github.com/js-arias/timetree"
	"golang.org/x/exp/slices"
	"gonum.org/v1/gonum/stat"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
)

// A speedTimePlot is a plot of the speed at each time stage
type speedTimePlot struct {
	speed, max, min map[int64]float64
	style           draw.LineStyle
}

// DataRange implements the plot.DataRanger interface.
func (tp *speedTimePlot) DataRange() (xMin, xMax, yMin, yMax float64) {
	ages := make([]int64, 0, len(tp.speed))
	for a, s := range tp.max {
		ages = append(ages, a)
		if s > yMax {
			yMax = s
		}
		if s < yMin && s > 0 {
			yMin = s
		}
	}
	slices.Sort(ages)

	return float64(ages[0]) / timestage.MillionYears, float64(ages[len(ages)-1])/timestage.MillionYears + 5, 0, yMax
}

// Plot implements the plot.Plotter interface.
func (tp *speedTimePlot) Plot(c draw.Canvas, plt *plot.Plot) {
	ages := make([]int64, 0, len(tp.speed))
	for a := range tp.max {
		ages = append(ages, a)
	}
	slices.Sort(ages)

	trX, trY := plt.Transforms(&c)

	for i, a := range ages {
		x := trX(float64(a) / timestage.MillionYears)
		next := float64(a)/timestage.MillionYears + 5
		if i < len(ages)-1 {
			next = float64(ages[i+1]) / timestage.MillionYears
		}

		pts := []vg.Point{
			{X: x, Y: trY(tp.max[a])},
			{X: trX(next), Y: trY(tp.max[a])},
			{X: trX(next), Y: trY(tp.min[a])},
			{X: x, Y: trY(tp.min[a])},
			{X: x, Y: trY(tp.max[a])},
		}
		c.FillPolygon(color.RGBA{127, 188, 165, 255}, pts)
	}

	c.SetLineStyle(tp.style)
	var p vg.Path
	for i, a := range ages {
		x := trX(float64(a) / timestage.MillionYears)
		y := trY(tp.speed[a])
		if i == 0 {
			p.Move(vg.Point{X: x, Y: y})
		} else {
			p.Line(vg.Point{X: x, Y: y})
		}

		next := float64(a)/timestage.MillionYears + 5
		if i < len(ages)-1 {
			next = float64(ages[i+1]) / timestage.MillionYears
		}
		p.Line(vg.Point{X: trX(next), Y: y})
	}
	c.Stroke(p)
}

func timeSpeedPlot(t *timetree.Tree, ts *treeSlice) error {
	p := plot.New()
	p.X.Label.Text = "age (Ma)"
	p.Y.Label.Text = "speed (km/My)"

	spp := &speedTimePlot{
		speed: make(map[int64]float64, len(ts.timeSlices)),
		min:   make(map[int64]float64, len(ts.timeSlices)),
		max:   make(map[int64]float64, len(ts.timeSlices)),
		style: plotter.DefaultLineStyle,
	}

	for a, s := range ts.timeSlices {
		dist := make([]float64, 0, len(s.distances))
		weights := make([]float64, 0, len(s.distances))
		for _, d := range s.distances {
			dist = append(dist, d*earth.Radius/1000)
			weights = append(weights, 1.0)
		}
		slices.Sort(dist)

		d := stat.Quantile(0.5, stat.Empirical, dist, weights)
		sp := d / s.sumBrLen

		spp.speed[a] = sp
		spp.max[a] = stat.Quantile(0.975, stat.Empirical, dist, weights) / s.sumBrLen
		spp.min[a] = stat.Quantile(0.025, stat.Empirical, dist, weights) / s.sumBrLen
	}

	p.Add(spp)
	if err := p.Save(6*vg.Inch, 4*vg.Inch, fmt.Sprintf("%s-%s-nodes-box.png", plotPrefix, t.Name())); err != nil {
		return err
	}
	return nil
}
