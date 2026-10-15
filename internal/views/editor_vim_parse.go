package views

import (
	"unicode"
	"unicode/utf8"
)

// vimCmd is one parsed normal-mode command: count 0 means none; op is the
// operator ("" for a bare motion or command); key the motion, object or
// command; arg the character of f F t T r.
type vimCmd struct {
	count        int
	op, key, arg string
}

// vimParse is how far the typed tokens got.
type vimParse int

const (
	vimMore vimParse = iota // a valid prefix: wait for more keys
	vimDone                 // a complete command
	vimBad                  // cannot be a command: drop it
)

// vimMaxCount is the count a longer run of digits saturates at.
const vimMaxCount = 99999

// vimMotionKeys are the motions that take no further token. `gg` is
// reached through the g prefix and f F t T through their argument.
var vimMotionKeys = map[string]bool{
	"h": true, "j": true, "k": true, "l": true,
	"w": true, "b": true, "e": true, "W": true, "B": true, "E": true,
	"0": true, "^": true, "$": true, "G": true,
	";": true, ",": true, "%": true, "{": true, "}": true,
	"ctrl+d": true, "ctrl+u": true,
}

// vimSimple are the commands that take no further token. Later tasks add
// tables for operators, prefixes and arguments beside it.
var vimSimple = map[string]bool{
	"i": true, "a": true, "I": true, "A": true, "o": true, "O": true,
	"u": true, "ctrl+r": true,
	"x": true, "X": true, "s": true, "S": true, "J": true,
	"p": true, "P": true, "D": true, "C": true, ".": true,
	"v": true, "V": true,
}

// vimOperators are the keys that take a motion, an object or themselves.
var vimOperators = map[string]bool{"d": true, "c": true, "y": true, ">": true, "<": true}

// vimVisualSimple are the visual-mode commands that take no further token:
// the operators act on the selection at once.
var vimVisualSimple = map[string]bool{
	"o": true, "v": true, "V": true,
	"d": true, "x": true, "c": true, "y": true, ">": true, "<": true,
	"J": true, "~": true, "u": true, "U": true,
}

// vimObjectOpener maps the character after i or a to the key an object is
// stored under: the brackets are named by their opener.
var vimObjectOpener = map[string]string{
	"w": "w", "W": "W", `"`: `"`, "'": "'", "p": "p",
	"(": "(", ")": "(", "b": "(", "[": "[", "]": "[", "{": "{", "}": "{",
}

// vimFindKeys are the motions that take one character token.
var vimFindKeys = map[string]bool{"f": true, "F": true, "t": true, "T": true}

// vimArgToken reports whether tok is one printable character, the argument of
// f F t T.
func vimArgToken(tok string) bool {
	r, n := utf8.DecodeRuneInString(tok)
	return n == len(tok) && r != utf8.RuneError && !unicode.IsControl(r)
}

// vimDigits reads a run of count digits from tokens[i:] into *n and returns
// the index after it. A 0 continues a count but never starts one.
func vimDigits(tokens []string, i int, n *int) int {
	for ; i < len(tokens); i++ {
		t := tokens[i]
		if len(t) != 1 || t[0] < '0' || t[0] > '9' || (t[0] == '0' && *n == 0) {
			break
		}
		*n = min(*n*10+int(t[0]-'0'), vimMaxCount)
	}
	return i
}

// vimMotionTokens reads the motion at tokens[i:]: a one-token motion, `gg`
// or a find with its character. next is the index after it.
func vimMotionTokens(tokens []string, i int) (key, arg string, next int, st vimParse) {
	t := tokens[i]
	i++
	switch {
	case t == "g":
		if i == len(tokens) {
			return "", "", i, vimMore
		}
		if tokens[i] != "g" {
			return "", "", i, vimBad
		}
		return "gg", "", i + 1, vimDone
	case vimFindKeys[t]:
		if i == len(tokens) {
			return "", "", i, vimMore
		}
		if !vimArgToken(tokens[i]) {
			return "", "", i, vimBad
		}
		return t, tokens[i], i + 1, vimDone
	case vimMotionKeys[t]:
		return t, "", i, vimDone
	}
	return "", "", i, vimBad
}

// vimOperand reads what follows operator op at tokens[i:]: the operator
// again, a text object or a motion. ctrl+d and ctrl+u scroll; no operator
// takes them.
func vimOperand(tokens []string, i int, op string) (key, arg string, next int, st vimParse) {
	t := tokens[i]
	switch {
	case t == op:
		return op, "", i + 1, vimDone
	case t == "i" || t == "a":
		if i+1 == len(tokens) {
			return "", "", i + 1, vimMore
		}
		open, ok := vimObjectOpener[tokens[i+1]]
		if !ok {
			return "", "", i + 1, vimBad
		}
		return t + open, "", i + 2, vimDone
	case t == "ctrl+d" || t == "ctrl+u":
		return "", "", i + 1, vimBad
	}
	return vimMotionTokens(tokens, i)
}

// parseVim reads the tokens typed so far. visual selects the visual-mode
// table.
func parseVim(tokens []string, visual bool) (vimCmd, vimParse) {
	var c vimCmd
	i := vimDigits(tokens, 0, &c.count)
	if i == len(tokens) {
		return vimCmd{}, vimMore
	}
	var st vimParse
	switch t := tokens[i]; {
	case visual && vimVisualSimple[t]:
		c.key, i, st = t, i+1, vimDone
	case visual:
		c.key, c.arg, i, st = vimMotionTokens(tokens, i)
	case vimOperators[t]:
		c.op = t
		var n int
		i = vimDigits(tokens, i+1, &n)
		if n > 0 {
			c.count = min(max(c.count, 1)*n, vimMaxCount)
		}
		if i == len(tokens) {
			return vimCmd{}, vimMore
		}
		c.key, c.arg, i, st = vimOperand(tokens, i, t)
	case t == "r":
		if i+1 == len(tokens) {
			return vimCmd{}, vimMore
		}
		if !vimArgToken(tokens[i+1]) {
			return vimCmd{}, vimBad
		}
		c.key, c.arg, i, st = t, tokens[i+1], i+2, vimDone
	case vimSimple[t]:
		c.key, i, st = t, i+1, vimDone
	default:
		c.key, c.arg, i, st = vimMotionTokens(tokens, i)
	}
	if st != vimDone {
		return vimCmd{}, st
	}
	if i != len(tokens) {
		return vimCmd{}, vimBad
	}
	return c, vimDone
}
