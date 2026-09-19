package triageevidence

import "testing"

func TestParseReportedVersionBuckets(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		channel ReleaseChannel
		major   int
		minor   int
		patch   int
		pre     string
	}{
		{name: "bare stable", raw: "v3.4.0", channel: ChannelStable, major: 3, minor: 4, patch: 0},
		{name: "product prefix", raw: "gentle-ai 3.4.0 (gga v2.10.1)", channel: ChannelStable, major: 3, minor: 4, patch: 0},
		{name: "first triplet wins", raw: "gga v2.10.1 comes before gentle-ai 3.4.0", channel: ChannelStable, major: 2, minor: 10, patch: 1},
		{name: "version word", raw: "Version 2.4.0", channel: ChannelStable, major: 2, minor: 4, patch: 0},
		{name: "source prose", raw: "Public release source **v3.4.0**, commit `82a6de96`", channel: ChannelStable, major: 3, minor: 4, patch: 0},
		{name: "prerelease rc", raw: "v2.2.0-rc.3", channel: ChannelPrerelease, major: 2, minor: 2, patch: 0, pre: "rc.3"},
		{name: "prerelease beta", raw: "3.0.0-beta.2", channel: ChannelPrerelease, major: 3, minor: 0, patch: 0, pre: "beta.2"},
		{name: "release word", raw: "release 1.2.3", channel: ChannelStable, major: 1, minor: 2, patch: 3},
		{name: "main build", raw: "build from main at the time of this report", channel: ChannelMain},
		{name: "bare main", raw: "main", channel: ChannelMain},
		{name: "unreleased main", raw: "unreleased main", channel: ChannelMain},
		{name: "not sure", raw: "not sure", channel: ChannelUnknown},
		{name: "latest", raw: "latest", channel: ChannelUnknown},
		{name: "empty", raw: "  ", channel: ChannelUnknown},
		{name: "partial no patch", raw: "3", channel: ChannelUnknown},
		{name: "overflow digit run never panics", raw: "99999999999999999999.0.0", channel: ChannelUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseReportedVersion(tt.raw)
			if got.Channel != tt.channel {
				t.Fatalf("ParseReportedVersion(%q).Channel = %v, want %v", tt.raw, got.Channel, tt.channel)
			}
			if got.Major != tt.major || got.Minor != tt.minor || got.Patch != tt.patch {
				t.Errorf("ParseReportedVersion(%q) = %d.%d.%d, want %d.%d.%d", tt.raw, got.Major, got.Minor, got.Patch, tt.major, tt.minor, tt.patch)
			}
			if got.Pre != tt.pre {
				t.Errorf("ParseReportedVersion(%q).Pre = %q, want %q", tt.raw, got.Pre, tt.pre)
			}
		})
	}
}

func TestReportedVersionCompare(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{name: "equal stable", a: "v3.4.0", b: "3.4.0", want: 0},
		{name: "newer minor", a: "3.5.0", b: "3.4.9", want: 1},
		{name: "older patch", a: "3.4.0", b: "3.4.1", want: -1},
		{name: "prerelease older than its stable", a: "3.4.0", b: "3.4.0-rc.1", want: 1},
		{name: "same prerelease", a: "2.2.0-rc.3", b: "2.2.0-rc.3", want: 0},
		{name: "main never older than exact", a: "main", b: "1.0.0", want: -1},
		{name: "unknown ties main", a: "not sure", b: "main", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseReportedVersion(tt.a).Compare(ParseReportedVersion(tt.b))
			if got != tt.want {
				t.Errorf("Compare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestReleaseChannelString(t *testing.T) {
	if got := ChannelStable.String(); got != "stable" {
		t.Errorf("ChannelStable.String() = %q, want stable", got)
	}
	if got := ReleaseChannel(99).String(); got != "unknown" {
		t.Errorf("unknown channel String() = %q, want unknown", got)
	}
}
