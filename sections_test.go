package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSectionSettingsDefaultsAndOverrides(t *testing.T) {
	for _, c := range []struct {
		input string
		want  int
	}{
		{`{}`, 4},
		{`{"sections":[]}`, 0},
		{`{"sections":[{"id":"work","start":"09:00","end":"17:00"}]}`, 1},
	} {
		var config settings
		if err := json.Unmarshal([]byte(c.input), &config); err != nil {
			t.Fatal(err)
		}
		if err := config.prepareSections(); err != nil {
			t.Fatal(err)
		}
		if len(config.Sections) != c.want {
			t.Fatalf("sections for %s: got %d, want %d", c.input, len(config.Sections), c.want)
		}
		if c.want == 4 {
			for i, id := range []string{"overnight", "morning", "afternoon", "evening"} {
				now := time.Date(2026, 11, 20, i*6, 0, 0, 0, time.UTC)
				if got, ok := sectionAt(config.Sections, now); !ok || got.ID != id {
					t.Errorf("section at %s: got %q, want %q", now.Format("15:04"), got.ID, id)
				}
			}
		}
	}
}

func TestSectionSettings(t *testing.T) {
	morning := section{ID: "am", Start: "08:00", End: "12:00"}
	afternoon := section{ID: "pm", Name: "午後", Start: "12:00", End: "24:00"}
	sections := []section{afternoon, morning}
	if err := validateSections(sections); err != nil {
		t.Fatal(err)
	}
	if sections[0].ID != "am" || sections[0].Name != "am" {
		t.Fatalf("unexpected first section: %+v", sections[0])
	}
	noon := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	if s, ok := sectionAt(sections, noon); !ok || s.ID != "pm" {
		t.Fatalf("unexpected section at noon: %+v", s)
	}
	if _, ok := sectionAt(sections, noon.Add(-5*time.Hour)); ok {
		t.Fatal("found a section in an unconfigured gap")
	}

	for name, invalid := range map[string][]section{
		"duplicate": {morning, morning},
		"overlap":   {morning, {ID: "pm", Start: "11:59", End: "14:00"}},
		"overnight": {{ID: "night", Start: "22:00", End: "06:00"}},
		"format":    {{ID: "am", Start: "8:00", End: "12:00"}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateSections(invalid); err == nil {
				t.Fatal("accepted invalid sections")
			}
		})
	}
}

func TestSectionCapacityUsesFullEstimates(t *testing.T) {
	s := section{ID: "pm", Start: "13:00", End: "15:00"}
	now := time.Date(2026, 11, 20, 14, 10, 0, 0, time.UTC)
	minutes := 45.0
	end := now.Add(-20 * time.Minute)
	rows := []*task{
		{SectionID: "pm", State: "pending", Minutes: &minutes, Sessions: []session{
			{Start: end.Add(-10 * time.Minute), End: &end},
		}},
		{SectionID: "pm", State: "done", Minutes: &minutes},
		{SectionID: "pm", State: "pending"},
	}
	if got := sectionCapacity(s, rows); got != "Free 00:30:00 | 1 unestimated" {
		t.Errorf("capacity = %q", got)
	}
	rows = append(rows, &task{State: "running", Minutes: &minutes})
	if got := sectionCapacity(s, rows); got != "Over 00:15:00 | 1 unestimated" {
		t.Errorf("capacity over budget = %q", got)
	}
	night := section{Start: "18:00", End: "24:00"}
	if got := sectionCapacity(night, nil); got != "Free 06:00:00" {
		t.Errorf("empty section capacity = %q", got)
	}
}

func TestTaskSectionUsesPlacementOrFirstActualStart(t *testing.T) {
	sections := []section{
		{ID: "am", Start: "06:00", End: "12:00"},
		{ID: "pm", Start: "12:00", End: "18:00"},
	}
	now := time.Date(2026, 11, 20, 15, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	first := now.Add(-4 * time.Hour).UTC()
	work := task{State: "done", Sessions: []session{{Start: first}, {Start: now}}}
	if got := taskSection(&work, sections, now); got != "am" {
		t.Fatalf("unassigned completed task: got %q", got)
	}
	work.State = "running"
	if got := taskSection(&work, sections, now); got != "am" {
		t.Fatalf("unassigned running task: got %q", got)
	}
	work.SectionID = "pm"
	if got := taskSection(&work, sections, now); got != "pm" {
		t.Fatalf("assigned task: got %q", got)
	}
	work.SectionID, work.State = "", "pending"
	if got := taskSection(&work, sections, now); got != "" {
		t.Fatalf("unassigned paused task: got %q", got)
	}
}
