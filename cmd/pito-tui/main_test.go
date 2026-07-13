package main

import (
	"flag"
	"testing"
)

// newTestFlagSet mirrors main()'s flag.CommandLine, but ContinueOnError so
// a bad flag returns an error here instead of os.Exit()ing the test binary.
func newTestFlagSet() *flag.FlagSet {
	return flag.NewFlagSet("pito-tui", flag.ContinueOnError)
}

func TestParseFlagsDefaults(t *testing.T) {
	f, err := parseFlags(newTestFlagSet(), nil)
	if err != nil {
		t.Fatalf("parseFlags() error = %v", err)
	}
	if f.resume != "" || f.instance != "" || f.tour || f.arg(0) != "" {
		t.Errorf("parseFlags(nil) = %+v, want all zero", f)
	}
}

func TestParseFlagsResumeByUUID(t *testing.T) {
	f, err := parseFlags(newTestFlagSet(), []string{"--resume", "50189b77-1234-4abc-8def-0123456789ab"})
	if err != nil {
		t.Fatalf("parseFlags() error = %v", err)
	}
	if f.resume != "50189b77-1234-4abc-8def-0123456789ab" {
		t.Errorf("resume = %q", f.resume)
	}
}

func TestParseFlagsResumeByName(t *testing.T) {
	f, err := parseFlags(newTestFlagSet(), []string{"--resume", "Library Sync"})
	if err != nil {
		t.Fatalf("parseFlags() error = %v", err)
	}
	if f.resume != "Library Sync" {
		t.Errorf("resume = %q", f.resume)
	}
}

func TestParseFlagsResumeWithInstance(t *testing.T) {
	f, err := parseFlags(newTestFlagSet(), []string{"--instance", "https://dev.pitomd.com", "--resume", "Library Sync"})
	if err != nil {
		t.Fatalf("parseFlags() error = %v", err)
	}
	if f.instance != "https://dev.pitomd.com" || f.resume != "Library Sync" {
		t.Errorf("f = %+v", f)
	}
}

func TestParseFlagsPositionalConversationUUIDUnaffectedByResume(t *testing.T) {
	// The pre-existing positional [conversation-uuid] argument and the new
	// --resume flag are independent surfaces (run.go picks between them);
	// parsing must keep both readable off the same cliFlags.
	f, err := parseFlags(newTestFlagSet(), []string{"some-uuid"})
	if err != nil {
		t.Fatalf("parseFlags() error = %v", err)
	}
	if f.arg(0) != "some-uuid" || f.resume != "" {
		t.Errorf("arg(0) = %q, resume = %q", f.arg(0), f.resume)
	}
}

func TestParseFlagsConfigSubcommand(t *testing.T) {
	f, err := parseFlags(newTestFlagSet(), []string{"config", "server=https://pito.example.com"})
	if err != nil {
		t.Fatalf("parseFlags() error = %v", err)
	}
	if f.arg(0) != "config" || len(f.args) != 2 || f.args[1] != "server=https://pito.example.com" {
		t.Errorf("args = %v", f.args)
	}
}

func TestParseFlagsRejectsUnknownFlag(t *testing.T) {
	if _, err := parseFlags(newTestFlagSet(), []string{"--not-a-flag"}); err == nil {
		t.Error("parseFlags() with an unknown flag: want an error, got nil")
	}
}
