package graph

import (
	"reflect"
	"testing"
	"time"
)

func civil(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestJournalDate(t *testing.T) {
	tests := []struct {
		name string
		want time.Time
		ok   bool
	}{
		{"2026-05-25", civil(2026, 5, 25), true},
		{"2028-02-29", civil(2028, 2, 29), true},
		{"2027-02-29", time.Time{}, false},
		{"2026-13-01", time.Time{}, false},
		{"Alpha", time.Time{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := JournalDate(tt.name)
			if ok != tt.ok || !got.Equal(tt.want) {
				t.Errorf("JournalDate(%q) = (%v, %v), want (%v, %v)", tt.name, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestAddMonthsClamped(t *testing.T) {
	tests := []struct {
		day  time.Time
		n    int
		want time.Time
	}{
		{civil(2026, 3, 31), -1, civil(2026, 2, 28)},
		{civil(2028, 3, 31), -1, civil(2028, 2, 29)},
		{civil(2026, 1, 15), -1, civil(2025, 12, 15)},
		{civil(2025, 12, 31), 1, civil(2026, 1, 31)},
		{civil(2026, 1, 31), 1, civil(2026, 2, 28)},
		{civil(2026, 5, 26), 0, civil(2026, 5, 26)},
	}
	for _, tt := range tests {
		got := AddMonthsClamped(tt.day, tt.n)
		if !got.Equal(tt.want) {
			t.Errorf("AddMonthsClamped(%s, %d) = %s, want %s", tt.day.Format("2006-01-02"), tt.n, got.Format("2006-01-02"), tt.want.Format("2006-01-02"))
		}
	}
}

func TestOnThisDay(t *testing.T) {
	journals := []string{"2023-05-26", "2024-05-26", "2025-05-25", "2025-05-26", "2026-04-26", "2026-05-19", "2026-05-26", "2027-05-26", "notes"}
	want := []OnThisDayEntry{
		{"1 week ago", "2026-05-19"},
		{"1 month ago", "2026-04-26"},
		{"2025", "2025-05-26"},
		{"2024", "2024-05-26"},
		{"2023", "2023-05-26"},
	}
	if got := OnThisDay(civil(2026, 5, 26), journals); !reflect.DeepEqual(got, want) {
		t.Errorf("OnThisDay = %v, want %v", got, want)
	}
}

func TestOnThisDayMonthEnd(t *testing.T) {
	got := OnThisDay(civil(2026, 3, 31), []string{"2026-02-28", "2026-03-03", "2026-03-24"})
	want := []OnThisDayEntry{{"1 week ago", "2026-03-24"}, {"1 month ago", "2026-02-28"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("OnThisDay = %v, want %v", got, want)
	}
}

func TestOnThisDayLeapDay(t *testing.T) {
	journals := []string{"2024-02-29", "2027-02-28", "2027-02-29", "2028-01-29", "2028-02-22"}
	got := OnThisDay(civil(2028, 2, 29), journals)
	want := []OnThisDayEntry{{"1 week ago", "2028-02-22"}, {"1 month ago", "2028-01-29"}, {"2024", "2024-02-29"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("OnThisDay = %v, want %v", got, want)
	}
}

func TestOnThisDayNone(t *testing.T) {
	if got := OnThisDay(civil(2026, 5, 26), nil); got != nil {
		t.Errorf("OnThisDay(nil) = %v, want nil", got)
	}
}

func TestAppendLine(t *testing.T) {
	tests := []struct{ content, want string }{
		{"", "- x\n"},
		{"a\n", "a\n- x\n"},
		{"a", "a\n- x\n"},
		{"a\r\nb\r\n", "a\r\nb\r\n- x\r\n"},
		{"a\r\nb", "a\r\nb\r\n- x\r\n"},
		{"\n", "\n- x\n"},
	}
	for _, tt := range tests {
		if got := AppendLine(tt.content, "- x"); got != tt.want {
			t.Errorf("AppendLine(%q) = %q, want %q", tt.content, got, tt.want)
		}
	}
}
