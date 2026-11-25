package ui

import (
	"reflect"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

const testServerAPIKey = "qbt_abcdefghijklmnopqrstuvwxyz12"

func webUIPrefsDoc() string {
	return `{"web_ui_address":"*","web_ui_port":8080,"web_ui_upnp":false,"use_https":false,
		"web_ui_https_cert_path":"","web_ui_https_key_path":"",
		"web_ui_username":"admin","bypass_local_auth":false,"bypass_auth_subnet_whitelist_enabled":false,
		"bypass_auth_subnet_whitelist":"10.0.0.0/8\n192.168.1.0/24",
		"web_ui_max_auth_fail_count":5,"web_ui_ban_duration":3600,"web_ui_session_timeout":3600,"web_ui_sessions_count_limit":0,
		"web_ui_api_key":"` + testServerAPIKey + `",
		"alternative_webui_enabled":false,"alternative_webui_path":"",
		"web_ui_clickjacking_protection_enabled":true,"web_ui_csrf_protection_enabled":true,"web_ui_secure_cookie_enabled":true,
		"web_ui_host_header_validation_enabled":true,"web_ui_domain_list":"*",
		"web_ui_use_custom_http_headers_enabled":false,"web_ui_custom_http_headers":"",
		"web_ui_reverse_proxy_enabled":false,"web_ui_reverse_proxies_list":"",
		"dyndns_enabled":false,"dyndns_service":0,"dyndns_username":"","dyndns_password":"","dyndns_domain":"changeme.dyndns.org"}`
}

func newWebUIForm(t *testing.T) *prefForm {
	t.Helper()
	test.NewTempApp(t)
	form := newPrefForm(serverPrefTabs(nil), rawPrefs(t, webUIPrefsDoc()))
	t.Cleanup(form.close)

	return form
}

func TestWebUITabRoundTripShowsWriteOnlyPassword(t *testing.T) {
	form := newWebUIForm(t)

	for _, key := range []string{"web_ui_port", "web_ui_password", "web_ui_api_key", "bypass_auth_subnet_whitelist", "dyndns_password"} {
		if form.fieldByKey(key) == nil {
			t.Fatalf("%s missing", key)
		}
	}
	edited, err := form.edited()
	if err != nil {
		t.Fatalf("edited: %v", err)
	}
	if changed := changedFields(form.fields, form.baseline, edited); len(changed) != 0 {
		t.Fatalf("untouched form reports changes: %v", fieldNames(changed))
	}
}

func TestWebUIPasswordNeedsTheUsernameKey(t *testing.T) {
	test.NewTempApp(t)
	// Servers without WebUI login keys show neither login field.
	form := newPrefForm(serverPrefTabs(nil), rawPrefs(t, `{"web_ui_port":8080}`))
	t.Cleanup(form.close)
	if form.fieldByKey("web_ui_password") != nil {
		t.Fatal("password shown without web_ui_username")
	}
}

func TestWebUIUsernameValidation(t *testing.T) {
	form := newWebUIForm(t)
	username := form.editors[form.fieldByKey("web_ui_username")].(*entryEditor)

	for text, wantErr := range map[string]bool{"ab": true, "adm:in": true, "abc": false, "admin": false} {
		username.entry.SetText(text)
		if _, err := form.edited(); (err != nil) != wantErr {
			t.Errorf("username %q: error %v, wantErr %v", text, err, wantErr)
		}
	}
}

func TestWebUIPasswordIsWriteOnly(t *testing.T) {
	form := newWebUIForm(t)
	field := form.fieldByKey("web_ui_password")
	password := form.editors[field].(*loginPasswordEditor)

	tests := []struct {
		password, repeat string
		wantErr          bool
	}{
		{"", "", false},
		{"secret1", "", true},
		{"", "secret1", true},
		{"secret1", "secret2", true},
		{"short", "short", true},
		{"secret1", "secret1", false},
	}
	for _, tt := range tests {
		password.entry.SetText(tt.password)
		password.repeat.SetText(tt.repeat)
		if _, err := form.edited(); (err != nil) != tt.wantErr {
			t.Errorf("password %q / repeat %q: error %v, wantErr %v", tt.password, tt.repeat, err, tt.wantErr)
		}
	}

	edited, err := form.edited()
	if err != nil {
		t.Fatalf("edited: %v", err)
	}
	changed := changedFields(form.fields, form.baseline, edited)
	if payload := changesPayload(changed, edited); !reflect.DeepEqual(payload, map[string]any{"web_ui_password": "secret1"}) {
		t.Fatalf("payload = %v", payload)
	}
	line := changeSummaryLines(changed, form.baseline, edited, form.baseline, nil)[0]
	if strings.Contains(line, "secret1") {
		t.Fatalf("summary leaks the password: %q", line)
	}
	// The server never returns the password, so it cannot be "not applied".
	if unapplied := unappliedFields(changed, edited, form.baseline); len(unapplied) != 0 {
		t.Fatal("write-only password reported as not applied")
	}

	form.load(form.baseline)
	if password.entry.Text != "" || password.repeat.Text != "" {
		t.Fatal("reloading the form kept the typed password")
	}
}

func TestAPIKeyEditorMasksAndRotates(t *testing.T) {
	form := newWebUIForm(t)
	field := form.fieldByKey("web_ui_api_key")
	editor := form.editors[field].(*apiKeyEditor)

	wantMasked := "qbt_" + strings.Repeat("•", 22) + "wxyz12"
	if editor.display.Text != wantMasked {
		t.Fatalf("shown %q, want %q", editor.display.Text, wantMasked)
	}
	if got := field.format(form.baseline); got != wantMasked {
		t.Fatalf("format = %q", got)
	}
	if editor.rotateButton.Disabled() == false {
		t.Fatal("Rotate is usable before the window wires it")
	}

	rotated := 0
	editor.setRotate(func() { rotated++ })
	if editor.rotateButton.Disabled() || editor.rotateButton.Text != "Rotate…" {
		t.Fatalf("rotate button %q disabled=%v", editor.rotateButton.Text, editor.rotateButton.Disabled())
	}
	test.Tap(editor.rotateButton)
	if rotated != 1 {
		t.Fatal("Rotate did not call the window")
	}

	test.Tap(editor.copyButton)
	if got := fyne.CurrentApp().Clipboard().Content(); got != testServerAPIKey {
		t.Fatalf("copied %q, want the full key", got)
	}

	// After a rotation the window shows the new key and makes it the baseline.
	editor.setKey("qbt_" + strings.Repeat("z", 28))
	form.baseline["web_ui_api_key"] = editor.loaded
	if !strings.HasSuffix(editor.display.Text, "zzzzzz") {
		t.Fatalf("display after rotation %q", editor.display.Text)
	}
	edited, err := form.edited()
	if err != nil {
		t.Fatalf("edited: %v", err)
	}
	if _, ok := changesPayload(changedFields(form.fields, form.baseline, edited), edited)["web_ui_api_key"]; ok {
		t.Fatal("the API key is sent with setPreferences")
	}
}

func TestAPIKeyEditorOffersGenerateWithoutKey(t *testing.T) {
	test.NewTempApp(t)
	field := apiKeyField("web_ui_api_key", "API key", "API key")
	editor := field.control.newEditor(field).(*apiKeyEditor)
	editor.setRotate(func() {})
	editor.load(prefValues{"web_ui_api_key": ""})

	if editor.rotateButton.Text != "Generate…" || !editor.copyButton.Disabled() {
		t.Fatalf("no key: button %q, copy disabled=%v", editor.rotateButton.Text, editor.copyButton.Disabled())
	}
}

func TestRequiredPathOnlyWhileEnabled(t *testing.T) {
	form := newWebUIForm(t)
	https := form.editors[form.fieldByKey("use_https")].(*checkEditor)

	if _, err := form.edited(); err != nil {
		t.Fatalf("empty certificate rejected while HTTPS is off: %v", err)
	}
	https.check.SetChecked(true)
	_, err := form.edited()
	if err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("empty certificate with HTTPS on: error %v", err)
	}
	form.editors[form.fieldByKey("web_ui_https_cert_path")].(*pathEditor).entry.SetText("/etc/qbt/cert.pem")
	form.editors[form.fieldByKey("web_ui_https_key_path")].(*pathEditor).entry.SetText("/etc/qbt/key.pem")
	if _, err := form.edited(); err != nil {
		t.Fatalf("filled HTTPS paths rejected: %v", err)
	}
}

func TestSubnetListEquivalence(t *testing.T) {
	field := subnetListField("bypass_auth_subnet_whitelist", "", "Auth whitelist")
	tests := []struct {
		sent, reread string
		unapplied    bool
	}{
		{"10.0.0.0/8, 192.168.1.0/24", "10.0.0.0/8\n192.168.1.0/24", false},
		{"192.168.1.7/24\n", "192.168.1.0/24", false},
		{"10.0.0.1", "10.0.0.1/32", false},
		{"fdff:ffff:c8::/40", "fdff:ffff:c8::/40", false},
		{"bogus\n10.0.0.0/8", "10.0.0.0/8", true},
		{"10.0.0.0/8", "", true},
	}
	for _, tt := range tests {
		got := unappliedFields([]*prefField{field}, prefValues{field.keys[0]: tt.sent}, prefValues{field.keys[0]: tt.reread})
		if (len(got) > 0) != tt.unapplied {
			t.Errorf("sent %q, server has %q: unapplied=%v, want %v", tt.sent, tt.reread, len(got) > 0, tt.unapplied)
		}
	}
}

func TestWebUIFieldsFollowTheirSwitches(t *testing.T) {
	form := newWebUIForm(t)
	editor := func(key string) prefEditor { return form.editors[form.fieldByKey(key)] }
	check := func(key string) *checkEditor { return editor(key).(*checkEditor) }

	for _, key := range []string{"web_ui_https_cert_path", "bypass_auth_subnet_whitelist", "alternative_webui_path",
		"web_ui_custom_http_headers", "web_ui_reverse_proxies_list", "dyndns_service", "dyndns_password"} {
		if !editor(key).disabled() {
			t.Errorf("%s enabled while its switch is off", key)
		}
	}
	if editor("web_ui_domain_list").disabled() {
		t.Fatal("server domains disabled while Host header validation is on")
	}
	check("web_ui_host_header_validation_enabled").check.SetChecked(false)
	if !editor("web_ui_domain_list").disabled() {
		t.Fatal("server domains editable without Host header validation")
	}
	check("dyndns_enabled").check.SetChecked(true)
	if editor("dyndns_domain").disabled() || editor("dyndns_username").disabled() {
		t.Fatal("dynamic DNS fields stay disabled")
	}
}

func TestDisconnectingFieldsAreMarked(t *testing.T) {
	form := newWebUIForm(t)
	want := map[string]bool{
		"web_ui_address": true, "web_ui_port": true, "use_https": true,
		"web_ui_https_cert_path": true, "web_ui_https_key_path": true,
		"web_ui_host_header_validation_enabled": true, "web_ui_domain_list": true,
	}
	for _, field := range form.fields {
		if field.disconnects != want[field.keys[0]] {
			t.Errorf("%s: disconnects=%v", field.keys[0], field.disconnects)
		}
	}
}
