package graph

import "testing"

func TestParseStampLine(t *testing.T) {
	valid := []struct {
		name string
		line string
		want StampLine
	}{
		{"bare date", "SCHEDULED: <2026-05-20>",
			StampLine{Kind: StampScheduled, Start: 0, End: 23, Date: "2026-05-20"}},
		{"weekday", "  DEADLINE: <2026-05-25 Mon>",
			StampLine{Kind: StampDeadline, Start: 2, End: 28, Date: "2026-05-25", Weekday: "Mon"}},
		{"weekday and time", "SCHEDULED: <2026-05-25 Mon 09:30>",
			StampLine{Kind: StampScheduled, Start: 0, End: 33, Date: "2026-05-25", Weekday: "Mon", Time: "09:30"}},
		{"repeater +1w", "DEADLINE: <2026-06-01 Mon +1w>",
			StampLine{Kind: StampDeadline, Start: 0, End: 30, Date: "2026-06-01", Weekday: "Mon", Repeater: "+1w"}},
		{"repeater .+1d with time", "DEADLINE: <2026-06-01 Mon 10:00 .+1d>",
			StampLine{Kind: StampDeadline, Start: 0, End: 37, Date: "2026-06-01", Weekday: "Mon", Time: "10:00", Repeater: ".+1d"}},
		{"repeater ++2m", "SCHEDULED: <2026-06-01 ++2m>",
			StampLine{Kind: StampScheduled, Start: 0, End: 28, Date: "2026-06-01", Repeater: "++2m"}},
		{"mismatched weekday", "SCHEDULED: <2026-05-20 Fri>",
			StampLine{Kind: StampScheduled, Start: 0, End: 27, Date: "2026-05-20", Weekday: "Fri"}},
		{"trailing CR", "SCHEDULED: <2026-05-20 Wed>\r",
			StampLine{Kind: StampScheduled, Start: 0, End: 27, Date: "2026-05-20", Weekday: "Wed"}},
		{"tab indent", "\t\tDEADLINE: <2026-05-20>",
			StampLine{Kind: StampDeadline, Start: 2, End: 24, Date: "2026-05-20"}},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseStampLine(tc.line)
			if !ok {
				t.Fatalf("ParseStampLine(%q) not ok", tc.line)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}

	for _, line := range []string{
		"SCHEDULED: <2026-02-30>",
		"SCHEDULED: <2026-5-01>",
		"SCHEDULED: <2026-05-01 25:00>",
		"SCHEDULED: <2026-05-01 9:30>",
		"SCHEDULED: <2026-05-01 23:60>",
		"SCHEDULED: <2026-05-01",
		"SCHEDULED: <2026-05-01> trailing",
		"scheduled: <2026-05-01>",
		"SCHEDULED:<2026-05-25>",
		"SCHEDULED: <2026-05-25> x",
		"SCHEDULED: <2026-05-25 +1q>",
		"just text",
		"",
	} {
		if got, ok := ParseStampLine(line); ok {
			t.Errorf("ParseStampLine(%q) = %+v, want not ok", line, got)
		}
	}
}

func TestStampLineStamp(t *testing.T) {
	s := StampLine{Kind: StampDeadline, Date: "2026-05-25", Weekday: "Mon", Time: "17:00", Repeater: "+1w"}
	if got, want := s.Stamp(), (Stamp{Date: "2026-05-25", Time: "17:00"}); got != want {
		t.Errorf("Stamp() = %+v, want %+v", got, want)
	}
	if StampScheduled.Keyword() != "SCHEDULED" || StampDeadline.Keyword() != "DEADLINE" {
		t.Error("Keyword mismatch")
	}
}
