package ui

import (
	"reflect"
	"testing"

	"fyne.io/fyne/v2/test"
)

func behaviorRSSPrefsDoc() string {
	return `{"locale":"en","performance_warning":false,"status_bar_external_ip":false,
		"confirm_torrent_deletion":true,"store_search_jobs":false,"store_search_job_results":false,
		"file_log_enabled":true,"file_log_path":"/var/log/qbt","file_log_backup_enabled":true,
		"file_log_max_size":65,"file_log_delete_old":true,"file_log_age":1,"file_log_age_type":1,
		"delete_torrent_content_files":false,
		"rss_refresh_interval":30,"rss_fetch_delay":2,"rss_max_articles_per_feed":50,
		"rss_processing_enabled":false,"rss_auto_downloading_enabled":false,
		"rss_download_repack_proper_episodes":true,
		"rss_smart_episode_filters":"s(\\d+)e(\\d+)\n(\\d+)x(\\d+)"}`
}

func TestIntRangeFieldEnforcesBothBounds(t *testing.T) {
	test.NewTempApp(t)
	field := intRangeField("file_log_age", "Age", "Log age", "", 1, 365)
	editor := field.control.newEditor(field).(*entryEditor)

	for text, wantErr := range map[string]bool{"1": false, "365": false, "0": true, "366": true} {
		editor.entry.SetText(text)
		if err := editor.store(prefValues{}); (err != nil) != wantErr {
			t.Errorf("store(%q) error %v, wantErr %v", text, err, wantErr)
		}
	}
}

func TestBehaviorAndRSSTabsRoundTrip(t *testing.T) {
	test.NewTempApp(t)
	form := newPrefForm(serverPrefTabs(nil), rawPrefs(t, behaviorRSSPrefsDoc()))
	t.Cleanup(form.close)

	for _, key := range []string{"locale", "file_log_path", "file_log_age_type", "rss_refresh_interval", "rss_smart_episode_filters"} {
		if form.fieldByKey(key) == nil {
			t.Fatalf("%s missing", key)
		}
	}
	if form.fieldByKey("delete_torrent_content_files") != nil {
		t.Fatal("desktop-only delete_torrent_content_files is shown")
	}
	edited, err := form.edited()
	if err != nil {
		t.Fatalf("edited: %v", err)
	}
	if changed := changedFields(form.fields, form.baseline, edited); len(changed) != 0 {
		t.Fatalf("untouched form reports changes: %v", fieldNames(changed))
	}

	form.editors[form.fieldByKey("rss_refresh_interval")].(*entryEditor).entry.SetText("0")
	if _, err := form.edited(); err == nil {
		t.Fatal("RSS refresh interval of 0 minutes accepted")
	}
	form.editors[form.fieldByKey("rss_refresh_interval")].(*entryEditor).entry.SetText("15")
	edited, err = form.edited()
	if err != nil {
		t.Fatalf("edited: %v", err)
	}
	payload := changesPayload(changedFields(form.fields, form.baseline, edited), edited)
	if !reflect.DeepEqual(payload, map[string]any{"rss_refresh_interval": int64(15)}) {
		t.Fatalf("payload = %v", payload)
	}
}

func TestLogFileFieldsFollowTheirSwitches(t *testing.T) {
	test.NewTempApp(t)
	form := newPrefForm(serverPrefTabs(nil), rawPrefs(t, behaviorRSSPrefsDoc()))
	t.Cleanup(form.close)
	editor := func(key string) prefEditor { return form.editors[form.fieldByKey(key)] }
	check := func(key string) *checkEditor { return editor(key).(*checkEditor) }

	if editor("file_log_max_size").disabled() || editor("file_log_age_type").disabled() {
		t.Fatal("log size and age disabled while their switches are on")
	}
	check("file_log_delete_old").check.SetChecked(false)
	if !editor("file_log_age").disabled() || !editor("file_log_age_type").disabled() || editor("file_log_max_size").disabled() {
		t.Fatal("turning off old-log deletion should only disable the age fields")
	}
	check("file_log_enabled").check.SetChecked(false)
	if !editor("file_log_path").disabled() || !editor("file_log_max_size").disabled() {
		t.Fatal("log fields stay enabled with file logging off")
	}
	if !editor("store_search_job_results").disabled() {
		t.Fatal("search results saving enabled without saving search tabs")
	}
}
