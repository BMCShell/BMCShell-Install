package generator

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestInstallerHelpFromStandardInput(t *testing.T) {
	cfg, err := Parse(strings.NewReader(minimalConfig))
	if err != nil {
		t.Fatal(err)
	}
	script, err := Render(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, findBash(t), "--noprofile", "--norc", "-s", "--", "--help")
	command.Stdin = strings.NewReader(string(script))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("stdin installer help failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "用法") || strings.Contains(string(output), "[1/5]") {
		t.Fatalf("stdin help should display usage without starting installation: %s", output)
	}
}

func TestTruncatedInstallerStreamDoesNotStart(t *testing.T) {
	cfg, err := Parse(strings.NewReader(minimalConfig))
	if err != nil {
		t.Fatal(err)
	}
	script, err := Render(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a transfer ending before the top-level compound block closes.
	// Even --help must wait for the complete block, so a partial download never
	// starts installation commands or accepts any answers from the script stream.
	trimmed := strings.TrimSpace(string(script))
	if !strings.HasSuffix(trimmed, "}") {
		t.Fatal("installer must end with its top-level compound block")
	}
	truncated := strings.TrimSuffix(trimmed, "}")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, findBash(t), "--noprofile", "--norc", "-s", "--", "--help")
	command.Stdin = strings.NewReader(truncated)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("accepted an incomplete installer stream")
	}
	if strings.Contains(string(output), "用法") || strings.Contains(string(output), "[1/5]") {
		t.Fatalf("incomplete stream executed installer commands: %s", output)
	}
}
