// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package cats implements discrete categories
// from a continuous probability distribution function.
// Each category is expected to have the same probability.
package cats

import (
	"fmt"

	"gonum.org/v1/gonum/stat/distuv"
)

// Discrete is a discrete category distribution.
type Discrete interface {
	// Cats returns the values of the different categories.
	Cats() []float64

	// String output for the function name and parameters.
	String() string
}

// Gamma is a discretized Gamma distribution.
type Gamma struct {
	// Parameters of the gamma distribution.
	Param distuv.Gamma

	// Number of categories
	NumCat int
}

// Cats returns the values for a Gamma distribution
// discretized in equal probability categories.
func (g Gamma) Cats() []float64 {
	return getCats(g.Param, g.NumCat)
}

// String output for the function name and parameters.
func (g Gamma) String() string {
	return fmt.Sprintf("gamma=%.6f", g.Param.Alpha)
}

// LogNormal is a discretized LogNormal distribution.
type LogNormal struct {
	// Parameters of the log normal distribution
	Param distuv.LogNormal

	// Number of categories
	NumCat int
}

// Cats return the values for a log Normal distribution
// discretized in equal probability categories.
func (ln LogNormal) Cats() []float64 {
	return getCats(ln.Param, ln.NumCat)
}

// String output for the function name and parameters.
func (ln LogNormal) String() string {
	return fmt.Sprintf("logNormal=%.6f", ln.Param.Sigma)
}

// Quantiler is a interfaces for distributions
// with a Quantile function
// (the inverse of the CDF function).
type quantiler interface {
	Quantile(p float64) float64
}

func getCats(q quantiler, n int) []float64 {
	cats := make([]float64, n)
	for i := range cats {
		p := (float64(i) + 0.5) / float64(n)
		cats[i] = q.Quantile(p)
	}
	return cats
}
