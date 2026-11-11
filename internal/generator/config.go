package generator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
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
	ReleaseIndex string `json:"release_index,omitempty"`
	Archive      string `json:"archive,omitempty"`
	SHA256       string `json:"sha256,omitempty"`
	Version      string `json:"version,omitempty"`
	InstallDir   string `json:"install_dir,omitempty"`
	Home         string `json:"home,omitempty"`
	Autoload     *bool  `json:"autoload,omitempty"`
}

type Globals struct {
	Env           map[string]string       `json:"env,omitempty"`
	Settings      map[string]string       `json:"settings,omitempty"`
	PluginSources map[string]PluginSource `json:"plugin_sources,omitempty"`
	Scan          Scan                    `json:"scan,omitempty"`
}

type PluginSource struct {
	URL      string `json:"url"`
	Insecure bool   `json:"insecure,omitempty"`
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
	sourcePattern  = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)
)

func Enabled(value *bool) bool { return value == nil || *value }

// Parse rejects misspelled fields, duplicate keys, trailing values and oversized input.
func Parse(r io.Reader) (Config, error) {
	var cfg Config
	data, err := io.ReadAll(io.LimitReader(r, MaxConfigSize+1))
	if err != nil {
		return cfg, fmt.Errorf("读取配置失败：%w", err)
	}
	if len(data) > MaxConfigSize {
		return cfg, errors.New("配置文件超过 1 MiB")
	}
	// Accept UTF-8 BOM produced by older Windows editors.
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if err := checkDuplicateKeys(json.NewDecoder(bytes.NewReader(data))); err != nil {
		return cfg, configJSONError(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return cfg, configJSONError(err)
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	cfg.defaults()
	return cfg, nil
}

func configJSONError(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return errors.New("配置为空或 JSON 内容不完整")
	}
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) {
		return fmt.Errorf("JSON 语法错误（字节位置 %d）", syntax.Offset)
	}
	var typeError *json.UnmarshalTypeError
	if errors.As(err, &typeError) {
		if typeError.Field == "" {
			return errors.New("配置的顶层值必须是 JSON 对象")
		}
		return fmt.Errorf("配置字段 %q 的值类型不正确（字节位置 %d）", typeError.Field, typeError.Offset)
	}
	if key, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
		return fmt.Errorf("配置中包含未知字段 %s", key)
	}
	return fmt.Errorf("JSON 配置无效：%w", err)
}

func checkDuplicateKeys(decoder *json.Decoder) error {
	var visit func(int) error
	visit = func(depth int) error {
		if depth > 100 {
			return errors.New("JSON 嵌套超过 100 层")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if token == nil {
			return errors.New("JSON 值不能为 null。可选字段可直接省略")
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
					return errors.New("JSON 对象缺少字段名")
				}
				if seen[name] {
					return fmt.Errorf("字段 %q 重复", name)
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
			return errors.New("JSON 分隔符无效")
		}
		_, err = decoder.Token()
		return err
	}
	if err := visit(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("配置必须只包含一个 JSON 值")
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
		return errors.New("schema 必须为 1")
	}
	if err := cfg.Framework.validate(); err != nil {
		return err
	}
	for name, value := range map[string]string{"name": cfg.Name, "framework.install_dir": cfg.Framework.InstallDir, "framework.home": cfg.Framework.Home} {
		if err := plain(name, value); err != nil {
			return err
		}
	}
	for key, value := range cfg.Globals.Env {
		if !envPattern.MatchString(key) || reservedEnv(key) {
			return fmt.Errorf("globals.env 不支持环境变量 %q。请在 framework 或 globals.scan 中配置启动路径", key)
		}
		if err := plain("globals.env."+key, value); err != nil {
			return err
		}
	}
	for key, value := range cfg.Globals.Settings {
		if strings.TrimSpace(key) == "" || strings.HasPrefix(key, "-") {
			return fmt.Errorf("globals.settings 的设置键 %q 无效", key)
		}
		if err := plain("globals.settings 的设置键", key); err != nil {
			return err
		}
		if err := plain("globals.settings."+key, value); err != nil {
			return err
		}
	}
	for name, source := range cfg.Globals.PluginSources {
		if !sourcePattern.MatchString(name) {
			return fmt.Errorf("globals.plugin_sources 的源名称 %q 无效", name)
		}
		if _, exists := cfg.Globals.Settings["plugin.source."+name]; exists {
			return fmt.Errorf("插件源 %q 已在 globals.settings 中定义", name)
		}
		if err := validatePluginSource(name, source); err != nil {
			return err
		}
	}
	for _, list := range [][]string{cfg.Globals.Scan.Commands, cfg.Globals.Scan.Required, cfg.Doctor.RequiredCommands} {
		for _, command := range list {
			if !commandPattern.MatchString(command) {
				return fmt.Errorf("命令名 %q 无效。目录路径请填入 globals.scan.directories", command)
			}
		}
	}
	for _, directory := range cfg.Globals.Scan.Directories {
		if directory == "" {
			return errors.New("globals.scan.directories 中的路径不能为空")
		}
		if err := plain("globals.scan.directories", directory); err != nil {
			return err
		}
	}
	for i, p := range cfg.Packages {
		prefix := fmt.Sprintf("packages[%d]", i)
		if (p.File == "") == (p.Selector == "") {
			return fmt.Errorf("%s 的 file 和 selector 必须且只能设置一个", prefix)
		}
		if p.File != "" {
			if err := validateSource(prefix+".file", p.File, p.SHA256); err != nil {
				return err
			}
		} else if p.SHA256 != "" || strings.HasPrefix(p.Selector, "-") || strings.TrimSpace(p.Selector) == "" {
			return fmt.Errorf("%s 的 selector 无效，或在未设置 file 时提供了 sha256", prefix)
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

func (f Framework) validate() error {
	if f.ReleaseIndex == "" || f.Version != "" {
		if !versionPattern.MatchString(f.Version) || len(f.Version) > 128 {
			return errors.New("framework.version 必须是具体的语义化版本号")
		}
	}
	if f.ReleaseIndex == "" {
		return validateSource("framework.archive", f.Archive, f.SHA256)
	}
	if f.Archive != "" || f.SHA256 != "" {
		return errors.New("framework.release_index 不能与 archive 或 sha256 同时配置")
	}
	u, err := url.Parse(f.ReleaseIndex)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return errors.New("framework.release_index 必须是 HTTPS URL，且不能包含凭据或片段")
	}
	return plain("framework.release_index", f.ReleaseIndex)
}

func reservedEnv(key string) bool {
	if strings.HasPrefix(key, "BASH") {
		return true
	}
	switch key {
	case "PATH", "HOME", "BMCSHELL_HOME", "BMCSHELL_EXE", "BMCSHELL_CONTEXT", "ENV", "IFS", "SHELLOPTS", "CDPATH", "GLOBIGNORE", "PS4", "SHELL", "UID", "EUID", "PPID", "PWD", "OLDPWD", "SHLVL", "FUNCNAME", "DIRSTACK", "GROUPS", "PIPESTATUS", "RANDOM", "SECONDS", "LINENO", "OPTIND", "OPTARG":
		return true
	case "TITLE", "CORE_RELEASE_INDEX", "CORE_ARCHIVE", "CORE_SHA256", "CORE_VERSION", "INSTALL_DIR_DEFAULT", "DATA_HOME_DEFAULT", "AUTOLOAD", "DOCTOR_AUTO_REPAIR", "SCAN_ENABLED", "SCAN_COMMANDS", "SCAN_DIRECTORIES", "SCAN_BASE_PATH", "REQUIRED_COMMANDS", "WARNINGS", "SCRIPT_DIR", "WORK_DIR", "REPLY", "FETCH_COUNT", "FETCHED_FILE", "STAGED_CORE", "CHOSEN_PATH", "INSTALL_DIR", "DATA_HOME", "STATE_DIR", "LOAD_FILE", "FOUND_DIRS", "ENV_KEYS", "ENV_VALUES", "CATALOG_READY":
		return true
	}
	return strings.HasPrefix(key, "BMCSHELL_BRIDGE_")
}

func plain(name, value string) error {
	if strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return fmt.Errorf("%s 不能包含控制字符", name)
	}
	return nil
}

func validateStep(prefix, name string, required bool, defaultValue *bool) error {
	if required && !Enabled(defaultValue) {
		return fmt.Errorf("%s 为必需步骤，默认值不能设为跳过", prefix)
	}
	return plain(prefix+".name", name)
}

func validatePluginSource(name string, source PluginSource) error {
	field := "globals.plugin_sources." + name + ".url"
	if err := plain(field, source.URL); err != nil {
		return err
	}
	u, err := url.Parse(source.URL)
	if err != nil || len(source.URL) > 4096 || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("%s 必须为 HTTPS URL，也接受回环地址的 HTTP URL。地址不能包含凭据或片段", field)
	}
	host := u.Hostname()
	address, _ := netip.ParseAddr(host)
	if u.Scheme != "https" && !(u.Scheme == "http" && (address.IsLoopback() || strings.EqualFold(strings.TrimSuffix(host, "."), "localhost"))) {
		return fmt.Errorf("%s 必须为 HTTPS URL，也接受回环地址的 HTTP URL。地址不能包含凭据或片段", field)
	}
	if source.Insecure {
		if u.Scheme != "https" || !address.IsPrivate() || address.Zone() != "" {
			return fmt.Errorf("%s 启用 insecure 时，必须使用带私网 IP 字面量的 HTTPS URL", field)
		}
		if u.Port() != "" {
			port, err := strconv.ParseUint(u.Port(), 10, 16)
			if err != nil || port == 0 {
				return fmt.Errorf("%s 的端口无效", field)
			}
		}
	}
	return nil
}

func validateSource(name, source, hash string) error {
	if source == "" || strings.TrimSpace(source) == "" {
		return fmt.Errorf("必须填写 %s", name)
	}
	if err := plain(name, source); err != nil {
		return err
	}
	if hash != "" && !hashPattern.MatchString(hash) {
		return fmt.Errorf("%s 的 sha256 必须为 64 位十六进制字符", name)
	}
	if strings.Contains(source, "://") {
		u, err := url.Parse(source)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.Fragment != "" {
			return fmt.Errorf("%s 的远程文件必须使用无片段的 HTTPS URL", name)
		}
		if hash == "" {
			return fmt.Errorf("%s 的远程文件必须配置 sha256", name)
		}
	}
	return nil
}
