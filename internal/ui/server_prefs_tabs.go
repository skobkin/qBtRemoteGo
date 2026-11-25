package ui

// Tab descriptors of the Server settings window. They mirror the qBittorrent
// WebUI Options tabs; the key-to-field map is in docs/qbittorrent/ and on
// issue #32.

// serverPrefTabs lists the implemented tabs in WebUI order.
func serverPrefTabs() []prefTab {
	return []prefTab{speedPrefTab(), bitTorrentPrefTab()}
}

// requiring makes field editable only while all the given switches are on.
func requiring(field *prefField, switches ...string) *prefField {
	field.requires = switches
	return field
}

func schedulerDayOptions() []prefOption[int64] {
	return []prefOption[int64]{
		{0, "Every day"}, {1, "Weekdays"}, {2, "Weekends"},
		{3, "Monday"}, {4, "Tuesday"}, {5, "Wednesday"}, {6, "Thursday"},
		{7, "Friday"}, {8, "Saturday"}, {9, "Sunday"},
	}
}

func speedPrefTab() prefTab {
	const schedule = "scheduler_enabled"

	return prefTab{title: "Speed", sections: []prefSection{
		{title: "Global Rate Limits", hint: "0 means unlimited", fields: []*prefField{
			rateLimitField("up_limit", "Upload (KiB/s)", "Global upload limit"),
			rateLimitField("dl_limit", "Download (KiB/s)", "Global download limit"),
		}},
		{title: "Alternative Rate Limits", hint: "0 means unlimited", fields: []*prefField{
			rateLimitField("alt_up_limit", "Upload (KiB/s)", "Alternative upload limit"),
			rateLimitField("alt_dl_limit", "Download (KiB/s)", "Alternative download limit"),
			checkField(schedule, "Use on a schedule", "Alternative limits schedule"),
			requiring(timeField("schedule_from_hour", "schedule_from_min", "From", "Alternative limits schedule start"), schedule),
			requiring(timeField("schedule_to_hour", "schedule_to_min", "To", "Alternative limits schedule end"), schedule),
			requiring(enumField("scheduler_days", "When", "Alternative limits schedule days", schedulerDayOptions()), schedule),
		}},
		{title: "Rate Limits Settings", fields: []*prefField{
			checkField("limit_utp_rate", "Apply rate limit to µTP protocol", "Rate limit µTP"),
			checkField("limit_tcp_overhead", "Apply rate limit to transport overhead", "Rate limit transport overhead"),
			checkField("limit_lan_peers", "Apply rate limit to peers on LAN", "Rate limit LAN peers"),
		}},
	}}
}

func encryptionOptions() []prefOption[int64] {
	return []prefOption[int64]{{0, "Allow encryption"}, {1, "Require encryption"}, {2, "Disable encryption"}}
}

func shareLimitsModeOptions() []prefOption[string] {
	return []prefOption[string]{{"MatchAny", "Any of the above"}, {"MatchAll", "All the above"}}
}

func shareLimitActionOptions() []prefOption[int64] {
	return []prefOption[int64]{
		{0, "Stop torrent"}, {1, "Remove torrent"},
		{3, "Remove torrent and its files"}, {2, "Enable super seeding for torrent"},
	}
}

func bitTorrentPrefTab() prefTab {
	const (
		queueing    = "queueing_enabled"
		ignoreSlow  = "dont_count_slow_torrents"
		addTrackers = "add_trackers_enabled"
		fromURL     = "add_trackers_from_url_enabled"
	)

	return prefTab{title: "BitTorrent", sections: []prefSection{
		{title: "Privacy", fields: []*prefField{
			checkField("dht", "Enable DHT (decentralized network) to find more peers", "DHT"),
			checkField("pex", "Enable Peer Exchange (PeX) to find more peers", "Peer Exchange"),
			checkField("lsd", "Enable Local Peer Discovery to find more peers", "Local Peer Discovery"),
			enumField("encryption", "Encryption mode", "Encryption mode", encryptionOptions()),
			checkField("anonymous_mode", "Enable anonymous mode", "Anonymous mode"),
		}},
		{title: "Torrent Checking", fields: []*prefField{
			unlimitedIntField("max_active_checking_torrents", "Max active checking torrents", "Max active checking torrents"),
		}},
		{title: "Torrent Queueing", fields: []*prefField{
			checkField(queueing, "Enable torrent queueing", "Torrent queueing"),
			requiring(unlimitedIntField("max_active_downloads", "Maximum active downloads", "Maximum active downloads"), queueing),
			requiring(unlimitedIntField("max_active_uploads", "Maximum active uploads", "Maximum active uploads"), queueing),
			requiring(unlimitedIntField("max_active_torrents", "Maximum active torrents", "Maximum active torrents"), queueing),
			requiring(checkField(ignoreSlow, "Do not count slow torrents in these limits", "Ignore slow torrents in queue limits"), queueing),
			requiring(intField("slow_torrent_dl_rate_threshold", "Download rate threshold (KiB/s)", "Slow torrent download threshold", "KiB/s", 1), queueing, ignoreSlow),
			requiring(intField("slow_torrent_ul_rate_threshold", "Upload rate threshold (KiB/s)", "Slow torrent upload threshold", "KiB/s", 1), queueing, ignoreSlow),
			requiring(intField("slow_torrent_inactive_timer", "Torrent inactivity timer (s)", "Slow torrent inactivity timer", "s", 1), queueing, ignoreSlow),
		}},
		{title: "Seeding Limits", fields: []*prefField{
			optionalRatioField("max_ratio_enabled", "max_ratio", "When ratio reaches", "Ratio limit"),
			optionalMinutesField("max_seeding_time_enabled", "max_seeding_time", "When total seeding time reaches", "Seeding time limit"),
			optionalMinutesField("max_inactive_seeding_time_enabled", "max_inactive_seeding_time", "When inactive seeding time reaches", "Inactive seeding time limit"),
			textEnumField("share_limits_mode", "Apply when", "Seeding limits mode", shareLimitsModeOptions()),
			enumField("max_ratio_act", "then", "Seeding limit action", shareLimitActionOptions()),
		}},
		{title: "Automatically Add Trackers", fields: []*prefField{
			checkField(addTrackers, "Append these trackers to new downloads", "Append trackers"),
			requiring(textAreaField("add_trackers", "Trackers", "Trackers to append"), addTrackers),
			checkField(fromURL, "Append trackers from URL to new downloads", "Append trackers from URL"),
			requiring(textField("add_trackers_url", "URL", "Trackers URL"), fromURL),
			readOnlyTextField("add_trackers_url_list", "Fetched trackers", "Fetched trackers"),
		}},
	}}
}
