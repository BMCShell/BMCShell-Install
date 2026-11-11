# BMCShell-Install

最终只需发布一个 `install-bmcshell.sh`，用户即可用一条 `curl` 命令启动交互安装。配置已写入脚本，安装用户不需要另外准备 JSON、Go 或生成器。脚本按内置地址下载框架、插件及附加脚本，并核对配置中的 SHA256。

JSON 和 Go 生成器供维护者制作、更新安装脚本；生成后的 Bash 脚本不需要 Go 或 Python。

当前面向 **Windows x64 BMCShell 3.x 发布 ZIP**，在 **MobaXterm Bash、Git Bash 或 MSYS2 Bash（Bash 4 及以上）**中运行。需要 `unzip`；HTTPS 下载需要 `curl`；SHA256 校验需要 `sha256sum`、`shasum` 或 `openssl` 之一。

## 安装流程

1. 显示安装计划，由用户确认安装目录、框架和配置内容。
2. 取得框架发布 ZIP，核对 SHA256，安装并加载 `bin/bmcshell.sh`。非空目录不会被覆盖；符合配置版本的已有安装可交互选择复用。
3. 扫描第三方命令，将确认的工具目录加入 PATH；设置配置里的全局环境变量和 `bmc setting` 参数。
4. 执行 `bmc doctor -d`，按配置检查必须存在的命令；可以启用框架提供的交互修复。
5. 逐项选择并安装配置里的插件包，再逐项选择运行附加 Bash 脚本。
6. 安装插件后再次检查依赖，保存环境加载文件；按配置选择写入 `~/.bashrc` 的自动加载标记行。

每个可选项目支持选择安装或跳过。`required: true` 的项目必须选择并成功完成，否则取消安装。失败时可以重试、跳过可选项目或取消。退出和取消不会回滚此前已经完成的设置、插件安装或附加脚本操作。

安装必须在可交互终端运行；不提供 `--yes`。通过 `curl ... | bash` 启动时，Bash 先读取完整的安装代码，再从 `/dev/tty` 接收确认和输入；依赖修复、插件和附加脚本也使用该终端。没有可用终端时，安装器会退出并提示原因。附加脚本会继承安装时的环境，并作为独立 Bash 子进程执行；它修改的当前 Shell 变量不会自动回写。

## 快速开始

安装用户只需在 Bash 中运行发布方提供的一条命令：

```bash
curl -fsSL 'https://example.invalid/install-bmcshell.sh' | bash
```

上面的 `example.invalid` 是占位地址，不能实际安装。维护者需要先填写真实发布包地址和校验值，生成脚本并上传到可访问的 HTTPS 地址，再提供对应命令。用户只下载这一个入口脚本，框架和所选组件由它继续下载。

维护者在项目根目录构建生成器：

```powershell
go build -trimpath -o build/BMCShell-Install.exe ./cmd/bmcshell-install
```

复制 `examples/install.online.json`，填写真实下载地址、SHA256 和所需参数后生成在线安装脚本：

```powershell
./build/BMCShell-Install.exe validate --config install.json
./build/BMCShell-Install.exe generate --config install.json --output dist/install-bmcshell.sh
./build/BMCShell-Install.exe version
```

将生成的 `dist/install-bmcshell.sh` 上传到发布地址即可；运行时不再读取维护者的 `install.json`。在线分发时，框架和文件形式的插件、附加脚本均应配置为 HTTPS URL，软件源安装的插件使用已配置的软件源。

需要离线安装时，可以用 `init --output install.json` 创建本地初始配置，或直接使用 `examples/install.local.json`：

```powershell
./build/BMCShell-Install.exe generate --config examples/install.local.json --output dist/install-bmcshell-local.sh
```

已有输出默认不会被覆盖。需要覆盖时给 `init` 或 `generate` 追加 `--force`。`validate` 检查配置格式和约束，实际文件、下载地址、发布包内容及工具可用性由生成的安装脚本运行时检查。

离线示例需将正式框架 ZIP 放入项目 `payload/`，然后从 Bash 启动：

```bash
bash ./dist/install-bmcshell-local.sh
```

本地执行时，相对文件路径按**生成后安装脚本所在目录**解析。例如 `dist/install-bmcshell-local.sh` 使用 `../payload/core.zip` 时，查找的是项目根目录下的 `payload/core.zip`，与生成器运行目录及 JSON 所在目录无关。离线分发时请同时保持本地附件的目录关系。通过管道运行时没有脚本文件目录，相对路径以启动命令时的工作目录为基准；在线配置应使用 HTTPS 资源地址。

安装完成后，在新 Bash 终端加载安装目录下的 `.bmcshell-install/load.sh`；选择自动加载后，重开终端即可使用 `bmc version`。安装脚本作为子进程运行，不能直接修改启动它的父终端环境。

登录型 Bash 如果尚未读取 `~/.bashrc`，需在自己的 `~/.bash_profile` 中加入 `[[ ! -f ~/.bashrc ]] || source ~/.bashrc`，使自动加载入口生效。

## JSON 配置

完整格式见 [`config.schema.json`](config.schema.json)。不认识的字段会被拒绝，以便及时发现拼写错误。`schema` 必须为数字 `1`；`name` 是可选的安装标题，省略时生成器使用默认名称。

### framework

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `archive` | 字符串，必填 | 当前 Windows x64 框架发布 ZIP 的本地路径或 HTTPS URL。 |
| `sha256` | 字符串 | ZIP 的 64 位十六进制 SHA256。远程文件必填；本地文件可省略，建议填写。 |
| `version` | 字符串，必填 | 必须与 ZIP 中的框架版本相同，例如 `3.0.0-dev.8`。 |
| `install_dir` | 字符串 | 安装根目录，默认 `$HOME/.local/share/BMCShell`。 |
| `home` | 字符串 | BMCShell 数据目录，默认等于安装目录；通过 `BMCSHELL_HOME` 加载。 |
| `autoload` | 布尔值 | 默认 `true`；交互选择是否将 `.bmcshell-install/load.sh` 加载入口作为标记行写入 `~/.bashrc`。设为 `false` 时不修改启动文件。 |

路径字段支持 `~`、`$HOME` 和 `${HOME}` 表示当前 Bash 的家目录，不执行命令替换或其他 Shell 表达式。URL 只支持 HTTPS。本地路径建议使用 Bash 路径，如 `/d/Tools/BMCShell`；空格和中文可以作为 JSON 字符串正常填写。

首次安装目录须为空，或不存在。复用已有安装只适用于可识别且版本与配置一致的安装。已有目录中的用户数据不会被当成新框架文件覆盖。

### globals

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `env` | 字符串映射 | 保存到环境加载文件的全局变量。值按字面使用，不执行 `$HOME` 展开、反引号或命令替换。 |
| `settings` | 字符串映射 | 逐项调用 `bmc setting --set <key> <value>` 持久保存。 |
| `scan.enabled` | 布尔值 | 是否扫描第三方命令，默认 `true`。 |
| `scan.commands` | 字符串数组 | 扫描命令名，默认 `curl`、`ssh`、`ipmitool`、`snmpget`、`python3`、`python`。 |
| `scan.directories` | 字符串数组 | PATH 之外的候选工具目录，支持目录 glob，例如 `/drives/c/Tools/*/bin`。 |
| `scan.required` | 字符串数组 | 扫描阶段结束时必须能找到的命令。 |

扫描先查当前 PATH，再查配置的候选目录及常用终端目录；运行时也可补充目录。必需命令会一并加入扫描列表，确认得到的目录随环境文件持久加载。扫描用于查找工具，不自动安装工具。`env` 禁止设置 `PATH`、`HOME`、`BMCSHELL_HOME`、`BMCSHELL_EXE` 等系统和启动控制变量；使用 `scan.directories`、`framework.home` 等对应字段。

环境变量名称使用大写字母、数字和下划线，并以大写字母开头。安装器内部状态名称也属于保留名称，例如 `WORK_DIR`、`INSTALL_DIR`、`STATE_DIR`，配置校验会拒绝这些名称。

当前框架的普通设置项使用已登记的环境变量名，例如 `BMCSHELL_TLS_MODE`，不是逻辑字段名 `tls.mode`。特殊设置项包括 `use.default_resolver`、`theme.active` 和 `plugin.source.<名称>`。普通 `setting` 不接受未知字段或敏感字段。框架对受管理变量有自己的读取规则，业务参数优先放入 `settings`，不要依赖通用 `env` 覆盖框架设置。配置文件和生成的安装脚本均以明文保存值，请勿放入密码、令牌等凭据。

解析器和主题设置需要对应插件已经存在。由于安装顺序是先设置全局参数、后安装插件，新安装时不要在 `settings` 中引用尚未安装的解析器或主题；可在后续附加脚本中完成这些设置。

### doctor

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `auto_repair` | 布尔值 | 默认 `false`；启用时调用框架的交互依赖修复。 |
| `required_commands` | 字符串数组 | 依赖检查阶段必须在 PATH 中可用的命令。 |

当前 `bmc doctor` 即使报告缺失依赖也可能返回退出码 0。安装脚本显示 `bmc doctor -d` 的完整结果，再用 `bmc doctor -f missing` 查询缺失项；发现缺失时允许交互处理和重新检查。需要强制安装前提时，明确填写 `scan.required` 或 `doctor.required_commands`；这些命令缺失时不能选择继续。框架按**已经安装的插件**声明检查依赖，因此安装前后各检查一次。

`bmc doctor -a` 的自动修复目前仅支持 Windows x64 的 MSYS2，并要求使用该 MSYS2 自身的 Bash、pacman 和软件包数据库。MobaXterm、Git Bash 仍可检查依赖，但不能据此自动调用 MSYS2 修复。修复过程中保留框架和包管理器的交互确认。

### packages

插件包按数组顺序处理。每项包含以下字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `name` | 字符串 | 交互界面显示名称；可省略，使用 `file` 或 `selector` 的值。 |
| `file` | 字符串 | 本地包目录、ZIP，或 HTTPS ZIP URL。与 `selector` 二选一。 |
| `sha256` | 字符串 | 文件的 SHA256；远程文件必填，本地 ZIP 可选。 |
| `selector` | 字符串 | 软件源包名，可带版本，如 `bmcshell-tool-ipmi@1.0.1`。与 `file` 二选一。 |
| `required` | 布尔值 | 默认 `false`；为 `true` 时必须安装成功。 |
| `default` | 布尔值 | 默认 `true`；交互选择的默认值。 |

`required: true` 不能与 `default: false` 同时使用。

本地包通过 `bmc plugin install --file` 安装；软件源包通过 `bmc plugin install <selector>` 安装。使用选择器前，应在 `globals.settings` 配置可访问的软件源，例如 `plugin.source.company`。实际包名、版本、依赖和软件源访问权限以发布方提供的内容为准。

### scripts

附加脚本按数组顺序执行，适合配置里声明的依赖安装、初始化或项目定制操作。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `name` | 字符串 | 显示名称，可省略，使用 `file` 的值。 |
| `file` | 字符串，必填 | 本地 Bash 脚本或 HTTPS URL。 |
| `sha256` | 字符串 | 脚本 SHA256；远程必填，本地可选。 |
| `args` | 字符串数组 | 传给脚本的独立参数，不作为 Shell 命令拼接。默认空数组。 |
| `required` | 布尔值 | 默认 `false`；必须成功完成时设为 `true`。 |
| `default` | 布尔值 | 默认 `true`，控制交互选择的默认值。 |

附加脚本在框架、全局参数和第一次依赖检查完成后执行。脚本使用继承到的 `BMCSHELL_HOME`、环境变量和 PATH；如需 `bmc` Bash 函数，应主动加载安装入口。`examples/scripts/hello.sh` 只显示示例信息，便于验证附加步骤。

## 示例与校验

- [`examples/install.local.json`](examples/install.local.json)：离线框架 ZIP 和本地附加脚本示例。
- [`examples/install.online.json`](examples/install.online.json)：HTTPS 分发和软件源安装示例。所有 `example.invalid` URL 和 SHA256 均为占位内容，必须替换为真实发布地址和校验值才能安装。

生成器不会把本地附件打包到输出脚本，也不会在生成时下载发布物。远程内容会在用户确认相应安装步骤后下载，并在执行或解压前验证配置提供的 SHA256。

开发验证：

```powershell
go test ./...
./build/BMCShell-Install.exe validate --config examples/install.local.json
./build/BMCShell-Install.exe validate --config examples/install.online.json
```

生成脚本语法可在目标 Bash 中用 `bash -n ./dist/install-bmcshell.sh` 检查。

2026-11-11 已通过 `go test ./...`，并在本机 Git Bash 5.3.9 中使用独立家目录及真实 `3.0.0-dev.8` 发布包验证首次安装、含空格和引号的路径、设置保存、附加脚本参数、已有安装复用、本地插件目录安装、失败跳过及自动加载去重。MobaXterm、MSYS2 的实际交互安装、HTTPS 下载和包管理器自动修复尚未实测。

单命令入口已用 Git Bash 的 `curl file://… | bash` 和同一真实框架发布包完成隔离安装验证：安装器及附加脚本都能接收终端输入，完成后正常退出。测试也覆盖了脚本流被截断时不启动安装。
