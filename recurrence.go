package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

var repeatWeek = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

type repeatRule struct {
	interval     int
	daily        bool
	days         []string
	monthDay     int
	monthOrdinal int
	monthWeekday string
}

func parseRepeat(value string) (repeatRule, error) {
	invalid := func() (repeatRule, error) {
		return repeatRule{}, fmt.Errorf("invalid repeat %q: use daily, weekday, holiday, mon,wed, weekly:1:tue, monthly:1 or monthly:2:tue with optional /N", value)
	}
	pattern, interval, hasInterval := strings.Cut(strings.ToLower(value), "/")
	if weekly, ok := strings.CutPrefix(pattern, "weekly:"); ok {
		count, weekday, found := strings.Cut(weekly, ":")
		n := repeatNumber(count)
		if hasInterval || !found || !slices.Contains(repeatWeek, weekday) || n < 1 {
			return invalid()
		}
		return repeatRule{interval: n, days: []string{weekday}}, nil
	}
	rule := repeatRule{interval: 1}
	if hasInterval {
		rule.interval = repeatNumber(interval)
		if rule.interval < 1 {
			return invalid()
		}
	}
	switch pattern {
	case "daily":
		rule.daily = true
		rule.days = repeatWeek
	case "weekday":
		rule.days = repeatWeek[:5]
	case "holiday":
		rule.days = repeatWeek[5:]
	default:
		if monthly, ok := strings.CutPrefix(pattern, "monthly:"); ok {
			number, weekday, ordinal := strings.Cut(monthly, ":")
			n := repeatNumber(number)
			if ordinal {
				if n < 1 || n > 5 || !slices.Contains(repeatWeek, weekday) {
					return invalid()
				}
				rule.monthOrdinal, rule.monthWeekday = n, weekday
			} else {
				if n < 1 || n > 31 {
					return invalid()
				}
				rule.monthDay = n
			}
		} else {
			days := strings.Split(pattern, ",")
			for _, day := range days {
				if !slices.Contains(repeatWeek, day) {
					return invalid()
				}
			}
			for _, day := range repeatWeek {
				if slices.Contains(days, day) {
					rule.days = append(rule.days, day)
				}
			}
		}
	}
	return rule, nil
}

func repeatNumber(value string) int {
	if strings.ContainsFunc(value, func(r rune) bool { return r < '0' || r > '9' }) {
		return 0
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return n
}

func daysLabel(days []string, allowDaily bool) string {
	if allowDaily && slices.Equal(days, repeatWeek) {
		return "daily"
	}
	if slices.Equal(days, repeatWeek[:5]) {
		return "weekday"
	}
	if slices.Equal(days, repeatWeek[5:]) {
		return "holiday"
	}
	return strings.Join(days, ",")
}

func (r repeatRule) label() string {
	var label string
	switch {
	case r.daily:
		label = "daily"
	case r.monthDay != 0:
		label = fmt.Sprintf("monthly:%d", r.monthDay)
	case r.monthOrdinal != 0:
		label = fmt.Sprintf("monthly:%d:%s", r.monthOrdinal, r.monthWeekday)
	default:
		label = daysLabel(r.days, r.interval == 1)
	}
	if r.interval > 1 {
		label += "/" + strconv.Itoa(r.interval)
	}
	return label
}

func (r *routine) setRepeat(value string) error {
	rule, err := parseRepeat(value)
	if err != nil {
		return err
	}
	r.Days, r.Repeat = nil, ""
	if len(rule.days) != 0 && rule.interval == 1 {
		r.Days = slices.Clone(rule.days)
	} else {
		r.Repeat = rule.label()
	}
	return nil
}

func (r routine) repeatLabel() string {
	if r.Repeat != "" {
		return r.Repeat
	}
	return daysLabel(r.Days, true)
}

func (r routine) occursOn(day time.Time) bool {
	if day.Format(dateLayout) < r.FirstDay {
		return false
	}
	weekday := (int(day.Weekday()) + 6) % 7
	if r.Repeat == "" {
		return slices.Contains(r.Days, repeatWeek[weekday])
	}
	start, err := time.Parse(dateLayout, r.FirstDay)
	if err != nil {
		return false
	}
	rule, err := parseRepeat(r.Repeat)
	if err != nil {
		return false
	}
	if rule.monthDay != 0 || rule.monthOrdinal != 0 {
		months := (day.Year()-start.Year())*12 + int(day.Month()-start.Month())
		if months%rule.interval != 0 {
			return false
		}
		if rule.monthDay != 0 {
			return day.Day() == rule.monthDay
		}
		return repeatWeek[weekday] == rule.monthWeekday && 1+(day.Day()-1)/7 == rule.monthOrdinal
	}
	// Both inputs come from date-only parsing in UTC. Unix-day differences
	// remain exact over the full date range, independent of local clock changes.
	days := (day.Unix() - start.Unix()) / 86400
	if rule.daily {
		return days%int64(rule.interval) == 0
	}
	startWeekday := (int(start.Weekday()) + 6) % 7
	weeks := (days + int64(startWeekday)) / 7
	return weeks%int64(rule.interval) == 0 && slices.Contains(rule.days, repeatWeek[weekday])
}
