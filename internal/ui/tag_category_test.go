package ui

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestTagSuggestions(t *testing.T) {
	known := []string{"debian", "Desktop", "linux", "iso"}
	tests := []struct {
		text string
		want []string
	}{
		{"", []string{"debian", "Desktop", "linux", "iso"}},
		{"de", []string{"debian", "Desktop"}},
		{"linux, de", []string{"debian", "Desktop"}},
		{"linux,", []string{"debian", "Desktop", "iso"}},
		{"debian, LINUX, ", []string{"Desktop", "iso"}},
		{"zzz", nil},
	}
	for _, tt := range tests {
		if got := tagSuggestions(tt.text, known); !slices.Equal(got, tt.want) {
			t.Errorf("tagSuggestions(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestApplyTagSuggestion(t *testing.T) {
	tests := []struct {
		text, tag, want string
	}{
		{"de", "debian", "debian"},
		{"linux, de", "debian", "linux, debian"},
		{"linux,de", "debian", "linux, debian"},
		{"linux, ", "iso", "linux, iso"},
	}
	for _, tt := range tests {
		if got := applyTagSuggestion(tt.text, tt.tag); got != tt.want {
			t.Errorf("applyTagSuggestion(%q, %q) = %q, want %q", tt.text, tt.tag, got, tt.want)
		}
	}
}

func TestTagAutocompleteEntryCompletesLastToken(t *testing.T) {
	test.NewTempApp(t)
	entry := newTagAutocompleteEntry([]string{"debian", "linux"})
	t.Cleanup(entry.Close)

	entry.SetText("linux, de")
	if !slices.Equal(entry.suggestions, []string{"debian"}) {
		t.Fatalf("suggestions %v, want [debian]", entry.suggestions)
	}

	entry.acceptSuggestion(0)
	if entry.Text != "linux, debian" {
		t.Fatalf("text %q after accepting, want %q", entry.Text, "linux, debian")
	}
}

func TestShouldAutofillSavePath(t *testing.T) {
	tests := []struct {
		name      string
		current   string
		untouched []string
		want      bool
	}{
		{"still the initial path", "/data", []string{"/data", ""}, true},
		{"empty field", "  ", []string{"/data"}, true},
		{"last autofilled path", "/data/movies", []string{"/data", "/data/movies"}, true},
		{"edited by the user", "/mine", []string{"/data", "/data/movies"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldAutofillSavePath(tt.current, tt.untouched...); got != tt.want {
				t.Fatalf("shouldAutofillSavePath = %v, want %v", got, tt.want)
			}
		})
	}
}
