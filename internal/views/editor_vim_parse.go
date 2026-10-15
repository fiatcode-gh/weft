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
}

// vimFindKeys are the motions that take one character token.
var vimFindKeys = map[string]bool{"f": true, "F": true, "t": true, "T": true}

// vimArgToken reports whether tok is one printable character, the argument of
// f F t T.
func vimArgToken(tok string) bool {
	r, n := utf8.DecodeRuneInString(tok)
	return n == len(tok) && r != utf8.RuneError && !unicode.IsControl(r)
}

// parseVim reads the tokens typed so far. visual selects the visual-mode
// table.
func parseVim(tokens []string, visual bool) (vimCmd, vimParse) {
	var c vimCmd
	i := 0
	for ; i < len(tokens); i++ {
		t := tokens[i]
		if len(t) != 1 || t[0] < '0' || t[0] > '9' || (t[0] == '0' && c.count == 0) {
			break
		}
		c.count = min(c.count*10+int(t[0]-'0'), vimMaxCount)
	}
	if i == len(tokens) {
		return vimCmd{}, vimMore
	}
	t := tokens[i]
	i++
	switch {
	case t == "g":
		if i == len(tokens) {
			return vimCmd{}, vimMore
		}
		if tokens[i] != "g" {
			return vimCmd{}, vimBad
		}
		c.key = "gg"
		i++
	case vimFindKeys[t]:
		if i == len(tokens) {
			return vimCmd{}, vimMore
		}
		if !vimArgToken(tokens[i]) {
			return vimCmd{}, vimBad
		}
		c.key, c.arg = t, tokens[i]
		i++
	case vimMotionKeys[t], vimSimple[t]:
		c.key = t
	default:
		return vimCmd{}, vimBad
	}
	if i != len(tokens) {
		return vimCmd{}, vimBad
	}
	return c, vimDone
}
