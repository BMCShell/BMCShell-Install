package generator

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDownloadHTTPSFallsBackOnlyForBrokenCertificateFile(t *testing.T) {
	cfg, err := Parse(strings.NewReader(minimalConfig))
	if err != nil {
		t.Fatal(err)
	}
	generated, err := Render(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, definition, found := strings.Cut(string(generated), "\ndownload_https() {\n")
	if !found {
		t.Fatal("missing download_https definition")
	}
	definition, _, found = strings.Cut(definition, "\nFETCH_COUNT=0\n")
	if !found {
		t.Fatal("missing end of download_https definition")
	}
	bash := findBash(t)
	for _, tc := range []struct {
		name       string
		firstExit  int
		windows    bool
		wantExit   int
		wantCalls  string
		wantResult string
	}{
		{"PATH curl succeeds", 0, true, 0, "path\n", "path response"},
		{"broken CA file uses Windows trust", 77, true, 0, "path\nwindows\n", "windows response"},
		{"HTTP failure skips Windows curl", 22, true, 22, "path\n", ""},
		{"missing Windows curl preserves CA error", 77, false, 77, "path\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			writeCommand := func(path, text string) {
				t.Helper()
				path = filepath.Join(directory, path)
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("#!/bin/bash\n"+text), 0755); err != nil {
					t.Fatal(err)
				}
			}
			writeCommand("bin/curl", `printf 'path\n' >> "$PWD/calls"
if [[ $BMCI_TEST_CURL_EXIT != 0 ]]; then exit "$BMCI_TEST_CURL_EXIT"; fi
while (( $# )); do
    if [[ $1 == --output ]]; then
        printf 'path response' > "$2"
        exit $?
    fi
    shift
done
exit 99
`)
			writeCommand("bin/cygpath", `[[ $# == 2 && $1 == -u && $2 == -S ]] || exit 99
printf '%s\n' "$PWD/Windows System32"
`)
			if tc.windows {
				writeCommand("Windows System32/curl.exe", `printf 'windows\n' >> "$PWD/calls"
[[ $1 == --disable && ! ${CURL_CA_BUNDLE+x} && ! ${SSL_CERT_FILE+x} && ! ${SSL_CERT_DIR+x} ]] || exit 99
printf '%s\n' "$@" > "$PWD/windows-args"
printf 'windows response'
`)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, bash, "--noprofile", "--norc", "-s", "--", strconv.Itoa(tc.firstExit))
			command.Dir = directory
			command.Stdin = strings.NewReader("download_https() {\n" + definition + `
export PATH="$PWD/bin:$PATH" BMCI_TEST_CURL_EXIT=$1
export CURL_CA_BUNDLE=broken SSL_CERT_FILE=broken SSL_CERT_DIR=broken
status=0
download_https 'https://example.invalid/asset.zip' "$PWD/asset with spaces.zip" || status=$?
[[ $CURL_CA_BUNDLE == broken && $SSL_CERT_FILE == broken && $SSL_CERT_DIR == broken ]] || exit 99
exit "$status"
`)
			output, err := command.CombinedOutput()
			if tc.wantExit == 0 {
				if err != nil {
					t.Fatalf("download failed: %v\n%s", err, output)
				}
			} else {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != tc.wantExit {
					t.Fatalf("expected exit %d, got %v\n%s", tc.wantExit, err, output)
				}
			}
			calls, err := os.ReadFile(filepath.Join(directory, "calls"))
			if err != nil || string(calls) != tc.wantCalls {
				t.Fatalf("expected curl calls %q, got %q (%v)", tc.wantCalls, calls, err)
			}
			if tc.wantResult != "" {
				result, err := os.ReadFile(filepath.Join(directory, "asset with spaces.zip"))
				if err != nil || string(result) != tc.wantResult {
					t.Fatalf("expected download %q, got %q (%v)", tc.wantResult, result, err)
				}
			}
			if strings.Contains(tc.wantCalls, "windows") {
				args, err := os.ReadFile(filepath.Join(directory, "windows-args"))
				if err != nil {
					t.Fatal(err)
				}
				flags := "\n" + string(args)
				if !strings.Contains(flags, "\n--proto\n=https\n") || !strings.Contains(flags, "\n--proto-redir\n=https\n") ||
					strings.Contains(flags, "\n--insecure\n") || strings.Contains(flags, "\n-k\n") {
					t.Fatalf("Windows curl must keep HTTPS and certificate verification: %s", args)
				}
			}
		})
	}
}
