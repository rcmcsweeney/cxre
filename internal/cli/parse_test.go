package cli

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    options
		wantErr string
	}{
		{name: "empty"},
		{name: "combined", args: []string{"--json", "--utc"}, want: options{json: true, utc: true}},
		{name: "help", args: []string{"-h"}, want: options{help: true}},
		{name: "unknown flag", args: []string{"--wat"}, wantErr: `unknown flag "--wat"`},
		{name: "future command", args: []string{"status"}, want: options{command: "status"}},
		{name: "extra argument", args: []string{"status", "extra"}, wantErr: `unexpected argument "extra"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parse(test.args)
			if test.wantErr != "" {
				if err == nil || err.Error() != test.wantErr {
					t.Fatalf("parse() error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("parse() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestWriteHelp(t *testing.T) {
	var output strings.Builder
	if err := writeHelp(&output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"CXRE — Codex Resets", "Usage:", "--json", "Codex CLI 0.143.0", "not affiliated with OpenAI"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("help missing %q", expected)
		}
	}
}
