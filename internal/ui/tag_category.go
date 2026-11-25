package ui

import (
	"strings"
)

// newTagAutocompleteEntry is a comma-separated tags entry that suggests known
// server tags for the tag being typed (the text after the last comma).
func newTagAutocompleteEntry(known []string) *pathAutocompleteEntry {
	entry := newPathAutocompleteEntryWithDelay(nil, 0, nil, nil)
	entry.localSuggest = func(text string) []string {
		return tagSuggestions(text, known)
	}
	entry.applySuggestion = applyTagSuggestion
	entry.acceptOnSpace = false
	entry.suggestions = tagSuggestions("", known)
	return entry
}

// splitLastTag splits comma-separated tags into everything up to the last
// comma and the tag currently being typed.
func splitLastTag(text string) (string, string) {
	index := strings.LastIndex(text, ",")
	if index < 0 {
		return "", text
	}
	return text[:index+1], text[index+1:]
}

// tagSuggestions lists known tags that start with the tag being typed,
// skipping tags already entered (case-insensitively).
func tagSuggestions(text string, known []string) []string {
	head, token := splitLastTag(text)
	token = strings.ToLower(strings.TrimSpace(token))
	entered := map[string]bool{}
	for _, tag := range strings.Split(head, ",") {
		if tag = strings.ToLower(strings.TrimSpace(tag)); tag != "" {
			entered[tag] = true
		}
	}

	var out []string
	for _, tag := range known {
		lower := strings.ToLower(tag)
		if entered[lower] || !strings.HasPrefix(lower, token) {
			continue
		}
		out = append(out, tag)
	}
	return out
}

// applyTagSuggestion replaces the tag being typed with the chosen one.
func applyTagSuggestion(text string, tag string) string {
	head, _ := splitLastTag(text)
	if head == "" {
		return tag
	}
	return head + " " + tag
}

// shouldAutofillSavePath reports whether picking a category may overwrite the
// save path: only while the field is empty or still holds a value the dialog
// filled in itself (the initial path, the server default, or an earlier
// category's path), never after the user typed their own.
func shouldAutofillSavePath(current string, untouched ...string) bool {
	current = strings.TrimSpace(current)
	if current == "" {
		return true
	}
	for _, value := range untouched {
		if current == strings.TrimSpace(value) {
			return true
		}
	}
	return false
}
