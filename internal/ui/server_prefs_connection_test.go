package ui

import (
	"reflect"
	"strconv"
	"testing"

	"fyne.io/fyne/v2/test"
)

func connectionPrefsDoc() string {
	return `{"bittorrent_protocol":0,"listen_port":6881,"random_port":false,"upnp":true,
		"max_connec":500,"max_connec_per_torrent":100,"max_uploads":-1,"max_uploads_per_torrent":4,
		"i2p_enabled":false,"i2p_address":"127.0.0.1","i2p_port":7656,"i2p_mixed_mode":false,"i2p_pex_enabled":false,
		"proxy_type":"None","proxy_ip":"0.0.0.0","proxy_port":8080,"proxy_hostname_lookup":true,
		"proxy_auth_enabled":false,"proxy_username":"","proxy_password":"",
		"proxy_bittorrent":true,"proxy_peer_connections":false,"proxy_rss":true,"proxy_misc":true,
		"ip_filter_enabled":false,"ip_filter_path":"","ip_filter_trackers":false,
		"banned_IPs":"10.0.0.1\n2001:db8::1"}`
}

func TestPortControlBoundsAndRandomButton(t *testing.T) {
	test.NewTempApp(t)
	field := portField("listen_port", "Port", "Listening port", true)
	editor := field.control.newEditor(field).(*entryEditor)

	for text, wantErr := range map[string]bool{"0": false, "6881": false, "65535": false, "65536": true, "-1": true, "port": true} {
		editor.entry.SetText(text)
		if err := editor.store(prefValues{}); (err != nil) != wantErr {
			t.Errorf("store(%q) error %v, wantErr %v", text, err, wantErr)
		}
	}

	random := editor.button
	if random == nil {
		t.Fatal("listening port has no Random button")
	}
	for range 20 {
		test.Tap(random)
		port, err := strconv.Atoi(editor.entry.Text)
		if err != nil || port < 1024 || port > 65535 {
			t.Fatalf("Random picked %q", editor.entry.Text)
		}
	}

	plain := portField("i2p_port", "Port", "I2P port", false)
	if plain.control.newEditor(plain).(*entryEditor).button != nil {
		t.Fatal("Random button on a port that should not have one")
	}
}

func TestUnlimitedIntFieldMinimum(t *testing.T) {
	test.NewTempApp(t)
	field := unlimitedIntField("max_connec", "Connections", "Global maximum connections", 1)
	editor := field.control.newEditor(field).(*unlimitedEditor)
	editor.load(prefValues{"max_connec": int64(500)})

	editor.entry.SetText("0")
	if err := editor.store(prefValues{}); err == nil {
		t.Fatal("0 accepted for a limit the server turns into unlimited")
	}
	editor.entry.SetText("1")
	values := prefValues{}
	if err := editor.store(values); err != nil || values["max_connec"] != int64(1) {
		t.Fatalf("store = %v, %v", values, err)
	}
}

func TestUnappliedFieldsComparesIPListsAsSets(t *testing.T) {
	field := ipListField("banned_IPs", "", "Banned IP addresses")
	tests := []struct {
		sent, reread string
		unapplied    bool
	}{
		{"10.0.0.2\n\n2001:0db8::0001\n10.0.0.2\n", "10.0.0.2\n2001:db8::1", false},
		{"", "", false},
		{" 10.0.0.1 ", "10.0.0.1", false},
		{"bogus\n10.0.0.1", "10.0.0.1", true},
		{"10.0.0.1", "", true},
	}
	for _, tt := range tests {
		got := unappliedFields([]*prefField{field}, prefValues{"banned_IPs": tt.sent}, prefValues{"banned_IPs": tt.reread})
		if (len(got) > 0) != tt.unapplied {
			t.Errorf("sent %q, server has %q: unapplied=%v, want %v", tt.sent, tt.reread, len(got) > 0, tt.unapplied)
		}
	}
}

func TestProxyFieldsFollowProxyType(t *testing.T) {
	test.NewTempApp(t)
	form := newPrefForm(serverPrefTabs(nil), rawPrefs(t, connectionPrefsDoc()))
	t.Cleanup(form.close)
	editor := func(key string) prefEditor { return form.editors[form.fieldByKey(key)] }
	proxyType := editor("proxy_type").(*enumEditor[string]).selector

	for _, key := range []string{"proxy_ip", "proxy_port", "proxy_auth_enabled", "proxy_bittorrent", "proxy_rss"} {
		if !editor(key).disabled() {
			t.Fatalf("%s enabled without a proxy", key)
		}
	}

	proxyType.SetSelected("SOCKS4")
	if editor("proxy_ip").disabled() || editor("proxy_bittorrent").disabled() {
		t.Fatal("SOCKS4 proxy: host and BitTorrent use should be editable")
	}
	if !editor("proxy_auth_enabled").disabled() || !editor("proxy_rss").disabled() || !editor("proxy_hostname_lookup").disabled() {
		t.Fatal("SOCKS4 proxy: authentication, RSS and hostname lookup are not supported")
	}

	proxyType.SetSelected("SOCKS5")
	if editor("proxy_auth_enabled").disabled() || editor("proxy_misc").disabled() {
		t.Fatal("SOCKS5 proxy: authentication and general use should be editable")
	}
	if !editor("proxy_password").disabled() {
		t.Fatal("proxy password editable with authentication off")
	}
	editor("proxy_auth_enabled").(*checkEditor).check.SetChecked(true)
	if editor("proxy_password").disabled() {
		t.Fatal("proxy password disabled with authentication on")
	}

	if editor("proxy_peer_connections").disabled() {
		t.Fatal("peer connections disabled while the proxy is used for BitTorrent")
	}
	editor("proxy_bittorrent").(*checkEditor).check.SetChecked(false)
	if !editor("proxy_peer_connections").disabled() {
		t.Fatal("peer connections editable without BitTorrent proxying")
	}
}

func TestConnectionTabRoundTripAndPortChange(t *testing.T) {
	test.NewTempApp(t)
	form := newPrefForm(serverPrefTabs(nil), rawPrefs(t, connectionPrefsDoc()))
	t.Cleanup(form.close)

	if form.fieldByKey("random_port") != nil {
		t.Fatal("deprecated random_port is editable")
	}
	edited, err := form.edited()
	if err != nil {
		t.Fatalf("edited: %v", err)
	}
	if changed := changedFields(form.fields, form.baseline, edited); len(changed) != 0 {
		t.Fatalf("untouched form reports changes: %v", fieldNames(changed))
	}

	form.editors[form.fieldByKey("listen_port")].(*entryEditor).entry.SetText("51413")
	edited, err = form.edited()
	if err != nil {
		t.Fatalf("edited: %v", err)
	}
	payload := changesPayload(changedFields(form.fields, form.baseline, edited), edited)
	if !reflect.DeepEqual(payload, map[string]any{"listen_port": int64(51413)}) {
		t.Fatalf("payload = %v", payload)
	}
}
