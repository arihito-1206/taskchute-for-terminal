package main

import (
	"testing"
	"time"
)

func TestRoutineRecurrence(t *testing.T) {
	tests := []struct {
		pattern, first string
		matches, skips []string
	}{
		{"weekday", "2026-11-20", []string{"2026-11-20", "2026-11-23"}, []string{"2026-11-19", "2026-11-21"}},
		{"holiday", "2026-11-20", []string{"2026-11-21", "2026-11-22"}, []string{"2026-11-20", "2026-11-23"}},
		{"daily/2", "2026-12-31", []string{"2026-12-31", "2027-01-02"}, []string{"2026-12-30", "2027-01-01"}},
		{"daily/2", "2026-03-07", []string{"2026-03-09"}, []string{"2026-03-08", "2026-03-10"}},
		{"mon,wed/2", "2026-12-30", []string{"2026-12-30", "2027-01-11", "2027-01-13"}, []string{"2026-12-28", "2027-01-04", "2027-01-06"}},
		{"weekday/2", "2026-12-30", []string{"2026-12-31", "2027-01-11"}, []string{"2027-01-04", "2027-01-10"}},
		{"holiday/2", "2027-01-03", []string{"2027-01-03", "2027-01-16"}, []string{"2027-01-02", "2027-01-09"}},
		{"monthly:1", "2026-12-31", []string{"2027-01-01", "2027-02-01"}, []string{"2026-12-01", "2027-01-02"}},
		{"monthly:31", "2026-01-30", []string{"2026-01-31", "2026-03-31"}, []string{"2026-02-28", "2026-04-30"}},
		{"monthly:2:tue/2", "2026-12-01", []string{"2026-12-08", "2027-02-09"}, []string{"2026-12-01", "2027-01-12"}},
		{"monthly:5:tue", "2026-12-01", []string{"2026-12-29", "2027-03-30"}, []string{"2027-01-26", "2027-02-23"}},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"/"+tt.first, func(t *testing.T) {
			r := routine{FirstDay: tt.first}
			if err := r.setRepeat(tt.pattern); err != nil {
				t.Fatal(err)
			}
			if label := r.repeatLabel(); label != tt.pattern {
				t.Fatalf("repeat label = %q, want %q", label, tt.pattern)
			}
			for _, date := range tt.matches {
				if !r.occursOn(routineDate(t, date)) {
					t.Errorf("did not match %s", date)
				}
			}
			for _, date := range tt.skips {
				if r.occursOn(routineDate(t, date)) {
					t.Errorf("unexpectedly matched %s", date)
				}
			}
		})
	}
}

func TestRoutineRecurrencePersistsLegacyDays(t *testing.T) {
	r := routine{Days: []string{"wed", "mon"}}
	if !r.occursOn(routineDate(t, "2026-11-23")) || r.occursOn(routineDate(t, "2026-11-24")) {
		t.Fatal("legacy days with no anchor must continue to repeat every week")
	}
	if err := r.setRepeat("daily/2"); err != nil || r.Repeat != "daily/2" || len(r.Days) != 0 {
		t.Fatalf("extended rule left legacy weekdays: %+v, %v", r, err)
	}
	if err := r.setRepeat("weekday"); err != nil || r.Repeat != "" || len(r.Days) != 5 {
		t.Fatalf("alias failed to restore legacy weekdays: %+v, %v", r, err)
	}
}

func TestRoutineRecurrenceRejectsInvalidPatterns(t *testing.T) {
	for _, value := range []string{"", "tues", "mon,", "daily/0", "daily/-1", "daily/+2", "daily//2", "mon/", "monthly:0", "monthly:32", "monthly:6:tue", "monthly:1:day", "monthly:2:tue:extra"} {
		if _, err := parseRepeat(value); err == nil {
			t.Errorf("accepted invalid repeat %q", value)
		}
	}
}

func TestEditingLegacyRoutineEstablishesMissingRecurrenceAnchor(t *testing.T) {
	const day = "2026-11-20"
	now := routineDate(t, day)
	selected := &task{ID: "existing", Day: day, RoutineID: "r1", State: "pending", SectionID: "morning"}
	s := &state{Tasks: []*task{selected}, Order: []string{"r1"}, Routines: map[string]routine{
		"r1": {ID: "r1", Days: []string{"fri"}, WorkCode: "PJ1", SectionID: "evening"},
	}}
	if err := s.apply(day, "routine", []string{selected.ID, "daily/2"}, now, nil); err != nil {
		t.Fatal(err)
	}
	if s.Routines["r1"].FirstDay != day || s.Routines["r1"].repeatLabel() != "daily/2" {
		t.Fatalf("routine has the wrong recurrence: %+v", s.Routines["r1"])
	}
	s.fill(day)
	s.fill("2026-11-21")
	s.fill("2026-11-22")
	if len(s.Tasks) != 2 || selected.SectionID != "morning" {
		t.Fatal("recurrence edit replaced an existing task or filled a skipped day")
	}
	generated := s.Tasks[1]
	if generated.Day != "2026-11-22" || generated.SectionID != "evening" || generated.WorkCode != "PJ1" {
		t.Fatalf("generated task did not inherit routine defaults: %+v", generated)
	}
}

func routineDate(t *testing.T, value string) time.Time {
	t.Helper()
	d, err := time.Parse(dateLayout, value)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
