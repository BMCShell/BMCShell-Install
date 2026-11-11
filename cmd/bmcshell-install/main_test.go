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
	var stdout bytes.Buffer
	call := func(args ...string) error { stdout.Reset(); return run(args, &stdout) }
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
	var stdout bytes.Buffer
	if err := run([]string{"generate", "--config", config, "--output", output, "--force"}, &stdout); err == nil {
		t.Fatal("accepted invalid configuration")
	}
	after, err := os.ReadFile(output)
	if err != nil || string(after) != "preserve me" {
		t.Fatalf("existing output changed: %q, %v", after, err)
	}
}

func TestCLIErrorsAndHelp(t *testing.T) {
	var stdout bytes.Buffer
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"unknown"}, "未知命令"},
		{[]string{"validate"}, "缺少必填选项 --config"},
		{[]string{"generate"}, "缺少必填选项 --config"},
		{[]string{"init", "unexpected"}, "不接受位置参数"},
		{[]string{"version", "unexpected"}, "不接受额外参数"},
		{[]string{"validate", "--config"}, "选项 --config 缺少参数值"},
		{[]string{"init", "--other"}, "不支持选项 --other"},
		{[]string{"init", "--force=maybe"}, "--force 的值无效"},
	} {
		if err := run(tc.args, &stdout); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("arguments %v: expected error containing %q, got %v", tc.args, tc.want, err)
		}
	}
	for _, args := range [][]string{nil, {"--help"}, {"init", "-h"}, {"validate", "--help"}, {"generate", "--help"}, {"version", "--help"}} {
		stdout.Reset()
		if err := run(args, &stdout); err != nil {
			t.Fatalf("help failed: %v: %v", args, err)
		}
		if !strings.Contains(stdout.String(), "用法\n") || strings.Contains(stdout.String(), "Usage") {
			t.Fatalf("expected Chinese help for %v, got %q", args, stdout.String())
		}
	}
	stdout.Reset()
	if err := run([]string{"version"}, &stdout); err != nil || stdout.String() != "BMCShell-Install "+version+"\n" {
		t.Fatalf("unexpected version output %q: %v", stdout.String(), err)
	}
}
