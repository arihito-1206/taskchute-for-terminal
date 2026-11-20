package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

type section struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Start string `json:"start"`
	End   string `json:"end"`
}

var sectionIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

func validateSections(sections []section) error {
	ids := make(map[string]bool, len(sections))
	for i := range sections {
		s := &sections[i]
		if !sectionIDPattern.MatchString(s.ID) {
			return fmt.Errorf("section ID %q must start with a letter or digit and use only letters, digits, - or _", s.ID)
		}
		if ids[s.ID] {
			return fmt.Errorf("duplicate section ID %q", s.ID)
		}
		ids[s.ID] = true
		s.Name = strings.TrimSpace(s.Name)
		if s.Name == "" {
			s.Name = s.ID
		}
		if !validSectionTime(s.Start) || (s.End != "24:00" && !validSectionTime(s.End)) || s.Start >= s.End {
			return fmt.Errorf("section %q needs increasing HH:MM start and end times (24:00 allowed as end)", s.ID)
		}
	}
	slices.SortFunc(sections, func(a, b section) int { return strings.Compare(a.Start, b.Start) })
	for i := 1; i < len(sections); i++ {
		if sections[i].Start < sections[i-1].End {
			return fmt.Errorf("sections %q and %q overlap", sections[i-1].ID, sections[i].ID)
		}
	}
	return nil
}

func validSectionTime(value string) bool {
	t, err := time.Parse("15:04", value)
	return err == nil && t.Format("15:04") == value
}

func sectionByID(sections []section, id string) (section, bool) {
	for _, s := range sections {
		if s.ID == id {
			return s, true
		}
	}
	return section{}, false
}

func sectionAt(sections []section, now time.Time) (section, bool) {
	clock := now.Format("15:04")
	for _, s := range sections {
		if s.Start <= clock && clock < s.End {
			return s, true
		}
	}
	return section{}, false
}

func sectionEnded(s section, day string, now time.Time) bool {
	today := now.Format(dateLayout)
	return day < today || (day == today && now.Format("15:04") >= s.End)
}

func checkSectionPlacement(t *task, destination string, sections []section, now time.Time) error {
	if destination != taskSection(t, sections, now) {
		if s, ok := sectionByID(sections, destination); ok && sectionEnded(s, t.Day, now) {
			return fmt.Errorf("section %q has already ended", s.Name)
		}
	}
	return nil
}

func taskSection(t *task, sections []section, now time.Time) string {
	if t.SectionID != "" {
		if s, ok := sectionByID(sections, t.SectionID); ok {
			return s.ID
		}
		return ""
	}
	if t.State != "running" && t.State != "done" {
		return ""
	}
	var first time.Time
	for _, session := range t.Sessions {
		if !session.Start.IsZero() && (first.IsZero() || session.Start.Before(first)) {
			first = session.Start
		}
	}
	if first.IsZero() {
		return ""
	}
	s, _ := sectionAt(sections, first.In(now.Location()))
	return s.ID
}

func sectionCapacity(s section, rows []*task) string {
	free := float64(sectionMinute(s.End) - sectionMinute(s.Start))
	unknown := 0
	for _, t := range rows {
		if t.Minutes == nil {
			unknown++
		} else {
			free -= *t.Minutes
		}
	}
	status := "Free"
	if free < 0 {
		status, free = "Over", -free
	}
	result := fmt.Sprintf("%s %s", status, duration(time.Duration(free*float64(time.Minute))))
	if unknown > 0 {
		result += fmt.Sprintf(" | %d unestimated", unknown)
	}
	return result
}

func sectionMinute(value string) int {
	if value == "24:00" {
		return 24 * 60
	}
	clock, _ := time.Parse("15:04", value)
	return clock.Hour()*60 + clock.Minute()
}
