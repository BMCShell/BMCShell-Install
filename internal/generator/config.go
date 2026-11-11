package generator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

const MaxConfigSize = 1024 * 1024

type Config struct {
	Schema    int       `json:"schema"`
	Name      string    `json:"name,omitempty"`
	Framework Framework `json:"framework"`
	Globals   Globals   `json:"globals,omitempty"`
	Doctor    Doctor    `json:"doctor,omitempty"`
	Packages  []Package `json:"packages,omitempty"`
	Scripts   []Script  `json:"scripts,omitempty"`
}

type Framework struct {
	Archive    string `json:"archive"`
	SHA256     string `json:"sha256,omitempty"`
	Version    string `json:"version"`
	InstallDir string `json:"install_dir,omitempty"`
	Home       string `json:"home,omitempty"`
	Autoload   *bool  `json:"autoload,omitempty"`
}

type Globals struct {
	Env      map[string]string `json:"env,omitempty"`
	Settings map[string]string `json:"settings,omitempty"`
	Scan     Scan              `json:"scan,omitempty"`
}

type Scan struct {
	Enabled     *bool    `json:"enabled,omitempty"`
	Commands    []string `json:"commands,omitempty"`
	Directories []string `json:"directories,omitempty"`
	Required    []string `json:"required,omitempty"`
}

type Doctor struct {
	AutoRepair       bool     `json:"auto_repair,omitempty"`
	RequiredCommands []string `json:"required_commands,omitempty"`
}

type Package struct {
	Name     string `json:"name,omitempty"`
	File     string `json:"file,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	Selector string `json:"selector,omitempty"`
	Required bool   `json:"required,omitempty"`
	Default  *bool  `json:"default,omitempty"`
}

type Script struct {
	Name     string   `json:"name,omitempty"`
	File     string   `json:"file"`
	SHA256   string   `json:"sha256,omitempty"`
	Args     []string `json:"args,omitempty"`
	Required bool     `json:"required,omitempty"`
	Default  *bool    `json:"default,omitempty"`
}

var (
	versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
	hashPattern    = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
	envPattern     = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	commandPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.+-]*$`)
)

func Enabled(value *bool) bool { return value == nil || *value }

// Parse rejects misspelled fields, duplicate keys, trailing values and oversized input.
func Parse(r io.Reader) (Config, error) {
	var cfg Config
	data, err := io.ReadAll(io.LimitReader(r, MaxConfigSize+1))
	if err != nil {
		return cfg, err
	}
	if len(data) > MaxConfigSize {
		return cfg, errors.New("configuration exceeds 1 MiB")
	}
	// Accept UTF-8 BOM produced by older Windows editors.
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if err := checkDuplicateKeys(json.NewDecoder(bytes.NewReader(data))); err != nil {
		return cfg, fmt.Errorf("JSON: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("JSON: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	cfg.defaults()
	return cfg, nil
}

func checkDuplicateKeys(decoder *json.Decoder) error {
	var visit func(int) error
	visit = func(depth int) error {
		if depth > 100 {
			return errors.New("JSON nesting exceeds 100 levels")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if token == nil {
			return errors.New("null is not allowed; omit optional fields instead")
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := make(map[string]bool)
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok {
					return errors.New("expected object key")
				}
				if seen[name] {
					return fmt.Errorf("duplicate key %q", name)
				}
				seen[name] = true
				if err := visit(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := visit(depth + 1); err != nil {
					return err
				}
			}
		default:
			return errors.New("unexpected delimiter")
		}
		_, err = decoder.Token()
		return err
	}
	if err := visit(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("expected one JSON value")
	}
	return nil
}

func (cfg *Config) defaults() {
	if cfg.Name == "" {
		cfg.Name = "BMCShell 交互式安装"
	}
	if cfg.Framework.InstallDir == "" {
		cfg.Framework.InstallDir = "$HOME/.local/share/BMCShell"
	}
	if cfg.Globals.Scan.Commands == nil {
		cfg.Globals.Scan.Commands = []string{"curl", "ssh", "ipmitool", "snmpget", "python3", "python"}
	}
	for i := range cfg.Packages {
		p := &cfg.Packages[i]
		if p.Name == "" {
			p.Name = p.Selector
			if p.Name == "" {
				p.Name = p.File
			}
		}
	}
	for i := range cfg.Scripts {
		if cfg.Scripts[i].Name == "" {
			cfg.Scripts[i].Name = cfg.Scripts[i].File
		}
	}
}

func (cfg Config) Validate() error {
	if cfg.Schema != 1 {
		return errors.New("schema must be 1")
	}
	if !versionPattern.MatchString(cfg.Framework.Version) || len(cfg.Framework.Version) > 128 {
		return errors.New("framework.version must be an exact semantic version")
	}
	if err := validateSource("framework.archive", cfg.Framework.Archive, cfg.Framework.SHA256); err != nil {
		return err
	}
	for name, value := range map[string]string{"name": cfg.Name, "framework.install_dir": cfg.Framework.InstallDir, "framework.home": cfg.Framework.Home} {
		if err := plain(name, value); err != nil {
			return err
		}
	}
	for key, value := range cfg.Globals.Env {
		if !envPattern.MatchString(key) || reservedEnv(key) {
			return fmt.Errorf("globals.env: %q is not an allowed environment variable; use framework or scan options for startup paths", key)
		}
		if err := plain("globals.env."+key, value); err != nil {
			return err
		}
	}
	for key, value := range cfg.Globals.Settings {
		if strings.TrimSpace(key) == "" || strings.HasPrefix(key, "-") {
			return fmt.Errorf("globals.settings: invalid key %q", key)
		}
		if err := plain("globals.settings key", key); err != nil {
			return err
		}
		if err := plain("globals.settings."+key, value); err != nil {
			return err
		}
	}
	for _, list := range [][]string{cfg.Globals.Scan.Commands, cfg.Globals.Scan.Required, cfg.Doctor.RequiredCommands} {
		for _, command := range list {
			if !commandPattern.MatchString(command) {
				return fmt.Errorf("invalid command name %q (paths belong in globals.scan.directories)", command)
			}
		}
	}
	for _, directory := range cfg.Globals.Scan.Directories {
		if directory == "" {
			return errors.New("globals.scan.directories cannot contain an empty path")
		}
		if err := plain("globals.scan.directories", directory); err != nil {
			return err
		}
	}
	for i, p := range cfg.Packages {
		prefix := fmt.Sprintf("packages[%d]", i)
		if (p.File == "") == (p.Selector == "") {
			return fmt.Errorf("%s: set exactly one of file and selector", prefix)
		}
		if p.File != "" {
			if err := validateSource(prefix+".file", p.File, p.SHA256); err != nil {
				return err
			}
		} else if p.SHA256 != "" || strings.HasPrefix(p.Selector, "-") || strings.TrimSpace(p.Selector) == "" {
			return fmt.Errorf("%s: invalid selector or SHA256 supplied without file", prefix)
		}
		if err := validateStep(prefix, p.Name, p.Required, p.Default); err != nil {
			return err
		}
		if err := plain(prefix+".selector", p.Selector); err != nil {
			return err
		}
	}
	for i, s := range cfg.Scripts {
		prefix := fmt.Sprintf("scripts[%d]", i)
		if err := validateSource(prefix+".file", s.File, s.SHA256); err != nil {
			return err
		}
		if err := validateStep(prefix, s.Name, s.Required, s.Default); err != nil {
			return err
		}
		for _, arg := range s.Args {
			if err := plain(prefix+".args", arg); err != nil {
				return err
			}
		}
	}
	return nil
}

func reservedEnv(key string) bool {
	if strings.HasPrefix(key, "BASH") {
		return true
	}
	switch key {
	case "PATH", "HOME", "BMCSHELL_HOME", "BMCSHELL_EXE", "BMCSHELL_CONTEXT", "ENV", "IFS", "SHELLOPTS", "CDPATH", "GLOBIGNORE", "PS4", "SHELL", "UID", "EUID", "PPID", "PWD", "OLDPWD", "SHLVL", "FUNCNAME", "DIRSTACK", "GROUPS", "PIPESTATUS", "RANDOM", "SECONDS", "LINENO", "OPTIND", "OPTARG":
		return true
	case "TITLE", "CORE_ARCHIVE", "CORE_SHA256", "CORE_VERSION", "INSTALL_DIR_DEFAULT", "DATA_HOME_DEFAULT", "AUTOLOAD", "DOCTOR_AUTO_REPAIR", "SCAN_ENABLED", "SCAN_COMMANDS", "SCAN_DIRECTORIES", "SCAN_BASE_PATH", "REQUIRED_COMMANDS", "WARNINGS", "SCRIPT_DIR", "WORK_DIR", "REPLY", "FETCH_COUNT", "FETCHED_FILE", "STAGED_CORE", "CHOSEN_PATH", "INSTALL_DIR", "DATA_HOME", "STATE_DIR", "LOAD_FILE", "FOUND_DIRS", "ENV_KEYS", "ENV_VALUES", "CATALOG_READY":
		return true
	}
	return strings.HasPrefix(key, "BMCSHELL_BRIDGE_")
}

func plain(name, value string) error {
	if strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return fmt.Errorf("%s cannot contain control characters", name)
	}
	return nil
}

func validateStep(prefix, name string, required bool, defaultValue *bool) error {
	if required && !Enabled(defaultValue) {
		return fmt.Errorf("%s: a required step cannot default to skipped", prefix)
	}
	return plain(prefix+".name", name)
}

func validateSource(name, source, hash string) error {
	if source == "" || strings.TrimSpace(source) == "" {
		return fmt.Errorf("%s is required", name)
	}
	if err := plain(name, source); err != nil {
		return err
	}
	if hash != "" && !hashPattern.MatchString(hash) {
		return fmt.Errorf("%s: sha256 must contain 64 hexadecimal characters", name)
	}
	if strings.Contains(source, "://") {
		u, err := url.Parse(source)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.Fragment != "" {
			return fmt.Errorf("%s: remote files require an HTTPS URL without fragment", name)
		}
		if hash == "" {
			return fmt.Errorf("%s: remote files require sha256", name)
		}
	}
	return nil
}
