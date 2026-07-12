package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestString(t *testing.T) {
	originalVersion, originalCommit, originalDate := Version, Commit, BuildDate
	t.Cleanup(func() { Version, Commit, BuildDate = originalVersion, originalCommit, originalDate })

	Version, Commit, BuildDate = "0.1.0", "abc1234", "release-date"
	if got, want := String(), "cxre 0.1.0 (abc1234)"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestResolveBuildMetadata(t *testing.T) {
	tests := []struct {
		name     string
		linked   Info
		embedded *debug.BuildInfo
		want     Info
	}{
		{
			name:   "release linker metadata wins",
			linked: Info{Version: "0.1.1", Commit: "linked-commit", Date: "linked-date"},
			embedded: &debug.BuildInfo{
				Main: debug.Module{Version: "v9.9.9"},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "embedded-commit"},
					{Key: "vcs.time", Value: "embedded-date"},
				},
			},
			want: Info{Version: "0.1.1", Commit: "linked-commit", Date: "linked-date"},
		},
		{
			name:   "go install module version fallback",
			linked: Info{Version: "dev", Commit: "unknown", Date: "unknown"},
			embedded: &debug.BuildInfo{
				Main: debug.Module{Version: "v0.1.1"},
			},
			want: Info{Version: "0.1.1", Commit: "unknown", Date: "unknown"},
		},
		{
			name:   "vcs settings fill missing linker metadata",
			linked: Info{Version: " dev ", Commit: "", Date: ""},
			embedded: &debug.BuildInfo{
				Main: debug.Module{Version: "(devel)"},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: " embedded-commit "},
					{Key: "vcs.time", Value: " embedded-date "},
				},
			},
			want: Info{Version: "dev", Commit: "embedded-commit", Date: "embedded-date"},
		},
		{
			name:     "missing metadata stays development build",
			linked:   Info{},
			embedded: nil,
			want:     Info{Version: "dev", Commit: "unknown", Date: "unknown"},
		},
		{
			name:   "empty embedded values are ignored",
			linked: Info{Version: "dev", Commit: "unknown", Date: "unknown"},
			embedded: &debug.BuildInfo{
				Main: debug.Module{},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "  "},
					{Key: "vcs.time", Value: "  "},
				},
			},
			want: Info{Version: "dev", Commit: "unknown", Date: "unknown"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolve(test.linked, test.embedded); got != test.want {
				t.Fatalf("resolve() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	tests := []struct {
		info Info
		want string
	}{
		{info: Info{Version: "dev", Commit: "embedded-commit"}, want: "cxre dev"},
		{info: Info{Version: "0.1.1", Commit: "unknown"}, want: "cxre 0.1.1"},
		{info: Info{Version: "0.1.1", Commit: "abc1234"}, want: "cxre 0.1.1 (abc1234)"},
	}

	for _, test := range tests {
		if got := format(test.info); got != test.want {
			t.Errorf("format(%#v) = %q, want %q", test.info, got, test.want)
		}
	}
}
