package views

import "math/rand/v2"

// sweepOffsets picks the scroll offsets of 0..last that an exhaustive sweep
// visits: the first and the last always, every other with probability
// 1/sweepStride from a generator seeded with seed, so a run visits the same
// offsets every time. The offsets are those of one page at one width; the sweep
// keeps every page, width and look, and only skips offsets in between.
func sweepOffsets(last int, seed uint64) []int {
	return sampleRange(last, seed, sweepStride)
}

// sampleRange is sweepOffsets with an explicit stride, for sweeps over a short
// range that a stride of sweepStride would leave with only its two ends.
func sampleRange(last int, seed uint64, stride int) []int {
	rng := rand.New(rand.NewPCG(seed, 0x5eed))
	var offs []int
	for off := range last + 1 {
		if off == 0 || off == last || rng.IntN(stride) == 0 {
			offs = append(offs, off)
		}
	}
	return offs
}
