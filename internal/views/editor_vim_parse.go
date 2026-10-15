package views

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

// vimSimple are the commands that take no further token. Later tasks add
// tables for counts, operators, prefixes and arguments beside it.
var vimSimple = map[string]bool{
	"h": true, "l": true,
	"i": true, "a": true, "I": true, "A": true, "o": true, "O": true,
	"u": true, "ctrl+r": true,
}

// parseVim reads the tokens typed so far. visual selects the visual-mode
// table.
func parseVim(tokens []string, visual bool) (vimCmd, vimParse) {
	if len(tokens) == 0 {
		return vimCmd{}, vimMore
	}
	if len(tokens) == 1 && vimSimple[tokens[0]] {
		return vimCmd{key: tokens[0]}, vimDone
	}
	return vimCmd{}, vimBad
}
