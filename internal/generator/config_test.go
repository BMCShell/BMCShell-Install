package generator

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const minimalConfig = `{"schema":1,"framework":{"archive":"payload/framework.zip","version":"3.0.0-dev.8"}}`

func TestParseDefaultsAndExplicitChoices(t *testing.T) {
	cfg, err := Parse(strings.NewReader("\ufeff" + minimalConfig))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name == "" || cfg.Framework.InstallDir != "$HOME/.local/share/BMCShell" || !Enabled(cfg.Framework.Autoload) || len(cfg.Globals.Scan.Commands) == 0 {
		t.Fatalf("missing defaults: %#v", cfg)
	}
	explicit := strings.Replace(minimalConfig, `"schema":1`, `"schema":1,"globals":{"scan":{"enabled":false,"commands":[]}}`, 1)
	cfg, err = Parse(strings.NewReader(explicit))
	if err != nil {
		t.Fatal(err)
	}
	if Enabled(cfg.Globals.Scan.Enabled) || cfg.Globals.Scan.Commands == nil || len(cfg.Globals.Scan.Commands) != 0 {
		t.Fatalf("explicit empty scan list or disabled scan lost: %#v", cfg.Globals.Scan)
	}
}

func TestParseRejectsInvalidConfiguration(t *testing.T) {
	with := func(fragment string) string {
		return strings.TrimSuffix(minimalConfig, "}") + "," + fragment + "}"
	}
	tests := map[string]string{
		"unknown top field":          with(`"framwork":{}`),
		"unknown nested field":       strings.Replace(minimalConfig, `"version":`, `"typo":true,"version":`, 1),
		"duplicate top key":          with(`"schema":1`),
		"duplicate nested key":       strings.Replace(minimalConfig, `"version":`, `"version":"3.0.0","version":`, 1),
		"duplicate map key":          with(`"globals":{"env":{"CUSTOM":"one","CUSTOM":"two"}}`),
		"null environment":           with(`"globals":{"env":{"CUSTOM":null}}`),
		"null optional object":       with(`"doctor":null`),
		"trailing JSON":              minimalConfig + `{}`,
		"malformed JSON":             minimalConfig[:len(minimalConfig)-2],
		"wrong schema":               strings.Replace(minimalConfig, `"schema":1`, `"schema":2`, 1),
		"bad version":                strings.Replace(minimalConfig, `3.0.0-dev.8`, `latest`, 1),
		"remote without hash":        strings.Replace(minimalConfig, `payload/framework.zip`, `https://example.invalid/framework.zip`, 1),
		"HTTP remote":                strings.Replace(minimalConfig, `payload/framework.zip`, `http://example.invalid/framework.zip`, 1),
		"bad local hash":             strings.Replace(minimalConfig, `"version":`, `"sha256":"oops","version":`, 1),
		"reserved environment":       with(`"globals":{"env":{"BASH_ENV":"evil.sh"}}`),
		"installer cleanup variable": with(`"globals":{"env":{"WORK_DIR":"/user/data"}}`),
		"installer input variable":   with(`"globals":{"env":{"REPLY":"y"}}`),
		"installer scan variable":    with(`"globals":{"env":{"SCAN_BASE_PATH":"/unexpected"}}`),
		"PATH environment":           with(`"globals":{"env":{"PATH":"/x"}}`),
		"bad environment name":       with(`"globals":{"env":{"X;touch x":"literal"}}`),
		"control character":          with(`"name":"bad\nname"`),
		"command path":               with(`"globals":{"scan":{"commands":["/bin/curl"]}}`),
		"empty directory":            with(`"globals":{"scan":{"directories":[""]}}`),
		"package missing source":     with(`"packages":[{"name":"x"}]`),
		"package dual source":        with(`"packages":[{"file":"x.zip","selector":"x"}]`),
		"option selector":            with(`"packages":[{"selector":"--file"}]`),
		"hash on selector":           with(`"packages":[{"selector":"x","sha256":"` + strings.Repeat("a", 64) + `"}]`),
		"required package skipped":   with(`"packages":[{"selector":"x","required":true,"default":false}]`),
		"required script skipped":    with(`"scripts":[{"file":"x.sh","required":true,"default":false}]`),
		"remote script missing hash": with(`"scripts":[{"file":"https://example.invalid/x.sh"}]`),
		"script control argument":    with(`"scripts":[{"file":"x.sh","args":["bad\rarg"]}]`),
		"too large":                  minimalConfig + strings.Repeat(" ", MaxConfigSize),
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(data)); err == nil {
				t.Fatalf("accepted invalid configuration: %s", data[:min(len(data), 200)])
			}
		})
	}
}

func TestRemoteSourcesAndInjectionStringsRemainData(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "injected")
	literal := `O'Brien $(touch '` + filepath.ToSlash(marker) + `') ` + "`touch should-not-run`" + `; $HOME *`
	cfg := Config{Schema: 1, Name: literal, Framework: Framework{Archive: "https://example.invalid/a.zip", SHA256: strings.Repeat("A", 64), Version: "3.0.0-dev.8"}, Globals: Globals{Env: map[string]string{"CUSTOM": literal}}}
	encoded, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Name != literal || parsed.Globals.Env["CUSTOM"] != literal {
		t.Fatal("configuration data was transformed")
	}
	bash := findBash(t)
	cmd := exec.Command(bash, "-c", "VALUE="+ShellQuote(literal)+"\nprintf '%s' \"$VALUE\"")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("shell quote: %v: %s", err, output)
	}
	if string(output) != literal {
		t.Fatalf("literal did not round trip: %q", output)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("shell payload executed: %v", err)
	}
}

func TestRenderStableBashAndConfiguredStepOrder(t *testing.T) {
	cfg, err := Parse(strings.NewReader(minimalConfig))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Globals.Env = map[string]string{"ZZ_CUSTOM": "last", "AA_CUSTOM": `a'b $HOME`}
	cfg.Globals.Settings = map[string]string{"BMCSHELL_TLS_MODE": "verify"}
	cfg.Globals.Scan.Required = []string{"curl", "ssh"}
	cfg.Doctor.RequiredCommands = []string{"ssh", "unzip"}
	cfg.Packages = []Package{{Name: "first", Selector: "pkg-one"}, {Name: "second", File: "two.zip"}}
	cfg.Scripts = []Script{{Name: "script", File: "script.sh", Args: []string{"", "space and 'quote'", "$(literal)"}}}
	first, err := Render(cfg)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Render(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("same config generated different scripts")
	}
	if bytes.Contains(first, []byte("\r")) || bytes.Contains(first, []byte("{{.Payload}}")) {
		t.Fatal("unrendered template or CRLF in script")
	}
	text := string(first)
	if strings.Index(text, "apply_env 'AA_CUSTOM'") > strings.Index(text, "apply_env 'ZZ_CUSTOM'") {
		t.Fatal("environment map output is nondeterministic")
	}
	if strings.Index(text, "install_package 'first'") > strings.Index(text, "install_package 'second'") {
		t.Fatal("package order changed")
	}
	if !strings.Contains(text, "REQUIRED_COMMANDS=( 'curl' 'ssh' 'unzip' )") {
		t.Fatal("required command order or deduplication lost")
	}
	if !strings.Contains(text, "SCAN_COMMANDS=( 'curl' 'ssh' 'ipmitool' 'snmpget' 'python3' 'python' 'unzip' )") {
		t.Fatal("required tools were not included in directory scanning")
	}
	script := filepath.Join(t.TempDir(), "installer.sh")
	if err := os.WriteFile(script, first, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(findBash(t), "-n", filepath.ToSlash(script))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated Bash syntax invalid: %v\n%s", err, output)
	}
	if !reflect.DeepEqual(cfg.Globals.Scan.Required, []string{"curl", "ssh"}) {
		t.Fatal("Render mutated caller's config")
	}
}

func findBash(t *testing.T) string {
	t.Helper()
	for _, path := range []string{`C:\Environment\Git\bin\bash.exe`, `C:\Environment\msys64\usr\bin\bash.exe`, "/bin/bash"} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	if path, err := exec.LookPath("bash"); err == nil {
		return path
	}
	t.Skip("Bash unavailable")
	return ""
}
