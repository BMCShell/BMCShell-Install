package generator

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnsureCoreExecutableOnlyRepairsSelectedRelease(t *testing.T) {
	const selectedVersion = "3.0.0-dev.9"
	const executable = "bin/releases/" + selectedVersion + "/BMCShell.exe"
	const executableContents = "#!/bin/bash\nexit 0\n"
	const settingsContents = "user settings fixture\n"
	cfg, err := Parse(strings.NewReader(minimalConfig))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Framework.Version = selectedVersion
	generated, err := Render(cfg)
	if err != nil {
		t.Fatal(err)
	}
	const begin = "\nensure_core_executable() {\n"
	_, definition, found := strings.Cut(string(generated), begin)
	if !found {
		t.Fatal("generated installer is missing its executable check")
	}
	definition, _, found = strings.Cut(definition, "\nprepare_core() {\n")
	if !found {
		t.Fatal("generated installer is missing prepare_core after its executable check")
	}
	bash := findBash(t)
	for _, tc := range []struct {
		name        string
		version     string
		file        string
		wantSuccess bool
		wantChmod   bool
	}{
		{"same version already executable", selectedVersion, "executable", true, false},
		{"restore missing owner execute bit", selectedVersion, "no-execute", true, true},
		{"missing executable", selectedVersion, "missing", false, false},
		{"different active version", "3.0.0-dev.8", "executable", false, false},
		{"symlink executable", selectedVersion, "symlink", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			root := filepath.Join(directory, "installation root")
			write := func(path, contents string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
					t.Fatal(err)
				}
			}
			active := filepath.Join(root, "bin/active")
			settings := filepath.Join(root, "settings.json")
			core := filepath.Join(root, executable)
			write(active, tc.version+"\n")
			write(settings, settingsContents)
			if err := os.MkdirAll(filepath.Dir(core), 0755); err != nil {
				t.Fatal(err)
			}
			switch tc.file {
			case "executable", "no-execute":
				write(core, executableContents)
			case "symlink":
				outside := filepath.Join(directory, "outside.exe")
				write(outside, executableContents)
				if err := os.Symlink(outside, core); err != nil {
					t.Skipf("native symlink unavailable: %v", err)
				}
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, bash, "--noprofile", "--norc", "-s", "--", tc.file)
			command.Dir = directory
			command.Stdin = strings.NewReader(begin + definition + `
CORE_VERSION=` + selectedVersion + `
test_root="$PWD/installation root"
test_executable="$test_root/` + executable + `"
case $1 in
    executable) command chmod u+x -- "$test_executable" || exit 99 ;;
    no-execute)
        command chmod u-x -- "$test_executable" || exit 99
        if [[ -x $test_executable ]]; then exit 125; fi ;;
esac
chmod() {
    printf '%s\n' "${!#}" >> "$PWD/chmod-targets"
    command chmod "$@"
}
if ensure_core_executable "$test_root"; then
    [[ -x $test_executable ]] || exit 99
    exit 0
fi
exit 1
`)
			output, err := command.CombinedOutput()
			if tc.wantSuccess {
				if err != nil {
					var exitErr *exec.ExitError
					if tc.file == "no-execute" && errors.As(err, &exitErr) && exitErr.ExitCode() == 125 {
						t.Skip("the test Bash mount treats .exe files as executable regardless of the owner bit")
					}
					t.Fatalf("executable check failed: %v\n%s", err, output)
				}
			} else {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
					t.Fatalf("expected executable check to reject the installation: %v\n%s", err, output)
				}
			}
			chmodTargets, err := os.ReadFile(filepath.Join(directory, "chmod-targets"))
			if tc.wantChmod {
				if err != nil || strings.Count(string(chmodTargets), "\n") != 1 ||
					!strings.HasSuffix(string(chmodTargets), "/installation root/"+executable+"\n") {
					t.Fatalf("expected chmod on only the selected executable, got %q (%v)", chmodTargets, err)
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unexpected permission change: %q (%v)", chmodTargets, err)
			}
			for path, expected := range map[string]string{active: tc.version + "\n", settings: settingsContents} {
				contents, err := os.ReadFile(path)
				if err != nil || string(contents) != expected {
					t.Fatalf("unrelated file %q was changed: %v", path, err)
				}
			}
		})
	}
}
