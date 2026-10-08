//go:build !race

package render

// sampleStride is the step through the generated documents of the exhaustive
// differential tests. It is coprime to 60, so the sampled indices still cycle
// through every widths × CRLF × padding combination the generator assigns by
// index (i%3, i%4, i%5).
const sampleStride = 7

// invalidateStride is the step through the 50 documents of the invalidation
// test, each of which takes 20 seeded edits; coprime to 4 and 5 so CRLF and
// padded documents are still visited.
const invalidateStride = 3
