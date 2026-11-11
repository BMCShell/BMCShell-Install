package generator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestFrameworkReleaseIndexConfiguration(t *testing.T) {
	const indexHostPath = "github.com/BMCShell/BMCShell/releases/download/release-index/index.json"
	const indexURL = "https://" + indexHostPath
	for _, tc := range []struct {
		name    string
		address string
		fields  string
		version string
		invalid bool
	}{
		{name: "latest from the index"},
		{name: "explicit version in the index", fields: `,"version":"3.0.0-dev.8"`, version: "3.0.0-dev.8"},
		{name: "archive conflicts", fields: `,"archive":"payload/framework.zip"`, invalid: true},
		{name: "checksum conflicts", fields: `,"sha256":"` + strings.Repeat("a", 64) + `"`, invalid: true},
		{name: "invalid version pin", fields: `,"version":"latest"`, invalid: true},
		{name: "HTTP index", address: "http://" + indexHostPath, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			address := tc.address
			if address == "" {
				address = indexURL
			}
			config := fmt.Sprintf(`{"schema":1,"framework":{"release_index":%q%s}}`, address, tc.fields)
			cfg, err := Parse(strings.NewReader(config))
			if tc.invalid {
				if err == nil {
					t.Fatal("accepted an invalid release index configuration")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Framework.ReleaseIndex != indexURL || cfg.Framework.Version != tc.version {
				t.Fatalf("release index selection changed: %#v", cfg.Framework)
			}
		})
	}
}

func TestWindowsReleaseIndexParser(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell unavailable")
	}
	powershell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skip("Windows PowerShell unavailable")
	}

	const latest, previous = "3.0.0-dev.9", "3.0.0-dev.8"
	const base = "https://github.com/BMCShell/BMCShell/releases/download/"
	url := func(version string) string {
		return base + "v" + version + "/BMCShell-" + version + "-windows-amd64.zip"
	}
	release := func(version, address, digest string) string {
		return fmt.Sprintf(`{"version":%q,"tag":%q,"artifacts":[{"platform":"windows-amd64","url":%q,"sha256":%q}]}`,
			version, "v"+version, address, digest)
	}
	index := func(schema string, releases ...string) string {
		return fmt.Sprintf(`{"schema":%s,"product":"BMCShell","latest":%q,"releases":[%s]}`, schema, latest, strings.Join(releases, ","))
	}
	latestHash, previousHash := strings.Repeat("A", 64), strings.Repeat("b", 64)
	current := release(latest, url(latest), latestHash)
	older := release(previous, url(previous), previousHash)
	for _, tc := range []struct {
		name, json, pin, want string
	}{
		{"latest pointer", index("1", older, current), "", latest + "\n" + url(latest) + "\n" + strings.ToLower(latestHash)},
		{"pinned older version", index("1", current, older), previous, previous + "\n" + url(previous) + "\n" + previousHash},
		{"unsupported schema", index("2", current), "", ""},
		{"string schema", index(`"1"`, current), "", ""},
		{"ambiguous release", index("1", current, current), "", ""},
		{"unknown pin", index("1", current), previous, ""},
		{"off-repository URL", index("1", release(latest, strings.Replace(url(latest), "github.com/", "github.com@evil.invalid/", 1), latestHash)), "", ""},
		{"mismatched artifact version", index("1", release(latest, url(previous), latestHash)), "", ""},
		{"invalid checksum", index("1", release(latest, url(latest), latestHash[:63]+"z")), "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "release index.json")
			if err := os.WriteFile(file, []byte(tc.json), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, powershell, "-NoProfile", "-NonInteractive", "-Command", releaseIndexParser)
			cmd.Env = append(os.Environ(), "BMCI_INDEX_PATH="+file, "BMCI_INDEX_VERSION="+tc.pin)
			output, err := cmd.CombinedOutput()
			if tc.want == "" {
				if err == nil || !strings.Contains(string(output), "BMCShell 版本索引：") {
					t.Fatalf("expected parser to reject the index, got %v with output %q", err, output)
				}
				return
			}
			got := strings.TrimRight(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n")
			if err != nil || got != tc.want {
				t.Fatalf("expected %q, got %q (%v)", tc.want, got, err)
			}
		})
	}
}
