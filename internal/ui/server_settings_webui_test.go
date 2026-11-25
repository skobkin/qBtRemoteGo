package ui

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"github.com/skobkin/qbtremotego/internal/config"
)

// newWebUIServerSettings opens the window on a server that also reports the
// WebUI tab's keys.
func newWebUIServerSettings(t *testing.T, setup func(b *fakePrefsBackend)) (*serverSettingsWindow, *serverSettingsRecorder, *fakePrefsBackend) {
	t.Helper()
	backend := newFakePrefsBackend(t)
	var webUI map[string]any
	if err := json.Unmarshal([]byte(webUIPrefsDoc()), &webUI); err != nil {
		t.Fatalf("decode WebUI prefs: %v", err)
	}
	for key, value := range webUI {
		backend.prefs[key] = value
	}
	if setup != nil {
		setup(backend)
	}
	w, rec := newTestServerSettings(t, backend)

	return w, rec, backend
}

const testLoginPassword = "secret12"

func setPassword(t *testing.T, w *serverSettingsWindow) {
	t.Helper()
	editor := w.form.editors[w.form.fieldByKey("web_ui_password")].(*loginPasswordEditor)
	editor.entry.SetText(testLoginPassword)
	editor.repeat.SetText(testLoginPassword)
}

func linesContaining(lines []string, parts ...string) bool {
	for _, line := range lines {
		all := true
		for _, part := range parts {
			all = all && strings.Contains(line, part)
		}
		if all {
			return true
		}
	}

	return false
}

func TestServerSettingsWarnsAboutChangesThatCanDisconnect(t *testing.T) {
	w, rec, _ := newWebUIServerSettings(t, nil)

	setEntry(t, w, "web_ui_sessions_count_limit", "3")
	w.startApply()
	if linesContaining(rec.confirmLines, "disconnect") {
		t.Fatalf("warning for a harmless change: %q", rec.confirmLines)
	}

	setEntry(t, w, "web_ui_port", "9090")
	w.startApply()
	if !linesContaining(rec.confirmLines, "disconnect", "WebUI port") {
		t.Fatalf("no disconnect warning naming the port: %q", rec.confirmLines)
	}
}

func TestServerSettingsLostConnectionAfterPortChangeOffersSettings(t *testing.T) {
	opened := 0
	w, rec, backend := newWebUIServerSettings(t, nil)
	w.openConfig = func() { opened++ }

	setEntry(t, w, "web_ui_port", "9090")
	w.startApply()
	backend.fetchErr = errors.New("connection refused")
	rec.onConfirm()

	if len(backend.applied) != 1 {
		t.Fatalf("applied %v", backend.applied)
	}
	if rec.confirmText != "Connection settings" || !linesContaining(rec.confirmLines, "WebUI port") {
		t.Fatalf("after losing the server: confirm %q, %q", rec.confirmText, rec.confirmLines)
	}
	rec.onConfirm()
	if opened != 1 {
		t.Fatal("connection settings not opened")
	}
	if w.dirty() {
		t.Fatal("sent values still count as edits")
	}
}

func TestServerSettingsOffersToUpdateTheSavedLogin(t *testing.T) {
	w, rec, backend := newWebUIServerSettings(t, func(b *fakePrefsBackend) { b.loginMethod = config.AuthMethodPassword })

	setEntry(t, w, "web_ui_username", "newadmin")
	setPassword(t, w)
	w.startApply()
	rec.onConfirm()

	if rec.confirmText != "Update login" {
		t.Fatalf("after changing the login: confirm %q %q", rec.confirmText, rec.confirmLines)
	}
	if len(backend.loginUpdates) != 0 {
		t.Fatal("saved login updated before the user agreed")
	}
	rec.onConfirm()
	if want := [][2]string{{"newadmin", "secret12"}}; !reflect.DeepEqual(backend.loginUpdates, want) {
		t.Fatalf("login updates %v, want %v", backend.loginUpdates, want)
	}
	if len(rec.errs) != 0 {
		t.Fatalf("errors %v", rec.errs)
	}
}

func TestServerSettingsLoginUpdateSendsOnlyTheChangedPart(t *testing.T) {
	w, rec, backend := newWebUIServerSettings(t, func(b *fakePrefsBackend) { b.loginMethod = config.AuthMethodPassword })

	setPassword(t, w)
	w.startApply()
	rec.onConfirm()
	rec.onConfirm()

	if want := [][2]string{{"", "secret12"}}; !reflect.DeepEqual(backend.loginUpdates, want) {
		t.Fatalf("login updates %v, want %v", backend.loginUpdates, want)
	}
}

func TestServerSettingsLoginPromptOnlyForPasswordLogin(t *testing.T) {
	w, rec, backend := newWebUIServerSettings(t, nil) // API key login

	setPassword(t, w)
	w.startApply()
	rec.onConfirm()
	if rec.confirmText == "Update login" || len(backend.loginUpdates) != 0 {
		t.Fatal("asked to update a password login the app does not use")
	}
}

func TestServerSettingsNoLoginPromptWhenApplyFails(t *testing.T) {
	w, rec, _ := newWebUIServerSettings(t, func(b *fakePrefsBackend) {
		b.loginMethod = config.AuthMethodPassword
		b.applyErr = errors.New("app/setPreferences returned 400 Bad Request")
	})

	setPassword(t, w)
	w.startApply()
	rec.onConfirm()
	if rec.confirmText == "Update login" {
		t.Fatal("offered to save a login the server may not have taken")
	}
}

func apiKeyEditorOf(w *serverSettingsWindow) *apiKeyEditor {
	return w.form.editors[w.form.fieldByKey("web_ui_api_key")].(*apiKeyEditor)
}

func TestServerSettingsRotatesTheAPIKey(t *testing.T) {
	newKey := "qbt_" + strings.Repeat("n", 28)
	w, rec, backend := newWebUIServerSettings(t, func(b *fakePrefsBackend) { b.rotatedKey = newKey })
	editor := apiKeyEditorOf(w)

	editor.rotateButton.OnTapped()
	if backend.rotations != 0 {
		t.Fatal("rotated without confirmation")
	}
	if rec.confirmText != "Rotate" || !linesContaining(rec.confirmLines, "stop") || !linesContaining(rec.confirmLines, "This app") {
		t.Fatalf("rotation confirm %q %q", rec.confirmText, rec.confirmLines)
	}
	rec.onConfirm()

	if backend.rotations != 1 {
		t.Fatalf("rotations %d", backend.rotations)
	}
	if !strings.HasSuffix(editor.display.Text, "nnnnnn") {
		t.Fatalf("display %q", editor.display.Text)
	}
	if w.dirty() {
		t.Fatal("rotation left the form dirty")
	}
	if len(rec.notes) != 1 || !strings.HasPrefix(rec.notes[0], "API key rotated") {
		t.Fatalf("notes %v", rec.notes)
	}
	if len(backend.applied) != 0 {
		t.Fatal("rotation went through setPreferences")
	}
}

func TestServerSettingsShowsARotatedKeyThatCouldNotBeSaved(t *testing.T) {
	newKey := "qbt_" + strings.Repeat("n", 28)
	w, rec, _ := newWebUIServerSettings(t, func(b *fakePrefsBackend) {
		b.rotatedKey = newKey
		b.rotateErr = errors.New("keychain locked")
	})

	apiKeyEditorOf(w).rotateButton.OnTapped()
	rec.onConfirm()

	if rec.confirmText != "Copy key" || !linesContaining(rec.confirmLines, "keychain locked") {
		t.Fatalf("save failure: confirm %q %q", rec.confirmText, rec.confirmLines)
	}
	rec.onConfirm()
	if got := fyne.CurrentApp().Clipboard().Content(); got != newKey {
		t.Fatalf("clipboard %q, want the new key", got)
	}
}

func TestServerSettingsReportsAFailedRotation(t *testing.T) {
	w, rec, _ := newWebUIServerSettings(t, func(b *fakePrefsBackend) { b.rotateErr = errors.New("403 Forbidden") })
	before := apiKeyEditorOf(w).display.Text

	apiKeyEditorOf(w).rotateButton.OnTapped()
	rec.onConfirm()

	if len(rec.errs) != 1 || !strings.Contains(rec.errs[0].Error(), "403") {
		t.Fatalf("errors %v", rec.errs)
	}
	if apiKeyEditorOf(w).display.Text != before {
		t.Fatal("display changed after a failed rotation")
	}
}

func TestServerSettingsDoesNotSaveAUsernameTheServerIgnored(t *testing.T) {
	w, rec, backend := newWebUIServerSettings(t, func(b *fakePrefsBackend) {
		b.loginMethod = config.AuthMethodPassword
		b.ignore["web_ui_username"] = true
	})

	setEntry(t, w, "web_ui_username", "newadmin")
	setPassword(t, w)
	w.startApply()
	rec.onConfirm()
	rec.onConfirm()

	if want := [][2]string{{"", testLoginPassword}}; !reflect.DeepEqual(backend.loginUpdates, want) {
		t.Fatalf("login updates %v, want only the password", backend.loginUpdates)
	}
}
