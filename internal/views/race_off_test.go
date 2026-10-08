//go:build !race

package views

const raceFactor = 1

// raceDetector is whether the test binary is race-instrumented.
const raceDetector = false

// sweepStride is the average step between the scroll offsets an exhaustive
// sweep visits.
const sweepStride = 8
