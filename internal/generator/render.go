package generator

import (
	"bytes"
	_ "embed"
	"fmt"
	"sort"
	"strings"
	"text/template"
)

//go:embed installer.sh.tmpl
var installerTemplate string

// ShellQuote preserves configuration values as data, including $, backticks and quotes.
func ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func Render(cfg Config) ([]byte, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg.defaults()
	var payload strings.Builder
	assign := func(key, value string) {
		fmt.Fprintf(&payload, "%s=%s\n", key, ShellQuote(value))
	}
	flag := func(key string, value bool) {
		text := "0"
		if value {
			text = "1"
		}
		assign(key, text)
	}
	array := func(key string, values []string) {
		fmt.Fprintf(&payload, "%s=(", key)
		for _, value := range values {
			fmt.Fprintf(&payload, " %s", ShellQuote(value))
		}
		payload.WriteString(" )\n")
	}
	assign("TITLE", cfg.Name)
	assign("CORE_ARCHIVE", cfg.Framework.Archive)
	assign("CORE_SHA256", strings.ToLower(cfg.Framework.SHA256))
	assign("CORE_VERSION", cfg.Framework.Version)
	assign("INSTALL_DIR_DEFAULT", cfg.Framework.InstallDir)
	assign("DATA_HOME_DEFAULT", cfg.Framework.Home)
	flag("AUTOLOAD", Enabled(cfg.Framework.Autoload))
	flag("DOCTOR_AUTO_REPAIR", cfg.Doctor.AutoRepair)
	flag("SCAN_ENABLED", Enabled(cfg.Globals.Scan.Enabled))
	required := append([]string{}, cfg.Globals.Scan.Required...)
	required = append(required, cfg.Doctor.RequiredCommands...)
	commands := append([]string{}, cfg.Globals.Scan.Commands...)
	commands = append(commands, required...)
	array("SCAN_COMMANDS", unique(commands))
	array("SCAN_DIRECTORIES", cfg.Globals.Scan.Directories)
	array("REQUIRED_COMMANDS", unique(required))
	payload.WriteString("\nconfig_environment() {\n    :\n")
	for _, key := range sortedKeys(cfg.Globals.Env) {
		fmt.Fprintf(&payload, "    apply_env %s %s\n", ShellQuote(key), ShellQuote(cfg.Globals.Env[key]))
	}
	payload.WriteString("}\n\nconfig_settings() {\n    :\n")
	for _, key := range sortedKeys(cfg.Globals.Settings) {
		fmt.Fprintf(&payload, "    apply_setting %s %s\n", ShellQuote(key), ShellQuote(cfg.Globals.Settings[key]))
	}
	payload.WriteString("}\n\nconfig_packages() {\n    :\n")
	for _, p := range cfg.Packages {
		fmt.Fprintf(&payload, "    install_package %s %s %s %s %d %d\n", ShellQuote(p.Name), ShellQuote(p.File), ShellQuote(strings.ToLower(p.SHA256)), ShellQuote(p.Selector), bit(p.Required), bit(Enabled(p.Default)))
	}
	payload.WriteString("}\n\nconfig_scripts() {\n    :\n")
	for _, s := range cfg.Scripts {
		fmt.Fprintf(&payload, "    install_script %s %s %s %d %d", ShellQuote(s.Name), ShellQuote(s.File), ShellQuote(strings.ToLower(s.SHA256)), bit(s.Required), bit(Enabled(s.Default)))
		for _, arg := range s.Args {
			fmt.Fprintf(&payload, " %s", ShellQuote(arg))
		}
		payload.WriteByte('\n')
	}
	payload.WriteString("}\n")
	tmpl, err := template.New("installer").Option("missingkey=error").Parse(installerTemplate)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	if err := tmpl.Execute(&output, struct{ Payload string }{payload.String()}); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func unique(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func bit(value bool) int {
	if value {
		return 1
	}
	return 0
}
