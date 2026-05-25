package views

import (
	"testing"

	"github.com/charmbracelet/x/exp/teatest"
)

func TestTodosDashboardAllFilter(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	td := NewTodos(loadFixture(t), 100)
	teatest.RequireEqualOutput(t, []byte(td.View()))
}

func TestTodosDashboardLaterFilter(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	td := NewTodos(loadFixture(t), 100)
	td.Update("t") // → TODO
	td.Update("t") // → LATER
	teatest.RequireEqualOutput(t, []byte(td.View()))
}
