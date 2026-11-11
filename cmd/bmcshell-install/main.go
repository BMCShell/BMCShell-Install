package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"bmcshell-install/internal/generator"
)

const version = "1.0.0"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func run(args []string, out, errOut io.Writer) error {
	if len(args) == 0 {
		usage(out)
		return nil
	}
	switch args[0] {
	case "version", "--version":
		if len(args) != 1 {
			return errors.New("version does not accept arguments")
		}
		fmt.Fprintln(out, "BMCShell-Install", version)
		return nil
	case "help", "--help", "-h":
		usage(out)
		return nil
	case "generate", "validate", "init":
	default:
		return fmt.Errorf("unknown command %q; use --help", args[0])
	}
	action := args[0]
	flags := flag.NewFlagSet(action, flag.ContinueOnError)
	flags.SetOutput(errOut)
	var configPath, outputPath string
	var force bool
	if action != "init" {
		flags.StringVar(&configPath, "config", "", "JSON 配置文件")
	}
	if action != "validate" {
		defaultOutput := "install-bmcshell.sh"
		if action == "init" {
			defaultOutput = "install.json"
		}
		flags.StringVar(&outputPath, "output", defaultOutput, "输出文件")
		flags.BoolVar(&force, "force", false, "替换已有输出文件")
	}
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if action == "init" {
		cfg := generator.Config{
			Schema: 1, Name: "BMCShell 交互式安装",
			Framework: generator.Framework{Archive: "../payload/BMCShell-3.0.0-dev.8-windows-amd64.zip", Version: "3.0.0-dev.8", InstallDir: "$HOME/.local/share/BMCShell"},
			Globals: generator.Globals{
				Settings: map[string]string{"BMCSHELL_TLS_MODE": "verify", "BMCSHELL_SNMP_VERSION": "2c"},
				Scan:     generator.Scan{Commands: []string{"curl", "ssh", "ipmitool", "snmpget", "python3", "python"}, Directories: []string{"/drives/c/Environment/*/bin", "/c/Environment/*/bin"}},
			},
		}
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return err
		}
		if err := writeOutput(outputPath, append(data, '\n'), force, 0600); err != nil {
			return err
		}
		fmt.Fprintf(out, "已创建配置：%s\n", outputPath)
		return nil
	}
	if configPath == "" {
		return errors.New("--config is required")
	}
	file, err := os.Open(configPath)
	if err != nil {
		return err
	}
	cfg, err := generator.Parse(file)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if action == "validate" {
		fmt.Fprintf(out, "配置有效：%s\n", configPath)
		return nil
	}
	configAbs, err := filepath.Abs(configPath)
	if err != nil {
		return err
	}
	outputAbs, err := filepath.Abs(outputPath)
	if err != nil {
		return err
	}
	if samePath(configAbs, outputAbs) {
		return errors.New("output cannot replace the input JSON configuration")
	}
	data, err := generator.Render(cfg)
	if err != nil {
		return err
	}
	if err := writeOutput(outputPath, data, force, 0700); err != nil {
		return err
	}
	fmt.Fprintf(out, "已生成交互式安装脚本：%s\n在 Bash 中运行：bash <安装脚本路径>\n本地资源路径相对于生成脚本所在目录。\n", outputPath)
	return nil
}

func samePath(a, b string) bool {
	if a == b {
		return true
	}
	left, le := os.Stat(a)
	right, re := os.Stat(b)
	return le == nil && re == nil && os.SameFile(left, right)
}

func writeOutput(path string, data []byte, force bool, mode os.FileMode) error {
	if path == "" {
		return errors.New("output path cannot be empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if !force {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			return fmt.Errorf("create output (use --force to replace): %w", err)
		}
		_, writeErr := file.Write(data)
		closeErr := file.Close()
		if writeErr != nil {
			os.Remove(path)
			return writeErr
		}
		return closeErr
	}
	// Stage the complete file beside its destination before publishing it.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".bmcshell-install-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func usage(out io.Writer) {
	fmt.Fprintln(out, `BMCShell-Install 1.0.0 — JSON 配置生成交互式 Bash 安装脚本

  BMCShell-Install init [--output install.json] [--force]
  BMCShell-Install validate --config install.json
  BMCShell-Install generate --config install.json [--output install-bmcshell.sh] [--force]
  BMCShell-Install version

生成器仅创建脚本；运行生成的脚本后才会安装 BMCShell。`)
}
