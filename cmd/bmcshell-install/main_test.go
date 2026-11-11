package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIConfigurationToInstaller(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "install.json")
	output := filepath.Join(dir, "generated", "install.sh")
	var stdout, stderr bytes.Buffer
	call := func(args ...string) error { stdout.Reset(); stderr.Reset(); return run(args, &stdout, &stderr) }
	if err := call("init", "--output", config); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := call("validate", "--config", config); err != nil {
		t.Fatal(err)
	}
	if err := call("generate", "--config", config, "--output", output); err != nil {
		t.Fatal(err)
	}
	generated, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(generated), "#!/usr/bin/env bash") {
		t.Fatal("output is not a Bash script")
	}
	if err := call("generate", "--config", config, "--output", output); err == nil {
		t.Fatal("overwrote output without --force")
	}
	if err := call("generate", "--config", config, "--output", output, "--force"); err != nil {
		t.Fatal(err)
	}
	if err := call("generate", "--config", config, "--output", config, "--force"); err == nil {
		t.Fatal("overwrote input configuration")
	}
	after, err := os.ReadFile(config)
	if err != nil || !bytes.Equal(after, original) {
		t.Fatal("input config changed")
	}
	if err := call("init", "--output", config); err == nil {
		t.Fatal("init overwrote existing file")
	}
}

func TestCLIFailurePreservesExistingOutput(t *testing.T) {
	dir := t.TempDir()
	config, output := filepath.Join(dir, "bad.json"), filepath.Join(dir, "existing.sh")
	if err := os.WriteFile(config, []byte(`{"schema":1,"framework":{"archive":"x.zip","version":"bad"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, []byte("preserve me"), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"generate", "--config", config, "--output", output, "--force"}, &stdout, &stderr); err == nil {
		t.Fatal("accepted invalid configuration")
	}
	after, err := os.ReadFile(output)
	if err != nil || string(after) != "preserve me" {
		t.Fatalf("existing output changed: %q, %v", after, err)
	}
}

func TestCLIErrorsAndHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	for _, args := range [][]string{{"unknown"}, {"validate"}, {"generate"}, {"init", "unexpected"}, {"version", "unexpected"}} {
		if err := run(args, &stdout, &stderr); err == nil {
			t.Fatalf("accepted arguments: %v", args)
		}
	}
	for _, args := range [][]string{nil, {"--help"}, {"version"}, {"generate", "--help"}} {
		stdout.Reset()
		stderr.Reset()
		if err := run(args, &stdout, &stderr); err != nil {
			t.Fatalf("help/version failed: %v: %v", args, err)
		}
		if stdout.Len()+stderr.Len() == 0 {
			t.Fatalf("help/version produced no output: %v", args)
		}
	}
}
