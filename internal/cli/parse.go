package cli

import (
	"fmt"
	"io"
)

type options struct {
	command string
	json    bool
	utc     bool
	help    bool
	version bool
}

func parse(args []string) (options, error) {
	var parsed options
	// Preserve JSON mode even when another argument is invalid so scripts never
	// receive a human usage block after explicitly requesting JSON.
	for _, arg := range args {
		if arg == "--json" {
			parsed.json = true
			break
		}
	}
	for _, arg := range args {
		switch arg {
		case "--json":
			parsed.json = true
		case "--utc":
			parsed.utc = true
		case "--version":
			parsed.version = true
		case "--help", "-h":
			parsed.help = true
		case "--":
			return parsed, fmt.Errorf("unexpected argument %q", arg)
		default:
			if len(arg) > 0 && arg[0] == '-' {
				return parsed, fmt.Errorf("unknown flag %q", arg)
			}
			if parsed.command != "" {
				return parsed, fmt.Errorf("unexpected argument %q", arg)
			}
			parsed.command = arg
		}
	}
	return parsed, nil
}

func writeHelp(out io.Writer) error {
	_, err := fmt.Fprint(out, `CXRE — Codex Resets

Know when your Codex limits reset and reset credits expire.

Usage:
  cxre [options]

Options:
  --json       Output stable machine-readable JSON
  --utc        Display timestamps in UTC
  --version    Display the CXRE version
  --help, -h   Show this help

CXRE requires Codex CLI 0.143.0 or newer and a ChatGPT sign-in.
It is an unofficial community tool and is not affiliated with OpenAI.
`)
	return err
}
