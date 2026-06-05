package views

// Key strings shared by the App key handler and the overlay Update methods.
// These mirror the tea.KeyMsg.String() values Bubble Tea produces. Only keys
// used in more than one file live here; single-use keys stay as literals at
// their call site.
const (
	keyEsc       = "esc"
	keyEnter     = "enter"
	keyUp        = "up"
	keyDown      = "down"
	keyE         = "e"
	keyK         = "k"
	keyJ         = "j"
	keyCtrlK     = "ctrl+k"
	keyCtrlJ     = "ctrl+j"
	keyQ         = "q"
	keySpace     = "space"
	keyBackspace = "backspace"
)
