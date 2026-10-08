//go:build race

package render

// sampleStride: the race detector slows the Glamour renders these tests are
// made of by an order of magnitude, so the generated corpus is sampled more
// sparsely. Coprime to 60, like the non-race stride.
const sampleStride = 17

// invalidateStride is the step through the 50 documents of the invalidation
// test, each of which takes 20 seeded edits; coprime to 4 and 5 so CRLF and
// padded documents are still visited.
const invalidateStride = 7
