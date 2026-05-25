package views

import (
	"testing"

	"github.com/charmbracelet/x/exp/teatest"
)

func TestTodosDashboardAllFilter(t *testing.T) {
	td := NewTodos(loadFixture(t))
	teatest.RequireEqualOutput(t, []byte(td.View()))
}

func TestTodosDashboardLaterFilter(t *testing.T) {
	td := NewTodos(loadFixture(t))
	td.Update("t") // → TODO
	td.Update("t") // → LATER
	teatest.RequireEqualOutput(t, []byte(td.View()))
}
