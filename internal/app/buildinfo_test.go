package app

import "testing"

func TestBuildVersion(t *testing.T) {
	original := Version
	t.Cleanup(func() {
		Version = original
	})

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "defaults to dev", in: "", want: DevBuildVersion},
		{name: "trims value", in: " 1.2.3 ", want: "1.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Version = tt.in
			if got := BuildVersion(); got != tt.want {
				t.Fatalf("BuildVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildDateYMD(t *testing.T) {
	original := BuildDate
	t.Cleanup(func() {
		BuildDate = original
	})

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty stays empty", in: "", want: ""},
		{name: "rfc3339 formatted", in: "2026-01-30T14:55:03Z", want: "2026-01-30"},
		{name: "date only", in: "2026-01-30", want: "2026-01-30"},
		{name: "unknown format returns as is", in: "not-a-date", want: "not-a-date"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			BuildDate = tt.in
			if got := BuildDateYMD(); got != tt.want {
				t.Fatalf("BuildDateYMD() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildVersionWithDate(t *testing.T) {
	originalVersion := Version
	originalBuildDate := BuildDate
	t.Cleanup(func() {
		Version = originalVersion
		BuildDate = originalBuildDate
	})

	Version = "0.1.2"
	BuildDate = "2026-01-30T14:55:03Z"
	if got := BuildVersionWithDate(); got != "0.1.2 (2026-01-30)" {
		t.Fatalf("BuildVersionWithDate() = %q", got)
	}
}
