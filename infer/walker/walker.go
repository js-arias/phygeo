// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package walker defines the walker interface
// and implement some walk models.
package walker

// StageProb contains the data for the movement probability
// and pixel priors
// at a particular time stage.
type StageProb struct {
	// Move contains an slice of movement pixel probabilities
	// associated to a given pixel.
	Move [][]PixProb

	// Prior contains the prior probability of a pixel in a pixelation
	Prior []float64
}

// PixProb contains the probability of a pixel.
type PixProb struct {
	ID   int     // ID of the pixel in the underlying pixelation
	Prob float64 // the probability
}

// Model is an interface for an object that provide
// the movement probabilities for a landscape model
// in a random walk.
//
// The Model should be safe for concurrent access.
//
// The model is responsible for any preprocessing
// related to building the stage probabilities.
type Model interface {
	// StageProb retrieves the pixel probabilities
	// of a given time,
	// and a particular trait ID.
	StageProb(age int64, trait int) StageProb

	// Traits returns the traits defined for the model.
	Traits() []string
}
