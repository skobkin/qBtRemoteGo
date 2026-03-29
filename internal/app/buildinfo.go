package app

import (
	"fmt"
	"strings"
	"time"
)

const DevBuildVersion = "dev"

var (
	// Version is filled by ldflags in release builds.
	Version = DevBuildVersion
	// BuildDate is filled by ldflags in release builds.
	BuildDate = ""
)

func BuildVersion() string {
	version := strings.TrimSpace(Version)
	if version == "" {
		return DevBuildVersion
	}

	return version
}

func BuildDateYMD() string {
	raw := strings.TrimSpace(BuildDate)
	if raw == "" {
		return ""
	}

	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.Format("2006-01-02")
	}

	if len(raw) >= len("2006-01-02") {
		date := raw[:len("2006-01-02")]
		if _, err := time.Parse("2006-01-02", date); err == nil {
			return date
		}
	}

	return raw
}

func BuildVersionWithDate() string {
	version := BuildVersion()
	if buildDate := BuildDateYMD(); buildDate != "" {
		return fmt.Sprintf("%s (%s)", version, buildDate)
	}

	return version
}
