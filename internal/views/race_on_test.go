//go:build race

package views

// raceFactor scales wall-clock bounds: the race detector slows the editor
// by roughly this much.
const raceFactor = 8
