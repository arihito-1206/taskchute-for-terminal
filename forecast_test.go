package main

import (
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestForecastFollowsFullDayOrderAcrossListViews(t *testing.T) {
	now := time.Date(2026, 11, 26, 14, 10, 0, 0, time.UTC)
	day := now.Format(dateLayout)
	activeEstimate, earlierEstimate, laterEstimate := 30.0, 15.0, 20.0
	m := terminal{
		now: now, day: day, list: true, height: 40,
		app: application{sections: []section{
			{ID: "earlier", Name: "Earlier", Start: "09:00", End: "13:00"},
			{ID: "current", Name: "Current", Start: "13:00", End: "18:00"},
		}},
		data: &state{Tasks: []*task{
			{ID: "b", Day: day, State: "pending", Title: "Unestimated", SectionID: "current"},
			{ID: "c", Day: day, State: "pending", Title: "Later", SectionID: "current", Minutes: &laterEstimate},
			{ID: "a", Day: day, State: "pending", Title: "EarlierTask", SectionID: "earlier", Minutes: &earlierEstimate},
			{ID: "r", Day: day, State: "running", Title: "Active", Minutes: &activeEstimate,
				Sessions: []session{{Start: now.Add(-20 * time.Minute)}}},
		}},
	}
	for _, view := range []struct {
		name, section, filter string
		plan                  bool
		want                  map[string]string
	}{
		{name: "list", want: map[string]string{"EarlierTask": "~14:20", "Unestimated": "~14:35", "Later": "—"}},
		{name: "plan", plan: true, want: map[string]string{"EarlierTask": "~14:20", "Unestimated": "~14:35", "Later": "—"}},
		{name: "section", section: "current", want: map[string]string{"Unestimated": "~14:35", "Later": "—"}},
		{name: "search", filter: "Unestimated", want: map[string]string{"Unestimated": "~14:35"}},
	} {
		t.Run(view.name, func(t *testing.T) {
			model := m
			model.sectionID, model.filter, model.planning = view.section, view.filter, view.plan
			rendered := ansi.Strip(model.render(100))
			if !strings.Contains(rendered, "FORECAST") || !strings.Contains(rendered, "START") {
				t.Fatal(rendered)
			}
			got := make(map[string]string)
			for _, line := range strings.Split(rendered, "\n") {
				fields := strings.Fields(strings.TrimPrefix(line, ">"))
				if len(fields) >= 7 && fields[0] == "Pending" {
					got[fields[1]] = fields[3]
					if fields[4] != "-" {
						t.Fatalf("forecast leaked into actual START: %s", line)
					}
				}
			}
			if !maps.Equal(got, view.want) {
				t.Fatalf("forecasts = %v, want %v", got, view.want)
			}
		})
	}
	if strings.Contains(m.render(77), "FORECAST") {
		t.Fatal("forecast took space from the actual-time columns on a narrow terminal")
	}
	m.day = "2026-11-27"
	if strings.Contains(m.render(100), "FORECAST") {
		t.Fatal("forecast shown on a future date")
	}
}

func TestExpectedStartsWhenRunningEstimateIsUnknownOrExhausted(t *testing.T) {
	now := time.Date(2026, 11, 26, 23, 55, 0, 0, time.UTC)
	pending := &task{ID: "next", State: "pending"}
	running := &task{State: "running", Sessions: []session{{Start: now.Add(-10 * time.Minute)}}}
	for _, estimate := range []*float64{nil, new(float64)} {
		running.Minutes = estimate
		if len(expectedStarts([]*task{pending}, running, now)) != 0 {
			t.Fatal("estimated a start while the running task had unknown remaining time")
		}
	}
	estimate := 30.0
	running.Minutes = &estimate
	starts := expectedStarts([]*task{pending}, running, now)
	if got := timeLabel(starts[pending.ID], "2026-11-26", time.UTC); got != "11/27 00:15" {
		t.Fatalf("midnight rollover = %q", got)
	}
}
