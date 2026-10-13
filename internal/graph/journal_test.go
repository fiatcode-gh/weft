package graph

import (
	"reflect"
	"testing"
	"time"
)

func civil(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
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

// jr builds a journal PageMeta for the OnThisDay tests.
func jr(y int, m time.Month, d int) PageMeta {
	day := civil(y, m, d)
	return PageMeta{Name: day.Format("2006_01_02"), IsJournal: true, Date: day}
}

func TestOnThisDay(t *testing.T) {
	journals := []PageMeta{jr(2023, 5, 26), jr(2024, 5, 26), jr(2025, 5, 25), jr(2025, 5, 26), jr(2026, 4, 26), jr(2026, 5, 19), jr(2026, 5, 26), jr(2027, 5, 26)}
	want := []OnThisDayEntry{
		{"1 week ago", jr(2026, 5, 19)},
		{"1 month ago", jr(2026, 4, 26)},
		{"2025", jr(2025, 5, 26)},
		{"2024", jr(2024, 5, 26)},
		{"2023", jr(2023, 5, 26)},
	}
	if got := OnThisDay(civil(2026, 5, 26), journals); !reflect.DeepEqual(got, want) {
		t.Errorf("OnThisDay = %v, want %v", got, want)
	}
}

func TestOnThisDayMonthEnd(t *testing.T) {
	got := OnThisDay(civil(2026, 3, 31), []PageMeta{jr(2026, 2, 28), jr(2026, 3, 3), jr(2026, 3, 24)})
	want := []OnThisDayEntry{{"1 week ago", jr(2026, 3, 24)}, {"1 month ago", jr(2026, 2, 28)}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("OnThisDay = %v, want %v", got, want)
	}
}

func TestOnThisDayLeapDay(t *testing.T) {
	journals := []PageMeta{jr(2024, 2, 29), jr(2027, 2, 28), jr(2028, 1, 29), jr(2028, 2, 22)}
	got := OnThisDay(civil(2028, 2, 29), journals)
	want := []OnThisDayEntry{{"1 week ago", jr(2028, 2, 22)}, {"1 month ago", jr(2028, 1, 29)}, {"2024", jr(2024, 2, 29)}}
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
