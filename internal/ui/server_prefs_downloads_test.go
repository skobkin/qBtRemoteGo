package ui

import (
	"context"
	"reflect"
	"testing"

	"fyne.io/fyne/v2/test"
)

func downloadsPrefsDoc() string {
	return `{"torrent_content_layout":"Original","add_to_top_of_queue":false,"add_stopped_enabled":false,
		"torrent_stop_condition":"None","merge_trackers":false,"auto_delete_mode":0,
		"preallocate_all":false,"incomplete_files_ext":false,"use_unwanted_folder":false,
		"auto_tmm_enabled":false,"torrent_changed_tmm_enabled":true,"save_path_changed_tmm_enabled":false,
		"category_changed_tmm_enabled":false,"use_category_paths_in_manual_mode":false,
		"save_path":"/data/downloads","temp_path_enabled":false,"temp_path":"/data/incomplete",
		"torrent_files_backup_enabled":false,"torrent_files_backup_dir":"","torrent_files_finished_backup_dir_enabled":false,
		"torrent_files_finished_backup_dir":"","remove_torrent_file_backup":false,
		"scan_dirs":{},"excluded_file_names_enabled":false,"excluded_file_names":"*.lnk\n*.url",
		"mail_notification_enabled":false,"mail_notification_sender":"qBittorrent_notification@example.com",
		"mail_notification_email":"","mail_notification_smtp":"smtp.example.com",
		"mail_notification_encryption_type":"SMTPS","mail_notification_auth_enabled":false,
		"mail_notification_username":"","mail_notification_password":"hunter2",
		"autorun_on_torrent_added_enabled":false,"autorun_on_torrent_added_program":"",
		"autorun_enabled":false,"autorun_program":"/usr/bin/notify \"%N\""}`
}

func TestPathControlLoadsWithoutLookupAndSendsText(t *testing.T) {
	test.NewTempApp(t)
	lookups := 0
	dirs := func(context.Context, string) ([]string, error) {
		lookups++
		return nil, nil
	}
	field := pathField("save_path", "Default Save Path", "Default save path", dirs)
	editor := field.control.newEditor(field).(*pathEditor)

	editor.load(prefValues{"save_path": "/data/downloads"})
	if editor.entry.Text != "/data/downloads" {
		t.Fatalf("shown %q", editor.entry.Text)
	}
	if gen := editor.entry.currentGeneration(); gen != 0 {
		t.Fatalf("loading a value scheduled a directory lookup (generation %d)", gen)
	}

	editor.entry.SetText("/data/new/")
	if editor.entry.currentGeneration() == 0 {
		t.Fatal("typing does not schedule a directory lookup")
	}
	values := prefValues{}
	if err := editor.store(values); err != nil || values["save_path"] != "/data/new/" {
		t.Fatalf("store = %v, %v; want the text as typed", values, err)
	}

	editor.setEnabled(false)
	if !editor.disabled() {
		t.Fatal("path entry stays enabled")
	}
	editor.close()
	if !editor.entry.closed {
		t.Fatal("close does not stop the autocomplete entry")
	}
	if lookups != 0 {
		t.Fatalf("lookups ran synchronously: %d", lookups)
	}
}

func TestUnappliedFieldsAcceptsRespelledPaths(t *testing.T) {
	field := pathField("save_path", "", "Default save path", nil)
	tests := []struct {
		sent, reread string
		unapplied    bool
	}{
		{"/data/dl/", "/data/dl", false},
		{"/data//dl/./x", "/data/dl/x", false},
		{"C:/Torrents/", `C:\Torrents`, false},
		{"", "", false},
		{"dl", "/home/qbt/Downloads/dl", true},
		{"/data/a", "/data/b", true},
	}
	for _, tt := range tests {
		got := unappliedFields([]*prefField{field}, prefValues{"save_path": tt.sent}, prefValues{"save_path": tt.reread})
		if (len(got) > 0) != tt.unapplied {
			t.Errorf("sent %q, server has %q: unapplied=%v, want %v", tt.sent, tt.reread, len(got) > 0, tt.unapplied)
		}
	}
}

func TestPasswordFieldNeverShowsTheSecret(t *testing.T) {
	test.NewTempApp(t)
	field := passwordField("mail_notification_password", "Password", "SMTP password")

	if got := field.format(prefValues{"mail_notification_password": "hunter2"}); got != "••••••••" {
		t.Fatalf("format = %q", got)
	}
	if got := field.format(prefValues{"mail_notification_password": ""}); got != "(empty)" {
		t.Fatalf("format empty = %q", got)
	}
	editor := field.control.newEditor(field).(*entryEditor)
	if !editor.entry.Password {
		t.Fatal("password shown in a plain entry")
	}
	editor.load(prefValues{"mail_notification_password": "hunter2"})
	values := prefValues{}
	if err := editor.store(values); err != nil || values["mail_notification_password"] != "hunter2" {
		t.Fatalf("store = %v, %v", values, err)
	}
}

func TestBoolEnumField(t *testing.T) {
	test.NewTempApp(t)
	field := boolEnumField("auto_tmm_enabled", "Default Torrent Management Mode", "Default management mode", tmmModeOptions())

	_, values := presentFields([]*prefField{field}, rawPrefs(t, `{"auto_tmm_enabled":true}`))
	if got := field.format(values); got != "Automatic" {
		t.Fatalf("format = %q", got)
	}
	editor := field.control.newEditor(field).(*enumEditor[bool])
	editor.load(values)
	editor.selector.SetSelected("Manual")
	stored := prefValues{}
	if err := editor.store(stored); err != nil || stored["auto_tmm_enabled"] != false {
		t.Fatalf("store = %v, %v", stored, err)
	}
	if _, ok := field.control.decode(field, rawPrefs(t, `{"auto_tmm_enabled":1}`)); ok {
		t.Fatal("number accepted by a bool select")
	}
}

func TestDownloadsTabRoundTripHasNoChanges(t *testing.T) {
	test.NewTempApp(t)
	form := newPrefForm(serverPrefTabs(nil), rawPrefs(t, downloadsPrefsDoc()))
	t.Cleanup(form.close)

	for _, key := range []string{"save_path", "torrent_content_layout", "mail_notification_password", "autorun_program", "auto_delete_mode"} {
		if form.fieldByKey(key) == nil {
			t.Fatalf("%s missing from the Downloads tab", key)
		}
	}
	if form.fieldByKey("scan_dirs") != nil {
		t.Fatal("deprecated scan_dirs is shown")
	}
	edited, err := form.edited()
	if err != nil {
		t.Fatalf("edited: %v", err)
	}
	if changed := changedFields(form.fields, form.baseline, edited); len(changed) != 0 {
		t.Fatalf("untouched form reports changes: %v", fieldNames(changed))
	}

	form.editors[form.fieldByKey("save_path")].(*pathEditor).entry.SetText("/srv/torrents")
	edited, err = form.edited()
	if err != nil {
		t.Fatalf("edited: %v", err)
	}
	payload := changesPayload(changedFields(form.fields, form.baseline, edited), edited)
	if !reflect.DeepEqual(payload, map[string]any{"save_path": "/srv/torrents"}) {
		t.Fatalf("payload = %v", payload)
	}
}

func TestDownloadsTabNestedSwitches(t *testing.T) {
	test.NewTempApp(t)
	form := newPrefForm(serverPrefTabs(nil), rawPrefs(t, downloadsPrefsDoc()))
	t.Cleanup(form.close)
	check := func(key string) *checkEditor { return form.editors[form.fieldByKey(key)].(*checkEditor) }
	editor := func(key string) prefEditor { return form.editors[form.fieldByKey(key)] }

	if !editor("temp_path").disabled() {
		t.Fatal("incomplete path enabled while switched off")
	}
	check("temp_path_enabled").check.SetChecked(true)
	if editor("temp_path").disabled() {
		t.Fatal("incomplete path stays disabled")
	}

	check("torrent_files_finished_backup_dir_enabled").check.SetChecked(true)
	if !editor("torrent_files_finished_backup_dir").disabled() {
		t.Fatal("finished backup dir enabled while backups are off")
	}
	check("torrent_files_backup_enabled").check.SetChecked(true)
	if editor("torrent_files_finished_backup_dir").disabled() {
		t.Fatal("finished backup dir disabled with both switches on")
	}

	check("mail_notification_enabled").check.SetChecked(true)
	if editor("mail_notification_smtp").disabled() || !editor("mail_notification_password").disabled() {
		t.Fatal("mail fields: SMTP should be editable, password only with authentication")
	}
	check("mail_notification_auth_enabled").check.SetChecked(true)
	if editor("mail_notification_password").disabled() {
		t.Fatal("password disabled with authentication on")
	}
}

func TestPrefFormCloseStopsPathEntries(t *testing.T) {
	test.NewTempApp(t)
	form := newPrefForm(serverPrefTabs(nil), rawPrefs(t, downloadsPrefsDoc()))

	form.close()
	if !form.editors[form.fieldByKey("save_path")].(*pathEditor).entry.closed {
		t.Fatal("form.close left a path entry running")
	}
}
