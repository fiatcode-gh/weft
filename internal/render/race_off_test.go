//go:build !race

package render

// sampleStride is the step through the generated documents of the exhaustive
// differential tests: 1, so without the race detector every document is checked.
const sampleStride = 1

// invalidateStride is the step through the 50 documents of the invalidation
// test: 1, so without the race detector every document is checked.
const invalidateStride = 1
