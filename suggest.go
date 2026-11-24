package main

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type inputSuggestion struct {
	label  string
	input  string
	source *task
}

func (m terminal) inputSuggestions() []inputSuggestion {
	if m.mode == modeSearch || m.endEdit.task != nil {
		return nil
	}
	if m.fieldEdit != nil {
		if m.fieldEdit.field == "code" {
			return m.workCodeSuggestions("", m.input)
		}
		return nil
	}
	command, body, found := strings.Cut(m.input, " ")
	if found && (command == "code" || command == "routine-code") {
		body = strings.TrimLeft(body, " ")
		target, query, separated := strings.Cut(body, " ")
		if command == "code" && !separated && target != "@" {
			return m.workCodeSuggestions("code @ ", target)
		}
		if target != "" && (separated || target == "@") {
			return m.workCodeSuggestions(command+" "+target+" ", query)
		}
		return nil
	}
	if words := strings.Fields(m.input); len(words) >= 3 && words[0] == "rename" {
		prefix := "rename " + words[1] + " "
		query := strings.Join(words[2:], " ")
		var matches []inputSuggestion
		for _, task := range m.matchingTaskTitles(query) {
			matches = append(matches, inputSuggestion{label: task.Title, input: prefix + task.Title})
		}
		return matches
	}
	return m.addSuggestions()
}

func (m terminal) workCodeSuggestions(prefix, input string) []inputSuggestion {
	query := strings.ToLower(strings.TrimSpace(input))
	seen := make(map[string]bool)
	var matches []inputSuggestion
	for i := len(m.data.Tasks) - 1; i >= 0 && len(matches) < 5; i-- {
		t := m.data.Tasks[i]
		if t.Deleted || t.Day > m.day || t.WorkCode == "" || strings.ContainsFunc(t.WorkCode, unicode.IsControl) {
			continue
		}
		if !containsFold(t.WorkCode, query) {
			continue
		}
		if code := strings.ToLower(t.WorkCode); !seen[code] {
			seen[code] = true
			matches = append(matches, inputSuggestion{label: t.WorkCode, input: prefix + t.WorkCode})
		}
	}
	return matches
}

// addSuggestions inherits the estimate unless the user supplied one.
func (m terminal) addSuggestions() []inputSuggestion {
	body, ok := strings.CutPrefix(m.input, "add ")
	if !ok {
		return nil
	}
	prefix := "add "
	explicitEstimate := false
	body = strings.TrimLeft(body, " ")
	if first, rest, spaced := strings.Cut(body, " "); spaced {
		if _, err := minutes(first); err == nil {
			prefix += first + " "
			explicitEstimate = true
			body = strings.TrimLeft(rest, " ")
		}
	}
	var matches []inputSuggestion
	for _, t := range m.matchingTaskTitles(body) {
		candidate := inputSuggestion{label: t.Title, input: prefix + t.Title, source: t}
		if t.Minutes != nil {
			candidate.label += " · " + estimateLabel(t.Minutes)
			if !explicitEstimate {
				candidate.input = fmt.Sprintf("add %d %s", roundedMinutes(*t.Minutes), t.Title)
			}
		} else if first, _, _ := strings.Cut(t.Title, " "); !explicitEstimate && looksLikeEstimate(first) {
			// "add 10 push-ups" would read 10 as minutes; "-" keeps it a title.
			candidate.input = "add - " + t.Title
		}
		matches = append(matches, candidate)
	}
	return matches
}

// matchingTaskTitles prefers prefix matches and keeps the newest task for each title.
func (m terminal) matchingTaskTitles(input string) []*task {
	query := strings.ToLower(strings.TrimLeftFunc(input, unicode.IsSpace))
	if query == "" {
		return nil
	}
	const limit = 5
	var prefixes, partials []*task
	seen := make(map[string]bool)
	for i := len(m.data.Tasks) - 1; i >= 0 && len(prefixes) < limit; i-- {
		t := m.data.Tasks[i]
		if t.Title == "" || t.Deleted || t.Day > m.day {
			continue
		}
		isPrefix := hasPrefixFold(t.Title, query)
		if !isPrefix && (len(partials) == limit || !containsFold(t.Title, query)) {
			continue
		}
		name := strings.ToLower(t.Title)
		if seen[name] {
			continue
		}
		seen[name] = true
		if isPrefix {
			prefixes = append(prefixes, t)
		} else {
			partials = append(partials, t)
		}
	}
	return append(prefixes, partials[:min(len(partials), limit-len(prefixes))]...)
}

// hasPrefixFold compares without allocating; prefix must already be lower case.
func hasPrefixFold(s, prefix string) bool {
	for _, want := range prefix {
		r, size := utf8.DecodeRuneInString(s)
		if size == 0 || unicode.ToLower(r) != want {
			return false
		}
		s = s[size:]
	}
	return true
}

// containsFold is the allocation-free form of Contains(ToLower(s), sub).
func containsFold(s, sub string) bool {
	for {
		if hasPrefixFold(s, sub) {
			return true
		}
		if s == "" {
			return false
		}
		_, size := utf8.DecodeRuneInString(s)
		s = s[size:]
	}
}
