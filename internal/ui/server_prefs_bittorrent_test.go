package ui

import (
	"reflect"
	"testing"

	"fyne.io/fyne/v2/test"
)

func bitTorrentPrefsDoc() string {
	return `{"dht":true,"pex":true,"lsd":false,"encryption":0,"anonymous_mode":false,
		"max_active_checking_torrents":1,
		"queueing_enabled":true,"max_active_downloads":3,"max_active_uploads":-1,"max_active_torrents":5,
		"dont_count_slow_torrents":false,"slow_torrent_dl_rate_threshold":2,"slow_torrent_ul_rate_threshold":2,
		"slow_torrent_inactive_timer":60,
		"max_ratio_enabled":false,"max_ratio":-1,"max_seeding_time_enabled":true,"max_seeding_time":1440,
		"max_inactive_seeding_time_enabled":false,"max_inactive_seeding_time":-1,
		"share_limits_mode":"MatchAny","max_ratio_act":0,
		"add_trackers_enabled":false,"add_trackers":"udp://a.example:80/announce\nudp://b.example:80/announce",
		"add_trackers_from_url_enabled":false,"add_trackers_url":"","add_trackers_url_list":"udp://c.example:80/announce"}`
}

func TestUnlimitedIntControl(t *testing.T) {
	test.NewTempApp(t)
	field := unlimitedIntField("max_active_uploads", "Uploads", "Maximum active uploads", 0)
	editor := field.control.newEditor(field).(*unlimitedEditor)
	editor.setEnabled(true)

	editor.load(prefValues{"max_active_uploads": int64(-1)})
	if !editor.unlimited.Checked || !editor.entry.Disabled() {
		t.Fatal("-1 should show as Unlimited with the count disabled")
	}
	values := prefValues{}
	if err := editor.store(values); err != nil || values["max_active_uploads"] != int64(-1) {
		t.Fatalf("store unlimited = %v, %v", values, err)
	}

	editor.unlimited.SetChecked(false)
	if editor.entry.Disabled() {
		t.Fatal("count stays disabled after unticking Unlimited")
	}
	tests := []struct {
		text    string
		want    int64
		wantErr bool
	}{
		{"0", 0, false},
		{" 12 ", 12, false},
		{"2147483647", 2147483647, false},
		{"2147483648", 0, true},
		{"-1", 0, true},
		{"", 0, true},
		{"1.5", 0, true},
	}
	for _, tt := range tests {
		editor.entry.SetText(tt.text)
		values := prefValues{}
		err := editor.store(values)
		if (err != nil) != tt.wantErr {
			t.Fatalf("store(%q) error %v, wantErr %v", tt.text, err, tt.wantErr)
		}
		if !tt.wantErr && values["max_active_uploads"] != tt.want {
			t.Fatalf("store(%q) = %v, want %d", tt.text, values["max_active_uploads"], tt.want)
		}
	}

	editor.load(prefValues{"max_active_uploads": int64(7)})
	if editor.unlimited.Checked || editor.entry.Text != "7" {
		t.Fatalf("loaded 7 as unlimited=%v text=%q", editor.unlimited.Checked, editor.entry.Text)
	}
	editor.setEnabled(false)
	if !editor.disabled() || !editor.unlimited.Disabled() || !editor.entry.Disabled() {
		t.Fatal("disabling the field leaves parts editable")
	}
}

func TestOptionalRatioControl(t *testing.T) {
	test.NewTempApp(t)
	field := optionalRatioField("max_ratio_enabled", "max_ratio", "When ratio reaches", "Ratio limit")
	editor := field.control.newEditor(field).(*optionalLimitEditor)
	editor.setEnabled(true)

	off := prefValues{"max_ratio_enabled": false, "max_ratio": float64(-1)}
	editor.load(off)
	if editor.check.Checked || !editor.entry.Disabled() || editor.entry.Text != "1" {
		t.Fatalf("off limit shown as checked=%v disabled=%v text=%q", editor.check.Checked, editor.entry.Disabled(), editor.entry.Text)
	}
	values := prefValues{}
	if err := editor.store(values); err != nil || !reflect.DeepEqual(values, off) {
		t.Fatalf("store off = %v, %v; want the server's values", values, err)
	}

	editor.check.SetChecked(true)
	tests := []struct {
		text    string
		want    float64
		wantErr bool
	}{
		{"1.5", 1.5, false},
		{"0", 0, false},
		{" 2 ", 2, false},
		{"-1", 0, true},
		{"abc", 0, true},
		{"NaN", 0, true},
		{"Inf", 0, true},
		{"", 0, true},
	}
	for _, tt := range tests {
		editor.entry.SetText(tt.text)
		values := prefValues{}
		err := editor.store(values)
		if (err != nil) != tt.wantErr {
			t.Fatalf("store(%q) error %v, wantErr %v", tt.text, err, tt.wantErr)
		}
		if !tt.wantErr && !reflect.DeepEqual(values, prefValues{"max_ratio_enabled": true, "max_ratio": tt.want}) {
			t.Fatalf("store(%q) = %v", tt.text, values)
		}
	}

	editor.load(prefValues{"max_ratio_enabled": true, "max_ratio": 2.5})
	if !editor.check.Checked || editor.entry.Text != "2.5" {
		t.Fatalf("loaded 2.5 as checked=%v text=%q", editor.check.Checked, editor.entry.Text)
	}
	// Turning a limit off sends -1, as the WebUI does.
	editor.check.SetChecked(false)
	values = prefValues{}
	if err := editor.store(values); err != nil || !reflect.DeepEqual(values, prefValues{"max_ratio_enabled": false, "max_ratio": float64(-1)}) {
		t.Fatalf("store after turning off = %v, %v", values, err)
	}
}

func TestOptionalLimitKeepsServerValueWhileOff(t *testing.T) {
	test.NewTempApp(t)
	field := optionalMinutesField("max_seeding_time_enabled", "max_seeding_time", "Seeding time", "Seeding time limit")
	editor := field.control.newEditor(field).(*optionalLimitEditor)

	// An older server may report a stale value for a disabled limit; an
	// untouched field must not turn that into a change.
	loaded := prefValues{"max_seeding_time_enabled": false, "max_seeding_time": int64(30)}
	editor.load(loaded)
	if editor.entry.Text != "1440" {
		t.Fatalf("off minutes limit shows %q, want the WebUI default 1440", editor.entry.Text)
	}
	values := prefValues{}
	if err := editor.store(values); err != nil || !reflect.DeepEqual(values, loaded) {
		t.Fatalf("store = %v, %v", values, err)
	}

	editor.check.SetChecked(true)
	editor.entry.SetText("90")
	values = prefValues{}
	if err := editor.store(values); err != nil || !reflect.DeepEqual(values, prefValues{"max_seeding_time_enabled": true, "max_seeding_time": int64(90)}) {
		t.Fatalf("store on = %v, %v", values, err)
	}
	editor.entry.SetText("1.5")
	if err := editor.store(prefValues{}); err == nil {
		t.Fatal("fractional minutes accepted")
	}
}

func TestIntControlEnforcesMinimum(t *testing.T) {
	test.NewTempApp(t)
	field := intField("slow_torrent_inactive_timer", "Timer", "Inactivity timer", "s", 1)
	editor := field.control.newEditor(field).(*entryEditor)

	editor.load(prefValues{"slow_torrent_inactive_timer": int64(60)})
	if editor.entry.Text != "60" {
		t.Fatalf("shown %q", editor.entry.Text)
	}
	for text, wantErr := range map[string]bool{"1": false, "0": true, "-5": true, "x": true, "2147483648": true} {
		editor.entry.SetText(text)
		if err := editor.store(prefValues{}); (err != nil) != wantErr {
			t.Errorf("store(%q) error %v, wantErr %v", text, err, wantErr)
		}
	}
}

func TestTextEnumControlKeepsUnknownValues(t *testing.T) {
	test.NewTempApp(t)
	field := textEnumField("share_limits_mode", "Apply when", "Seeding limits mode", shareLimitsModeOptions())
	prefs := rawPrefs(t, `{"share_limits_mode":"MatchFuture"}`)

	present, values := presentFields([]*prefField{field}, prefs)
	if len(present) != 1 || values["share_limits_mode"] != "MatchFuture" {
		t.Fatalf("decoded %v", values)
	}
	editor := field.control.newEditor(field).(*enumEditor[string])
	editor.load(values)
	stored := prefValues{}
	if err := editor.store(stored); err != nil || stored["share_limits_mode"] != "MatchFuture" {
		t.Fatalf("unknown value not preserved: %v, %v", stored, err)
	}
	editor.selector.SetSelected("All the above")
	stored = prefValues{}
	if err := editor.store(stored); err != nil || stored["share_limits_mode"] != "MatchAll" {
		t.Fatalf("store = %v, %v", stored, err)
	}

	if _, ok := field.control.decode(field, rawPrefs(t, `{"share_limits_mode":1}`)); ok {
		t.Fatal("numeric value accepted by a text enum")
	}
}

func TestNewControlFormatting(t *testing.T) {
	tests := []struct {
		field  *prefField
		values prefValues
		want   string
	}{
		{unlimitedIntField("n", "", "", 0), prefValues{"n": int64(-1)}, "Unlimited"},
		{unlimitedIntField("n", "", "", 0), prefValues{"n": int64(4)}, "4"},
		{intField("n", "", "", "KiB/s", 1), prefValues{"n": int64(5)}, "5 KiB/s"},
		{intField("n", "", "", "", 0), prefValues{"n": int64(5)}, "5"},
		{optionalRatioField("e", "v", "", ""), prefValues{"e": false, "v": float64(-1)}, "Off"},
		{optionalRatioField("e", "v", "", ""), prefValues{"e": true, "v": 1.25}, "1.25"},
		{optionalMinutesField("e", "v", "", ""), prefValues{"e": true, "v": int64(1440)}, "1440 min"},
		{textEnumField("m", "", "", shareLimitsModeOptions()), prefValues{"m": "MatchAll"}, "All the above"},
		{textField("u", "", ""), prefValues{"u": ""}, "(empty)"},
		{textField("u", "", ""), prefValues{"u": "https://x.example/list"}, `"https://x.example/list"`},
		{textAreaField("t", "", ""), prefValues{"t": "a\nb\n"}, "2 lines"},
		{textAreaField("t", "", ""), prefValues{"t": "a"}, "1 line"},
	}
	for _, tt := range tests {
		if got := tt.field.format(tt.values); got != tt.want {
			t.Errorf("format(%v) = %q, want %q", tt.values, got, tt.want)
		}
	}
}

func TestBitTorrentTabRoundTripHasNoChanges(t *testing.T) {
	test.NewTempApp(t)
	form := newPrefForm(serverPrefTabs(nil), rawPrefs(t, bitTorrentPrefsDoc()))

	if len(form.fields) == 0 || form.fieldByKey("max_ratio") == nil || form.fieldByKey("add_trackers_url_list") == nil {
		t.Fatalf("BitTorrent fields missing: %v", fieldNames(form.fields))
	}
	edited, err := form.edited()
	if err != nil {
		t.Fatalf("edited: %v", err)
	}
	if changed := changedFields(form.fields, form.baseline, edited); len(changed) != 0 {
		t.Fatalf("untouched form reports changes: %v", fieldNames(changed))
	}
}

func TestSeedingLimitSendsSwitchAndValueTogether(t *testing.T) {
	test.NewTempApp(t)
	form := newPrefForm(serverPrefTabs(nil), rawPrefs(t, bitTorrentPrefsDoc()))

	ratio := form.editors[form.fieldByKey("max_ratio")].(*optionalLimitEditor)
	ratio.check.SetChecked(true)
	ratio.entry.SetText("2")

	edited, err := form.edited()
	if err != nil {
		t.Fatalf("edited: %v", err)
	}
	changed := changedFields(form.fields, form.baseline, edited)
	want := map[string]any{"max_ratio_enabled": true, "max_ratio": float64(2)}
	if got := changesPayload(changed, edited); !reflect.DeepEqual(got, want) {
		t.Fatalf("payload = %v, want %v", got, want)
	}
}

func TestSlowTorrentFieldsRequireQueueingAndSwitch(t *testing.T) {
	test.NewTempApp(t)
	form := newPrefForm(serverPrefTabs(nil), rawPrefs(t, bitTorrentPrefsDoc()))

	threshold := form.editors[form.fieldByKey("slow_torrent_dl_rate_threshold")]
	downloads := form.editors[form.fieldByKey("max_active_downloads")]
	dontCount := form.editors[form.fieldByKey("dont_count_slow_torrents")].(*checkEditor).check
	queueing := form.editors[form.fieldByKey("queueing_enabled")].(*checkEditor).check

	if !threshold.disabled() {
		t.Fatal("threshold enabled while slow torrents are counted")
	}
	dontCount.SetChecked(true)
	if threshold.disabled() {
		t.Fatal("threshold disabled with queueing on and slow torrents ignored")
	}
	queueing.SetChecked(false)
	if !threshold.disabled() || !downloads.disabled() {
		t.Fatal("queueing fields stay enabled with queueing off")
	}
}

func TestFetchedTrackersAreReadOnly(t *testing.T) {
	test.NewTempApp(t)
	form := newPrefForm(serverPrefTabs(nil), rawPrefs(t, bitTorrentPrefsDoc()))

	list := form.editors[form.fieldByKey("add_trackers_url_list")]
	form.editors[form.fieldByKey("add_trackers_from_url_enabled")].(*checkEditor).check.SetChecked(true)
	if !list.disabled() {
		t.Fatal("fetched trackers became editable")
	}
	list.(*readOnlyTextEditor).entry.SetText("udp://edited.example:80/announce")
	edited, err := form.edited()
	if err != nil {
		t.Fatalf("edited: %v", err)
	}
	for _, field := range changedFields(form.fields, form.baseline, edited) {
		if field.keys[0] == "add_trackers_url_list" {
			t.Fatal("read-only field reported as changed")
		}
	}
}

func TestServerPrefTabsFollowWebUIOrder(t *testing.T) {
	var titles []string
	for _, tab := range serverPrefTabs(nil) {
		titles = append(titles, tab.title)
	}
	if want := []string{"Behavior", "Downloads", "Connection", "Speed", "BitTorrent", "RSS", "Advanced"}; !reflect.DeepEqual(titles, want) {
		t.Fatalf("tabs = %v, want %v", titles, want)
	}
}
