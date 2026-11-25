package ui

import (
	"reflect"
	"testing"

	"fyne.io/fyne/v2/test"
)

func advancedPrefsDoc() string {
	return `{"async_io_threads":10,"hashing_threads":1,"file_pool_size":100,"checking_memory_use":32,
		"disk_cache":-1,"disk_cache_ttl":60,"enable_coalesce_read_write":true,
		"disk_queue_size":1048576,"disk_io_type":0,"disk_io_read_mode":1,"disk_io_write_mode":1,
		"enable_piece_extent_affinity":false,
		"connection_speed":30,"socket_send_buffer_size":0,"socket_receive_buffer_size":1000,"socket_backlog_size":30,
		"outgoing_ports_min":0,"outgoing_ports_max":0,"utp_tcp_mixed_mode":0,
		"enable_multi_connections_from_same_ip":false,"block_peers_on_privileged_ports":false,
		"reannounce_when_address_changed":false,"announce_to_all_trackers":false,"announce_to_all_tiers":true,
		"announce_ip":"","announce_port":0,"max_concurrent_http_announces":50,"stop_tracker_timeout":2,
		"peer_turnover":4,"peer_turnover_cutoff":90,"peer_turnover_interval":300}`
}

func TestKiBControlConvertsAndKeepsUntouchedBytes(t *testing.T) {
	test.NewTempApp(t)
	field := kibField("socket_receive_buffer_size", "Receive buffer (KiB)", "Socket receive buffer")
	editor := field.control.newEditor(field).(*entryEditor)

	// 1000 bytes is not a whole KiB: shown rounded down, but an untouched
	// box keeps the server's exact value.
	editor.load(prefValues{"socket_receive_buffer_size": int64(1000)})
	if editor.entry.Text != "0" {
		t.Fatalf("shown %q, want 0 KiB", editor.entry.Text)
	}
	values := prefValues{}
	if err := editor.store(values); err != nil || values["socket_receive_buffer_size"] != int64(1000) {
		t.Fatalf("untouched store = %v, %v", values, err)
	}

	editor.entry.SetText("64")
	values = prefValues{}
	if err := editor.store(values); err != nil || values["socket_receive_buffer_size"] != int64(65536) {
		t.Fatalf("store 64 KiB = %v, %v", values, err)
	}
	editor.entry.SetText("-1")
	if err := editor.store(prefValues{}); err == nil {
		t.Fatal("negative size accepted")
	}

	if got := field.format(prefValues{"socket_receive_buffer_size": int64(65536)}); got != "64 KiB" {
		t.Fatalf("format = %q", got)
	}
}

func TestAdvancedTabCuratedSubset(t *testing.T) {
	test.NewTempApp(t)
	form := newPrefForm(serverPrefTabs(nil), rawPrefs(t, advancedPrefsDoc()))
	t.Cleanup(form.close)

	for _, key := range []string{"disk_cache", "disk_cache_ttl", "enable_coalesce_read_write"} {
		if form.fieldByKey(key) != nil {
			t.Fatalf("libtorrent 1.x-only %s is shown", key)
		}
	}
	for _, key := range []string{"disk_io_type", "disk_queue_size", "outgoing_ports_max", "announce_ip", "peer_turnover_interval"} {
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

	turnover := form.editors[form.fieldByKey("peer_turnover")].(*entryEditor)
	turnover.entry.SetText("101")
	if _, err := form.edited(); err == nil {
		t.Fatal("peer turnover above 100% accepted")
	}
	turnover.entry.SetText("10")
	form.editors[form.fieldByKey("disk_queue_size")].(*entryEditor).entry.SetText("2048")
	edited, err = form.edited()
	if err != nil {
		t.Fatalf("edited: %v", err)
	}
	payload := changesPayload(changedFields(form.fields, form.baseline, edited), edited)
	want := map[string]any{"peer_turnover": int64(10), "disk_queue_size": int64(2048 * 1024)}
	if !reflect.DeepEqual(payload, want) {
		t.Fatalf("payload = %v, want %v", payload, want)
	}
}

func TestAnnounceIPNormalizationIsNotReported(t *testing.T) {
	field := ipAddressField("announce_ip", "", "Announce IP")
	if got := unappliedFields([]*prefField{field}, prefValues{"announce_ip": "2001:0db8::0001"}, prefValues{"announce_ip": "2001:db8::1"}); len(got) != 0 {
		t.Fatal("normalized IPv6 reported as unapplied")
	}
	if got := unappliedFields([]*prefField{field}, prefValues{"announce_ip": "not-an-ip"}, prefValues{"announce_ip": ""}); len(got) != 1 {
		t.Fatal("rejected address not reported")
	}
}
