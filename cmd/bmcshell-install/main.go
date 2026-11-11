package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"bmcshell-install/internal/generator"
)

const version = "1.1.0"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "错误："+err.Error())
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		usage(out)
		return nil
	}
	switch args[0] {
	case "version", "--version":
		if len(args) == 2 && args[0] == "version" && (args[1] == "-h" || args[1] == "--help") {
			commandHelp(out, "version")
			return nil
		}
		if len(args) != 1 {
			return errors.New("version 不接受额外参数。请运行 BMCShell-Install version --help")
		}
		fmt.Fprintln(out, "BMCShell-Install", version)
		return nil
	case "help", "--help", "-h":
		usage(out)
		return nil
	case "generate", "validate", "init":
	default:
		return fmt.Errorf("未知命令 %q。请运行 BMCShell-Install --help", args[0])
	}
	action := args[0]
	flags := flag.NewFlagSet(action, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
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
			commandHelp(out, action)
			return nil
		}
		return fmt.Errorf("%s。请运行 BMCShell-Install %s --help", flagError(err), action)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("不接受位置参数 %q。请运行 BMCShell-Install %s --help", flags.Arg(0), action)
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
			return fmt.Errorf("生成 JSON 配置失败：%w", err)
		}
		if err := writeOutput(outputPath, append(data, '\n'), force, 0600); err != nil {
			return err
		}
		fmt.Fprintf(out, "已创建配置：%s\n", outputPath)
		return nil
	}
	if configPath == "" {
		return fmt.Errorf("缺少必填选项 --config。请运行 BMCShell-Install %s --help", action)
	}
	file, err := os.Open(configPath)
	if err != nil {
		return fmt.Errorf("无法打开配置文件 %q：%w", configPath, err)
	}
	cfg, err := generator.Parse(file)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return fmt.Errorf("无法关闭配置文件 %q：%w", configPath, closeErr)
	}
	if action == "validate" {
		fmt.Fprintf(out, "配置有效：%s\n", configPath)
		return nil
	}
	configAbs, err := filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("无法解析配置文件路径：%w", err)
	}
	outputAbs, err := filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("无法解析输出文件路径：%w", err)
	}
	if samePath(configAbs, outputAbs) {
		return errors.New("输出文件不能覆盖输入的 JSON 配置。请用 --output 指定其他路径")
	}
	data, err := generator.Render(cfg)
	if err != nil {
		return err
	}
	if err := writeOutput(outputPath, data, force, 0700); err != nil {
		return err
	}
	fmt.Fprintf(out, "已生成交互式安装脚本：%s\n请在交互式 Bash 中运行：bash %s\n本地资源路径相对于脚本所在目录。\n", outputPath, generator.ShellQuote(filepath.ToSlash(outputPath)))
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
		return errors.New("输出文件路径不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("无法创建输出目录：%w", err)
	}
	if !force {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				return fmt.Errorf("输出文件 %q 已存在。使用 --force 可覆盖", path)
			}
			return fmt.Errorf("无法创建输出文件 %q：%w", path, err)
		}
		_, writeErr := file.Write(data)
		closeErr := file.Close()
		if writeErr != nil {
			os.Remove(path)
			return fmt.Errorf("写入输出文件 %q 失败：%w", path, writeErr)
		}
		if closeErr != nil {
			return fmt.Errorf("无法关闭输出文件 %q：%w", path, closeErr)
		}
		return nil
	}
	// Stage the complete file beside its destination before publishing it.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".bmcshell-install-*.tmp")
	if err != nil {
		return fmt.Errorf("无法在输出目录中创建临时文件：%w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("写入临时文件失败：%w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return fmt.Errorf("无法设置临时文件权限：%w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("无法关闭临时文件：%w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("无法替换输出文件 %q：%w", path, err)
	}
	return nil
}

func flagError(err error) string {
	message := err.Error()
	if option, ok := strings.CutPrefix(message, "flag provided but not defined: "); ok {
		return "不支持选项 -" + option
	}
	if option, ok := strings.CutPrefix(message, "flag needs an argument: "); ok {
		return "选项 -" + option + " 缺少参数值"
	}
	if option, ok := strings.CutPrefix(message, "bad flag syntax: "); ok {
		return "选项格式无效：" + option
	}
	if strings.Contains(message, "for -force:") {
		return "--force 的值无效。请直接使用 --force，或指定 true 或 false"
	}
	return "无法解析选项：" + message
}

func usage(out io.Writer) {
	fmt.Fprintf(out, `BMCShell-Install %s

说明
  读取 JSON 配置并生成交互式 Bash 安装脚本。

用法
  BMCShell-Install <命令> [选项]
  BMCShell-Install <命令> --help

命令
  init      创建示例 JSON 配置
  validate  校验配置结构和字段
  generate  生成交互式 Bash 安装脚本
  version   显示生成器版本

示例
  BMCShell-Install init
  BMCShell-Install validate --config install.json
  BMCShell-Install generate --config install.json

使用说明
  运行生成的脚本后才会安装 BMCShell。
`, version)
}

func commandHelp(out io.Writer, action string) {
	switch action {
	case "init":
		fmt.Fprintln(out, `说明
  创建示例 JSON 配置，用于指定框架、扩展及安装选项。

用法
  BMCShell-Install init [--output <文件>] [--force]

参数与默认值
  --output <文件>  配置文件路径，默认 install.json。
  --force          覆盖已有输出文件。

示例
  BMCShell-Install init --output install.json

使用说明
  运行 generate 前，请先按实际环境修改生成的配置。`)
	case "validate":
		fmt.Fprintln(out, `说明
  校验安装配置的 JSON 格式、字段及取值。

用法
  BMCShell-Install validate --config <文件>

参数
  --config <文件>  待校验的 JSON 配置，必填。

示例
  BMCShell-Install validate --config install.json`)
	case "generate":
		fmt.Fprintln(out, `说明
  根据 JSON 配置生成交互式 Bash 安装脚本。

用法
  BMCShell-Install generate --config <文件> [--output <文件>] [--force]

参数与默认值
  --config <文件>  JSON 配置文件，必填。
  --output <文件>  安装脚本路径，默认 install-bmcshell.sh。
  --force          覆盖已有输出文件。

示例
  BMCShell-Install generate --config install.json --output install-bmcshell.sh
  bash ./install-bmcshell.sh

使用说明
  本地资源路径相对于生成脚本所在目录。`)
	case "version":
		fmt.Fprintln(out, `说明
  显示 BMCShell-Install 版本。

用法
  BMCShell-Install version
  BMCShell-Install --version`)
	}
}
