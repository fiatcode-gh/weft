//go:build race

package views

// raceFactor scales wall-clock bounds: the race detector slows the editor
// by roughly this much.
const raceFactor = 8

// raceDetector is whether the test binary is race-instrumented.
const raceDetector = true

// sweepStride is the average step between the scroll offsets an exhaustive
// sweep visits; sparser than without the detector, which slows each visit by
// an order of magnitude.
const sweepStride = 16
