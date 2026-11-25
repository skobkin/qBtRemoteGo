package ui

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/skobkin/qbtremotego/internal/qbt"
)

// fakePrefsBackend is an in-memory qBittorrent preferences store.
type fakePrefsBackend struct {
	prefs    map[string]any
	ignore   map[string]bool // keys the server silently drops
	applyErr error
	fetchErr error
	applied  []map[string]any
}

func newFakePrefsBackend(t *testing.T) *fakePrefsBackend {
	t.Helper()
	var prefs map[string]any
	if err := json.Unmarshal([]byte(speedPrefsDoc()), &prefs); err != nil {
		t.Fatalf("decode prefs: %v", err)
	}

	return &fakePrefsBackend{prefs: prefs, ignore: map[string]bool{}}
}

func (b *fakePrefsBackend) FetchPreferences(context.Context) (qbt.Preferences, error) {
	if b.fetchErr != nil {
		return nil, b.fetchErr
	}
	raw, err := json.Marshal(b.prefs)
	if err != nil {
		return nil, err
	}
	var prefs qbt.Preferences
	err = json.Unmarshal(raw, &prefs)

	return prefs, err
}

func (b *fakePrefsBackend) ApplyPreferences(_ context.Context, changes map[string]any) error {
	b.applied = append(b.applied, changes)
	for key, value := range changes {
		if !b.ignore[key] {
			b.prefs[key] = value
		}
	}

	return b.applyErr
}

type serverSettingsRecorder struct {
	confirmTitle string
	confirmText  string
	confirmLines []string
	onConfirm    func()
	errs         []error
	notes        []string
	refreshes    int
}

func newTestServerSettings(t *testing.T, backend *fakePrefsBackend) (*serverSettingsWindow, *serverSettingsRecorder) {
	t.Helper()
	test.NewTempApp(t)
	rec := &serverSettingsRecorder{}
	w := newServerSettingsWindow(fyne.CurrentApp(), backend, "qbt.example:8080")
	w.run = func(work func(), done func()) {
		work()
		done()
	}
	w.confirm = func(title string, content fyne.CanvasObject, confirmText string, onConfirm func()) {
		rec.confirmTitle = title
		rec.confirmText = confirmText
		rec.confirmLines = labelTexts(content)
		rec.onConfirm = onConfirm
	}
	w.showError = func(err error) { rec.errs = append(rec.errs, err) }
	w.notify = func(title, message string) { rec.notes = append(rec.notes, title+": "+message) }
	w.afterApply = func() { rec.refreshes++ }
	t.Cleanup(w.win.Close)
	w.load()

	return w, rec
}

func setEntry(t *testing.T, w *serverSettingsWindow, key, text string) {
	t.Helper()
	field := w.form.fieldByKey(key)
	if field == nil {
		t.Fatalf("no field for %s", key)
	}
	w.form.editors[field].(*entryEditor).entry.SetText(text)
}

func TestServerSettingsApplySendsOnlyChangedKeys(t *testing.T) {
	backend := newFakePrefsBackend(t)
	w, rec := newTestServerSettings(t, backend)

	setEntry(t, w, "dl_limit", "100")
	w.startApply()

	if rec.onConfirm == nil {
		t.Fatal("apply did not ask for confirmation")
	}
	if rec.confirmText != "Apply" {
		t.Fatalf("confirm button %q, want Apply", rec.confirmText)
	}
	wantLine := "Global download limit: Unlimited → 100 KiB/s"
	if !containsLine(rec.confirmLines, wantLine) {
		t.Fatalf("summary %q lacks %q", rec.confirmLines, wantLine)
	}

	rec.onConfirm()
	if len(backend.applied) != 1 || !reflect.DeepEqual(backend.applied[0], map[string]any{"dl_limit": int64(102400)}) {
		t.Fatalf("applied %v", backend.applied)
	}
	if len(rec.errs) != 0 {
		t.Fatalf("errors %v", rec.errs)
	}
	if len(rec.notes) != 1 || !strings.HasPrefix(rec.notes[0], "Server settings applied") {
		t.Fatalf("notes %v", rec.notes)
	}
	if w.dirty() {
		t.Fatal("form still dirty after a successful apply")
	}
	if rec.refreshes != 1 {
		t.Fatalf("main window refreshed %d times, want 1", rec.refreshes)
	}
}

func TestServerSettingsApplyWithoutChangesDoesNothing(t *testing.T) {
	backend := newFakePrefsBackend(t)
	w, rec := newTestServerSettings(t, backend)

	w.startApply()
	if rec.onConfirm != nil || len(backend.applied) != 0 {
		t.Fatal("unchanged form tried to apply")
	}
	if len(rec.notes) != 1 || !strings.HasPrefix(rec.notes[0], "No changes") {
		t.Fatalf("notes %v", rec.notes)
	}
}

func TestServerSettingsFlagsConcurrentServerChanges(t *testing.T) {
	backend := newFakePrefsBackend(t)
	w, rec := newTestServerSettings(t, backend)

	// Another client changes the same limit after the window loaded.
	backend.prefs["dl_limit"] = 2048 * 1024
	setEntry(t, w, "dl_limit", "100")
	w.startApply()

	if rec.confirmText != "Apply anyway" {
		t.Fatalf("confirm button %q, want Apply anyway", rec.confirmText)
	}
	wantLine := "Global download limit: Unlimited → 100 KiB/s  (server now: 2048 KiB/s)"
	if !containsLine(rec.confirmLines, wantLine) {
		t.Fatalf("summary %q lacks %q", rec.confirmLines, wantLine)
	}
}

func TestServerSettingsReportsValuesTheServerIgnored(t *testing.T) {
	backend := newFakePrefsBackend(t)
	backend.ignore["scheduler_days"] = true
	w, rec := newTestServerSettings(t, backend)

	days := w.form.editors[w.form.fieldByKey("scheduler_days")].(*enumEditor)
	days.selector.SetSelected("Weekends")
	w.startApply()
	rec.onConfirm()

	if len(rec.notes) != 1 || !strings.Contains(rec.notes[0], "Alternative limits schedule days: sent Weekends, server has Every day") {
		t.Fatalf("notes %v", rec.notes)
	}
	if days.selector.Selected != "Every day" {
		t.Fatalf("form shows %q, want the server's value", days.selector.Selected)
	}
}

func TestServerSettingsRejectedApplyReloadsServerState(t *testing.T) {
	backend := newFakePrefsBackend(t)
	backend.applyErr = errors.New("app/setPreferences returned 400 Bad Request: nope")
	w, rec := newTestServerSettings(t, backend)

	setEntry(t, w, "up_limit", "10")
	w.startApply()
	rec.onConfirm()

	if len(rec.errs) != 1 || !strings.Contains(rec.errs[0].Error(), "nope") {
		t.Fatalf("errors %v", rec.errs)
	}
	// The fake applied the key before failing, like the real server can.
	if got := w.form.baseline["up_limit"]; got != int64(10240) {
		t.Fatalf("baseline up_limit %v, want the re-read server value", got)
	}
	if w.dirty() {
		t.Fatal("form should show the re-read server state")
	}
}

func TestServerSettingsInvalidFieldBlocksApply(t *testing.T) {
	backend := newFakePrefsBackend(t)
	w, rec := newTestServerSettings(t, backend)

	setEntry(t, w, "schedule_from_hour", "8:75")
	w.startApply()

	if rec.onConfirm != nil || len(backend.applied) != 0 {
		t.Fatal("invalid form was applied")
	}
	if len(rec.errs) != 1 || !strings.Contains(rec.errs[0].Error(), "Alternative limits schedule start") {
		t.Fatalf("errors %v", rec.errs)
	}
}

func TestServerSettingsLoadErrorDisablesApply(t *testing.T) {
	backend := newFakePrefsBackend(t)
	backend.fetchErr = errors.New("connection refused")
	w, _ := newTestServerSettings(t, backend)

	if w.form != nil {
		t.Fatal("form built despite the load error")
	}
	if !w.applyButton.Disabled() {
		t.Fatal("Apply enabled without loaded settings")
	}
	if w.reloadButton.Disabled() {
		t.Fatal("Reload disabled after a failed load")
	}
	if !containsLine(labelTexts(w.body), "Could not load the server settings:\nconnection refused") {
		t.Fatalf("body %q", labelTexts(w.body))
	}
}

func TestServerSettingsCloseAsksOnlyWhenDirty(t *testing.T) {
	backend := newFakePrefsBackend(t)
	w, rec := newTestServerSettings(t, backend)

	setEntry(t, w, "alt_up_limit", "1")
	w.requestClose()
	if rec.confirmTitle != "Discard changes?" {
		t.Fatalf("dirty close confirm %q", rec.confirmTitle)
	}
}

func TestRemoteHostLabel(t *testing.T) {
	tests := map[string]string{
		"https://qbt.example:8443/": "qbt.example:8443",
		"http://10.0.0.5:8080":      "10.0.0.5:8080",
		"":                          "(not configured)",
		"not a url":                 "not a url",
	}
	for in, want := range tests {
		if got := remoteHostLabel(in); got != want {
			t.Errorf("remoteHostLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func containsLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}

	return false
}
