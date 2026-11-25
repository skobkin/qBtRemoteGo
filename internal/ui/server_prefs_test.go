package ui

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/skobkin/qbtremotego/internal/qbt"
)

func rawPrefs(t *testing.T, doc string) qbt.Preferences {
	t.Helper()
	var prefs qbt.Preferences
	if err := json.Unmarshal([]byte(doc), &prefs); err != nil {
		t.Fatalf("decode prefs: %v", err)
	}

	return prefs
}

func speedPrefsDoc() string {
	return `{"up_limit":2560000,"dl_limit":0,"alt_up_limit":512000,"alt_dl_limit":512000,
		"scheduler_enabled":false,"schedule_from_hour":8,"schedule_from_min":0,
		"schedule_to_hour":20,"schedule_to_min":0,"scheduler_days":0,
		"limit_utp_rate":true,"limit_tcp_overhead":false,"limit_lan_peers":false}`
}

func fieldNames(fields []*prefField) []string {
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		names = append(names, f.name)
	}

	return names
}

func TestRateLimitControlConvertsKiBAndValidates(t *testing.T) {
	test.NewTempApp(t)
	field := rateLimitField("dl_limit", "Download", "Global download limit")
	editor := field.control.newEditor(field)

	editor.load(prefValues{"dl_limit": int64(1048576)})
	if got := editor.(*entryEditor).entry.Text; got != "1024" {
		t.Fatalf("shown %q, want 1024 (KiB/s)", got)
	}

	tests := []struct {
		text    string
		want    int64
		wantErr bool
	}{
		{"2048", 2097152, false},
		{" 0 ", 0, false},
		{"2097151", 2097151 * 1024, false},
		{"2097152", 0, true},
		{"-1", 0, true},
		{"1.5", 0, true},
		{"", 0, true},
	}
	for _, tt := range tests {
		editor.(*entryEditor).entry.SetText(tt.text)
		values := prefValues{}
		err := editor.store(values)
		if (err != nil) != tt.wantErr {
			t.Fatalf("store(%q) error %v, wantErr %v", tt.text, err, tt.wantErr)
		}
		if !tt.wantErr && values["dl_limit"] != tt.want {
			t.Fatalf("store(%q) = %v, want %d", tt.text, values["dl_limit"], tt.want)
		}
	}
}

func TestTimeControlUsesHourMinutePair(t *testing.T) {
	test.NewTempApp(t)
	field := timeField("schedule_from_hour", "schedule_from_min", "From", "Schedule start")
	editor := field.control.newEditor(field)

	editor.load(prefValues{"schedule_from_hour": int64(8), "schedule_from_min": int64(5)})
	if got := editor.(*entryEditor).entry.Text; got != "08:05" {
		t.Fatalf("shown %q, want 08:05", got)
	}

	tests := []struct {
		text       string
		hour, mins int64
		wantErr    bool
	}{
		{"8:30", 8, 30, false},
		{"23:59", 23, 59, false},
		{"00:00", 0, 0, false},
		{"24:00", 0, 0, true},
		{"12:60", 0, 0, true},
		{"1230", 0, 0, true},
		{"ab:cd", 0, 0, true},
	}
	for _, tt := range tests {
		editor.(*entryEditor).entry.SetText(tt.text)
		values := prefValues{}
		err := editor.store(values)
		if (err != nil) != tt.wantErr {
			t.Fatalf("store(%q) error %v, wantErr %v", tt.text, err, tt.wantErr)
		}
		if !tt.wantErr && (values["schedule_from_hour"] != tt.hour || values["schedule_from_min"] != tt.mins) {
			t.Fatalf("store(%q) = %v", tt.text, values)
		}
	}
}

func TestPresentFieldsHidesUnsupportedKeys(t *testing.T) {
	fields := []*prefField{
		rateLimitField("dl_limit", "Download", "dl"),
		checkField("limit_lan_peers", "LAN", "lan"),
		timeField("schedule_from_hour", "schedule_from_min", "From", "from"),
		enumField("scheduler_days", "When", "days", schedulerDayOptions()),
	}
	// schedule_from_min is missing (pair incomplete), limit_lan_peers has the
	// wrong type and scheduler_days is absent: only dl_limit survives.
	prefs := rawPrefs(t, `{"dl_limit":0,"limit_lan_peers":"yes","schedule_from_hour":8}`)

	present, values := presentFields(fields, prefs)
	if got := fieldNames(present); !reflect.DeepEqual(got, []string{"dl"}) {
		t.Fatalf("present = %v", got)
	}
	if !reflect.DeepEqual(values, prefValues{"dl_limit": int64(0)}) {
		t.Fatalf("values = %v", values)
	}
}

func TestChangesPayloadSendsWholeFieldWhenOneKeyChanged(t *testing.T) {
	from := timeField("schedule_from_hour", "schedule_from_min", "From", "from")
	lan := checkField("limit_lan_peers", "LAN", "lan")
	fields := []*prefField{from, lan}

	baseline := prefValues{"schedule_from_hour": int64(8), "schedule_from_min": int64(0), "limit_lan_peers": false}
	edited := prefValues{"schedule_from_hour": int64(8), "schedule_from_min": int64(30), "limit_lan_peers": false}

	changed := changedFields(fields, baseline, edited)
	if got := fieldNames(changed); !reflect.DeepEqual(got, []string{"from"}) {
		t.Fatalf("changed = %v", got)
	}
	payload := changesPayload(changed, edited)
	want := map[string]any{"schedule_from_hour": int64(8), "schedule_from_min": int64(30)}
	if !reflect.DeepEqual(payload, want) {
		t.Fatalf("payload = %v, want both keys of the pair", payload)
	}
}

func TestConflictingFieldsOnlyReportsEditedFields(t *testing.T) {
	dl := rateLimitField("dl_limit", "Download", "dl")

	baseline := prefValues{"dl_limit": int64(0), "up_limit": int64(0)}
	// Both changed remotely, but only dl was edited locally.
	fresh := prefValues{"dl_limit": int64(1024), "up_limit": int64(2048)}

	got := conflictingFields([]*prefField{dl}, baseline, fresh)
	if names := fieldNames(got); !reflect.DeepEqual(names, []string{"dl"}) {
		t.Fatalf("conflicts = %v", names)
	}
	if got := conflictingFields([]*prefField{dl}, baseline, baseline); len(got) != 0 {
		t.Fatalf("unchanged server reported conflicts: %v", fieldNames(got))
	}
}

func TestUnappliedFieldsComparesSentWithReread(t *testing.T) {
	dl := rateLimitField("dl_limit", "Download", "dl")
	days := enumField("scheduler_days", "When", "days", schedulerDayOptions())
	sent := prefValues{"dl_limit": int64(2048), "scheduler_days": int64(1)}
	// The server kept the old schedule days and dropped dl_limit entirely.
	reread := prefValues{"scheduler_days": int64(0)}

	got := unappliedFields([]*prefField{dl, days}, sent, reread)
	if names := fieldNames(got); !reflect.DeepEqual(names, []string{"dl", "days"}) {
		t.Fatalf("unapplied = %v", names)
	}
	if got := unappliedFields([]*prefField{dl}, sent, sent); len(got) != 0 {
		t.Fatalf("applied value reported as unapplied")
	}
}

func TestPrefFieldFormatting(t *testing.T) {
	tests := []struct {
		field  *prefField
		values prefValues
		want   string
	}{
		{rateLimitField("dl_limit", "", ""), prefValues{"dl_limit": int64(0)}, "Unlimited"},
		{rateLimitField("dl_limit", "", ""), prefValues{"dl_limit": int64(2560000)}, "2500 KiB/s"},
		{checkField("lsd", "", ""), prefValues{"lsd": true}, "On"},
		{checkField("lsd", "", ""), prefValues{"lsd": false}, "Off"},
		{timeField("h", "m", "", ""), prefValues{"h": int64(20), "m": int64(5)}, "20:05"},
		{enumField("scheduler_days", "", "", schedulerDayOptions()), prefValues{"scheduler_days": int64(2)}, "Weekends"},
		{enumField("encryption", "", "", []prefOption[int64]{{0, "Allow"}, {1, "Require"}}), prefValues{"encryption": int64(42)}, "42"},
		{rateLimitField("dl_limit", "", ""), prefValues{}, "(not set)"},
	}
	for _, tt := range tests {
		if got := tt.field.format(tt.values); got != tt.want {
			t.Errorf("format(%v) = %q, want %q", tt.values, got, tt.want)
		}
	}
}

func TestPrefFormDisablesFieldsWhoseSwitchIsOff(t *testing.T) {
	test.NewTempApp(t)
	form := newPrefForm(serverPrefTabs(nil), rawPrefs(t, speedPrefsDoc()))

	from := form.fieldByKey("schedule_from_hour")
	days := form.fieldByKey("scheduler_days")
	if from == nil || days == nil {
		t.Fatal("schedule fields missing from the Speed tab")
	}
	if !form.editors[from].disabled() || !form.editors[days].disabled() {
		t.Fatal("schedule fields enabled while the schedule is off")
	}

	toggle := form.editors[form.fieldByKey("scheduler_enabled")].(*checkEditor).check
	toggle.SetChecked(true)
	if form.editors[from].disabled() || form.editors[days].disabled() {
		t.Fatal("schedule fields stay disabled after enabling the schedule")
	}
}

func TestPrefFormRoundTripHasNoChanges(t *testing.T) {
	test.NewTempApp(t)
	form := newPrefForm(serverPrefTabs(nil), rawPrefs(t, speedPrefsDoc()))

	edited, err := form.edited()
	if err != nil {
		t.Fatalf("edited: %v", err)
	}
	if changed := changedFields(form.fields, form.baseline, edited); len(changed) != 0 {
		t.Fatalf("untouched form reports changes: %v", fieldNames(changed))
	}

	form.editors[form.fieldByKey("dl_limit")].(*entryEditor).entry.SetText("100")
	edited, err = form.edited()
	if err != nil {
		t.Fatalf("edited: %v", err)
	}
	changed := changedFields(form.fields, form.baseline, edited)
	if len(changed) != 1 || changed[0].keys[0] != "dl_limit" {
		t.Fatalf("changed = %v", fieldNames(changed))
	}
	if got := changesPayload(changed, edited); !reflect.DeepEqual(got, map[string]any{"dl_limit": int64(102400)}) {
		t.Fatalf("payload = %v", got)
	}
}

func TestPrefFormReportsInvalidFieldByName(t *testing.T) {
	test.NewTempApp(t)
	form := newPrefForm(serverPrefTabs(nil), rawPrefs(t, speedPrefsDoc()))
	form.editors[form.fieldByKey("schedule_to_hour")].(*entryEditor).entry.SetText("25:00")

	_, err := form.edited()
	if err == nil {
		t.Fatal("invalid time accepted")
	}
	if want := "Alternative limits schedule end"; !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not name the field %q", err, want)
	}
}

func TestPrefFormShowsPlaceholderForUnsupportedTab(t *testing.T) {
	test.NewTempApp(t)
	form := newPrefForm(serverPrefTabs(nil), rawPrefs(t, `{"unrelated":1}`))

	if len(form.fields) != 0 {
		t.Fatalf("fields = %v", fieldNames(form.fields))
	}
	if len(form.tabs.Items) != len(serverPrefTabs(nil)) || form.tabs.Items[0].Text != "Behavior" {
		t.Fatalf("tabs = %d", len(form.tabs.Items))
	}
	for _, item := range form.tabs.Items {
		if !slices.Contains(labelTexts(item.Content), unsupportedTabText) {
			t.Fatalf("placeholder %q not rendered on %s", unsupportedTabText, item.Text)
		}
	}
}

func labelTexts(obj fyne.CanvasObject) []string {
	var out []string
	switch o := obj.(type) {
	case *widget.Label:
		out = append(out, o.Text)
	case *fyne.Container:
		for _, child := range o.Objects {
			out = append(out, labelTexts(child)...)
		}
	case fyne.Widget:
		for _, child := range test.WidgetRenderer(o).Objects() {
			out = append(out, labelTexts(child)...)
		}
	}

	return out
}
