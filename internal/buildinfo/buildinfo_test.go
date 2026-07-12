package buildinfo

import "testing"

func TestString(t *testing.T) {
	originalVersion, originalCommit, originalDate := Version, Commit, BuildDate
	t.Cleanup(func() { Version, Commit, BuildDate = originalVersion, originalCommit, originalDate })

	Version, Commit, BuildDate = "dev", "unknown", "unknown"
	if got, want := String(), "cxre dev"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}

	Version, Commit = "0.1.0", "abc1234"
	if got, want := String(), "cxre 0.1.0 (abc1234)"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}
