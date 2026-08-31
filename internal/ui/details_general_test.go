package ui

import (
	"testing"
)

func TestDetailsCountLimit(t *testing.T) {
	tests := []struct {
		name  string
		count int
		limit int
		want  string
	}{
		{name: "limited connections", count: 42, limit: 500, want: "42 (500 max)"},
		{name: "unlimited connections", count: 42, limit: -1, want: "42 (∞ max)"},
		{name: "zero limit means unlimited", count: 0, limit: 0, want: "0 (∞ max)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detailsCountLimit(tt.count, tt.limit); got != tt.want {
				t.Fatalf("detailsCountLimit(%d, %d) = %q, want %q", tt.count, tt.limit, got, tt.want)
			}
		})
	}
}

func TestDetailsAvailability(t *testing.T) {
	tests := []struct {
		name  string
		value float64
		want  string
	}{
		{name: "fully available", value: 1, want: "100.0%"},
		{name: "partial availability", value: 0.75, want: "75.0%"},
		{name: "seed surplus", value: 2.5, want: "250.0%"},
		{name: "unknown availability", value: -1, want: "?"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detailsAvailability(tt.value); got != tt.want {
				t.Fatalf("detailsAvailability(%v) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestDetailsUnixFallback(t *testing.T) {
	if got := detailsUnix(0, "Never"); got != "Never" {
		t.Fatalf("detailsUnix(0) = %q, want %q", got, "Never")
	}
	if got := detailsUnix(-1, "Never"); got != "Never" {
		t.Fatalf("detailsUnix(-1) = %q, want %q", got, "Never")
	}
	if got := detailsUnix(0, ""); got != "" {
		t.Fatalf("detailsUnix(0) = %q, want empty", got)
	}
	if got := detailsUnix(1, "Never"); got == "Never" {
		t.Fatalf("expected a formatted date for a valid timestamp, got %q", got)
	}
}
