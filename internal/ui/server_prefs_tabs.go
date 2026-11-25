package ui

// Tab descriptors of the Server settings window. They mirror the qBittorrent
// WebUI Options tabs; the key-to-field map is in docs/qbittorrent/ and on
// issue #32.

// serverPrefTabs lists the implemented tabs in WebUI order. dirs suggests
// directories on the server for path fields (nil disables suggestions).
func serverPrefTabs(dirs autocompleteFetcher) []prefTab {
	return []prefTab{downloadsPrefTab(dirs), speedPrefTab(), bitTorrentPrefTab()}
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
