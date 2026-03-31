package platform

import "testing"

func TestWindowsHandlerCommand(t *testing.T) {
	got := windowsHandlerCommand(`C:\Program Files\qBtRemoteGo\qbtremotego.exe`)
	want := `"C:\Program Files\qBtRemoteGo\qbtremotego.exe" "%1"`
	if got != want {
		t.Fatalf("windowsHandlerCommand() = %q, want %q", got, want)
	}
}

func TestWindowsDefaultIcon(t *testing.T) {
	got := windowsDefaultIcon(`C:\Program Files\qBtRemoteGo\qbtremotego.exe`)
	want := `"C:\Program Files\qBtRemoteGo\qbtremotego.exe",0`
	if got != want {
		t.Fatalf("windowsDefaultIcon() = %q, want %q", got, want)
	}
}

func TestIsOurWindowsHandlerProgID(t *testing.T) {
	tests := []struct {
		name      string
		expected  string
		current   string
		wantMatch bool
	}{
		{
			name:      "matches app progid",
			expected:  windowsMagnetProgID,
			current:   "qbtremotego.magnet",
			wantMatch: true,
		},
		{
			name:      "matches executable application progid",
			expected:  windowsMagnetProgID,
			current:   `Applications\qbtremotego.exe`,
			wantMatch: true,
		},
		{
			name:      "different app",
			expected:  windowsMagnetProgID,
			current:   "OtherClient.magnet",
			wantMatch: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isOurWindowsHandlerProgID(tc.expected, tc.current)
			if got != tc.wantMatch {
				t.Fatalf("isOurWindowsHandlerProgID(%q, %q) = %v, want %v", tc.expected, tc.current, got, tc.wantMatch)
			}
		})
	}
}

func TestWindowsDefaultSelectionWarning(t *testing.T) {
	got := windowsDefaultSelectionWarning("magnet links", "OtherClient.magnet")
	if got != `Windows registered qBtRemoteGo for magnet links, but the active default is still "OtherClient.magnet". Choose qBtRemoteGo in Settings > Apps > Default apps.` {
		t.Fatalf("windowsDefaultSelectionWarning() = %q", got)
	}
}
