package ui

// Tab descriptors of the Server settings window. They mirror the qBittorrent
// WebUI Options tabs; the key-to-field map is in docs/qbittorrent/ and on
// issue #32.

// serverPrefTabs lists the implemented tabs in WebUI order. dirs suggests
// directories on the server for path fields (nil disables suggestions).
func serverPrefTabs(dirs autocompleteFetcher) []prefTab {
	return []prefTab{
		behaviorPrefTab(dirs), downloadsPrefTab(dirs), connectionPrefTab(dirs),
		speedPrefTab(), bitTorrentPrefTab(), rssPrefTab(),
	}
}

// requiring makes field editable only while all the given checkboxes are on.
func requiring(field *prefField, switches ...string) *prefField {
	for _, key := range switches {
		field.requires = append(field.requires, on(key))
	}
	return field
}

// requiringWhen makes field editable only while all conditions hold.
func requiringWhen(field *prefField, conds ...prefCondition) *prefField {
	field.requires = append(field.requires, conds...)
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
			unlimitedIntField("max_active_checking_torrents", "Max active checking torrents", "Max active checking torrents", 0),
		}},
		{title: "Torrent Queueing", fields: []*prefField{
			checkField(queueing, "Enable torrent queueing", "Torrent queueing"),
			requiring(unlimitedIntField("max_active_downloads", "Maximum active downloads", "Maximum active downloads", 0), queueing),
			requiring(unlimitedIntField("max_active_uploads", "Maximum active uploads", "Maximum active uploads", 0), queueing),
			requiring(unlimitedIntField("max_active_torrents", "Maximum active torrents", "Maximum active torrents", 0), queueing),
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

func contentLayoutOptions() []prefOption[string] {
	return []prefOption[string]{{"Original", "Original"}, {"Subfolder", "Create subfolder"}, {"NoSubfolder", "Don't create subfolder"}}
}

func stopConditionOptions() []prefOption[string] {
	return []prefOption[string]{{"None", "None"}, {"MetadataReceived", "Metadata received"}, {"FilesChecked", "Files checked"}}
}

func autoDeleteModeOptions() []prefOption[int64] {
	return []prefOption[int64]{{0, "Never"}, {1, "If added"}, {2, "Always (also when addition is cancelled)"}}
}

func tmmModeOptions() []prefOption[bool] {
	return []prefOption[bool]{{false, "Manual"}, {true, "Automatic"}}
}

func relocateTorrentOptions() []prefOption[bool] {
	return []prefOption[bool]{{true, "Relocate torrent"}, {false, "Switch torrent to Manual Mode"}}
}

func relocateAffectedOptions() []prefOption[bool] {
	return []prefOption[bool]{{true, "Relocate affected torrents"}, {false, "Switch affected torrents to Manual Mode"}}
}

func smtpEncryptionOptions() []prefOption[string] {
	return []prefOption[string]{{"None", "None"}, {"STARTTLS", "STARTTLS"}, {"SMTPS", "SMTPS"}}
}

const autorunParametersHint = `Supported parameters (case sensitive): %N torrent name, %L category, ` +
	`%G tags (comma separated), %F content path, %R root path, %D save path, %C number of files, ` +
	`%Z torrent size (bytes), %T current tracker, %I info hash v1, %J info hash v2, %K torrent ID, ` +
	`%M comment. Quote parameters to keep whitespace, e.g. "%N".`

// downloadsPrefTab mirrors the WebUI Downloads tab. "Automatically add
// torrents from" (scan_dirs, deprecated) is not shown.
func downloadsPrefTab(dirs autocompleteFetcher) prefTab {
	const (
		backup         = "torrent_files_backup_enabled"
		finishedBackup = "torrent_files_finished_backup_dir_enabled"
		tempPath       = "temp_path_enabled"
		excluded       = "excluded_file_names_enabled"
		mail           = "mail_notification_enabled"
		mailAuth       = "mail_notification_auth_enabled"
		runOnAdded     = "autorun_on_torrent_added_enabled"
		runOnFinished  = "autorun_enabled"
	)

	return prefTab{title: "Downloads", sections: []prefSection{
		{title: "When adding a torrent", fields: []*prefField{
			textEnumField("torrent_content_layout", "Torrent content layout", "Torrent content layout", contentLayoutOptions()),
			checkField("add_to_top_of_queue", "Add to top of queue", "Add to top of queue"),
			checkField("add_stopped_enabled", "Do not start the download automatically", "Add torrents stopped"),
			textEnumField("torrent_stop_condition", "Torrent stop condition", "Torrent stop condition", stopConditionOptions()),
		}},
		{title: "When adding a duplicate torrent", fields: []*prefField{
			checkField("merge_trackers", "Merge trackers to existing torrent", "Merge trackers of duplicates"),
		}},
		{title: "Torrent files", fields: []*prefField{
			checkField(backup, "Store backup .torrent file", "Store .torrent backups"),
			requiring(pathField("torrent_files_backup_dir", "Store backup in", "Backup .torrent folder", dirs), backup),
			requiring(checkField(finishedBackup, "Move backup when torrent finished", "Move finished .torrent backups"), backup),
			requiring(pathField("torrent_files_finished_backup_dir", "Finished backup folder", "Finished .torrent backup folder", dirs), backup, finishedBackup),
			requiring(checkField("remove_torrent_file_backup", "Remove backup when removing torrent", "Remove .torrent backup with torrent"), backup),
			enumField("auto_delete_mode", "Delete .torrent files afterwards", "Delete .torrent files", autoDeleteModeOptions()),
		}},
		{title: "Files", fields: []*prefField{
			checkField("preallocate_all", "Pre-allocate disk space for all files", "Pre-allocate disk space"),
			checkField("incomplete_files_ext", "Append .!qB extension to incomplete files", "Append .!qB extension"),
			checkField("use_unwanted_folder", `Keep unselected files in ".unwanted" folder`, "Keep unselected files in .unwanted"),
		}},
		{title: "Saving Management", fields: []*prefField{
			boolEnumField("auto_tmm_enabled", "Default Torrent Management Mode", "Default torrent management mode", tmmModeOptions()),
			boolEnumField("torrent_changed_tmm_enabled", "When Torrent Category changed", "When torrent category changed", relocateTorrentOptions()),
			boolEnumField("save_path_changed_tmm_enabled", "When Default Save Path changed", "When default save path changed", relocateAffectedOptions()),
			boolEnumField("category_changed_tmm_enabled", "When Category Save Path changed", "When category save path changed", relocateAffectedOptions()),
			checkField("use_category_paths_in_manual_mode", "Use Category paths in Manual Mode", "Use category paths in manual mode"),
			pathField("save_path", "Default Save Path", "Default save path", dirs),
			checkField(tempPath, "Use another path for incomplete torrents", "Separate path for incomplete torrents"),
			requiring(pathField("temp_path", "Incomplete torrents path", "Incomplete torrents path", dirs), tempPath),
		}},
		{title: "Excluded file names", hint: "One name or wildcard pattern per line.", fields: []*prefField{
			checkField(excluded, "Exclude matching file names", "Excluded file names"),
			requiring(textAreaField("excluded_file_names", "Excluded names", "Excluded file name list"), excluded),
		}},
		{title: "Email notification upon download completion", fields: []*prefField{
			checkField(mail, "Send email notifications", "Email notification"),
			requiring(textField("mail_notification_sender", "From", "Notification sender"), mail),
			requiring(textField("mail_notification_email", "To", "Notification recipients"), mail),
			requiring(textField("mail_notification_smtp", "SMTP server", "SMTP server"), mail),
			requiring(textEnumField("mail_notification_encryption_type", "SMTP encryption", "SMTP encryption", smtpEncryptionOptions()), mail),
			requiring(checkField(mailAuth, "Authentication", "SMTP authentication"), mail),
			requiring(textField("mail_notification_username", "Username", "SMTP username"), mail, mailAuth),
			requiring(passwordField("mail_notification_password", "Password", "SMTP password"), mail, mailAuth),
		}},
		{title: "Run external program", hint: autorunParametersHint, fields: []*prefField{
			checkField(runOnAdded, "Run on torrent added", "Run program on torrent added"),
			requiring(textField("autorun_on_torrent_added_program", "Command", "Command on torrent added"), runOnAdded),
			checkField(runOnFinished, "Run on torrent finished", "Run program on torrent finished"),
			requiring(textField("autorun_program", "Command", "Command on torrent finished"), runOnFinished),
		}},
	}}
}

func btProtocolOptions() []prefOption[int64] {
	return []prefOption[int64]{{0, "TCP and μTP"}, {1, "TCP"}, {2, "μTP"}}
}

func proxyTypeOptions() []prefOption[string] {
	return []prefOption[string]{{"None", "(None)"}, {"SOCKS4", "SOCKS4"}, {"SOCKS5", "SOCKS5"}, {"HTTP", "HTTP"}}
}

// connectionPrefTab mirrors the WebUI Connection tab. The deprecated
// random_port is never sent: it would override listen_port.
func connectionPrefTab(dirs autocompleteFetcher) prefTab {
	const (
		i2p       = "i2p_enabled"
		proxyType = "proxy_type"
		proxyAuth = "proxy_auth_enabled"
		proxyBT   = "proxy_bittorrent"
		ipFilter  = "ip_filter_enabled"
	)
	anyProxy := notIn(proxyType, "None")
	fullProxy := notIn(proxyType, "None", "SOCKS4") // SOCKS4 lacks auth and hostnames

	return prefTab{title: "Connection", sections: []prefSection{
		{title: "Protocol", fields: []*prefField{
			enumField("bittorrent_protocol", "Peer connection protocol", "Peer connection protocol", btProtocolOptions()),
		}},
		{title: "Listening Port", hint: "Set the port to 0 to let the system pick an unused port.", fields: []*prefField{
			portField("listen_port", "Port used for incoming connections", "Listening port", true),
			checkField("upnp", "Use UPnP / NAT-PMP port forwarding from my router", "UPnP / NAT-PMP port forwarding"),
		}},
		{title: "Connections Limits", fields: []*prefField{
			unlimitedIntField("max_connec", "Global maximum number of connections", "Global maximum connections", 1),
			unlimitedIntField("max_connec_per_torrent", "Maximum number of connections per torrent", "Maximum connections per torrent", 1),
			unlimitedIntField("max_uploads", "Global maximum number of upload slots", "Global maximum upload slots", 1),
			unlimitedIntField("max_uploads_per_torrent", "Maximum number of upload slots per torrent", "Maximum upload slots per torrent", 1),
		}},
		{title: "I2P (Experimental)", fields: []*prefField{
			checkField(i2p, "Enable I2P", "I2P"),
			requiring(textField("i2p_address", "Host", "I2P host"), i2p),
			requiring(portField("i2p_port", "Port", "I2P port", false), i2p),
			requiring(checkField("i2p_mixed_mode", "Mixed mode", "I2P mixed mode"), i2p),
			requiring(checkField("i2p_pex_enabled", "Enable I2P Peer Exchange (requires restart)", "I2P Peer Exchange"), i2p),
		}},
		{title: "Proxy Server", fields: []*prefField{
			textEnumField(proxyType, "Type", "Proxy type", proxyTypeOptions()),
			requiringWhen(textField("proxy_ip", "Host", "Proxy host"), anyProxy),
			requiringWhen(portField("proxy_port", "Port", "Proxy port", false), anyProxy),
			requiringWhen(checkField("proxy_hostname_lookup", "Perform hostname lookup via proxy", "Proxy hostname lookup"), fullProxy),
			requiringWhen(checkField(proxyAuth, "Authentication", "Proxy authentication"), fullProxy),
			requiringWhen(textField("proxy_username", "Username", "Proxy username"), fullProxy, on(proxyAuth)),
			requiringWhen(passwordField("proxy_password", "Password", "Proxy password"), fullProxy, on(proxyAuth)),
			requiringWhen(checkField(proxyBT, "Use proxy for BitTorrent purposes", "Proxy for BitTorrent"), anyProxy),
			requiringWhen(checkField("proxy_peer_connections", "Use proxy for peer connections", "Proxy for peer connections"), anyProxy, on(proxyBT)),
			requiringWhen(checkField("proxy_rss", "Use proxy for RSS purposes", "Proxy for RSS"), fullProxy),
			requiringWhen(checkField("proxy_misc", "Use proxy for general purposes", "Proxy for general purposes"), fullProxy),
		}},
		{title: "IP Filtering", hint: "Banned IP addresses: one per line.", fields: []*prefField{
			checkField(ipFilter, "Use an IP filter file", "IP filter"),
			requiring(pathField("ip_filter_path", "Filter path (.dat, .p2p, .p2b)", "IP filter path", dirs), ipFilter),
			checkField("ip_filter_trackers", "Apply to trackers", "IP filter for trackers"),
			ipListField("banned_IPs", "Manually banned IP addresses", "Banned IP addresses"),
		}},
	}}
}

func logAgeUnitOptions() []prefOption[int64] {
	return []prefOption[int64]{{0, "days"}, {1, "months"}, {2, "years"}}
}

// behaviorPrefTab mirrors the server-side part of the WebUI Behavior tab;
// browser-only settings (color scheme, density, double-click actions...)
// are not in the API.
func behaviorPrefTab(dirs autocompleteFetcher) prefTab {
	const (
		searchJobs = "store_search_jobs"
		fileLog    = "file_log_enabled"
		logBackup  = "file_log_backup_enabled"
		logDelete  = "file_log_delete_old"
	)

	return prefTab{title: "Behavior", sections: []prefSection{
		{title: "Localization", hint: "Language code of the WebUI and the server's messages, e.g. en, ru or zh_CN.", fields: []*prefField{
			textField("locale", "User interface language", "Interface language"),
		}},
		{title: "Transfer List", hint: "Applies to WebUI clients.", fields: []*prefField{
			checkField("confirm_torrent_deletion", "Confirm when deleting torrents", "Confirm torrent deletion"),
		}},
		{title: "Status bar", hint: "Applies to WebUI clients.", fields: []*prefField{
			checkField("status_bar_external_ip", "Show external IP", "Show external IP in status bar"),
		}},
		{title: "Search", fields: []*prefField{
			checkField(searchJobs, "Save search tabs", "Save search tabs"),
			requiring(checkField("store_search_job_results", "Also save search results", "Save search results"), searchJobs),
		}},
		{title: "Log Files", fields: []*prefField{
			checkField(fileLog, "Write a log file", "Log file"),
			requiring(pathField("file_log_path", "Save path", "Log file folder", dirs), fileLog),
			requiring(checkField(logBackup, "Back up the log file when it grows", "Back up log file"), fileLog),
			requiring(intRangeField("file_log_max_size", "Backup after (KiB)", "Log backup size", "KiB", 1, 1024000), fileLog, logBackup),
			requiring(checkField(logDelete, "Delete old backup logs", "Delete old log backups"), fileLog),
			requiring(intRangeField("file_log_age", "Delete backups older than", "Log backup age", "", 1, 365), fileLog, logDelete),
			requiring(enumField("file_log_age_type", "Age unit", "Log backup age unit", logAgeUnitOptions()), fileLog, logDelete),
			checkField("performance_warning", "Log performance warnings", "Log performance warnings"),
		}},
	}}
}

// rssPrefTab mirrors the WebUI RSS tab; the auto-downloading rule editor is
// not part of the preferences.
func rssPrefTab() prefTab {
	return prefTab{title: "RSS", sections: []prefSection{
		{title: "RSS Reader", fields: []*prefField{
			checkField("rss_processing_enabled", "Enable fetching RSS feeds", "Fetch RSS feeds"),
			intField("rss_refresh_interval", "Feeds refresh interval (min)", "RSS refresh interval", "min", 1),
			intField("rss_fetch_delay", "Same host request delay (s)", "RSS same host delay", "s", 0),
			intField("rss_max_articles_per_feed", "Maximum number of articles per feed", "RSS articles per feed", "", 0),
		}},
		{title: "RSS Torrent Auto Downloader", fields: []*prefField{
			checkField("rss_auto_downloading_enabled", "Enable auto downloading of RSS torrents", "RSS auto downloading"),
		}},
		{title: "RSS Smart Episode Filter", hint: "Filters: one regular expression per line.", fields: []*prefField{
			checkField("rss_download_repack_proper_episodes", "Download REPACK/PROPER episodes", "Download REPACK/PROPER episodes"),
			textAreaField("rss_smart_episode_filters", "Filters", "Smart episode filters"),
		}},
	}}
}
