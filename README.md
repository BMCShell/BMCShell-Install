# BMCShell-Install

最终只需发布一个 `install-bmcshell.sh`，用户即可用一条 `curl` 命令启动交互安装。配置已写入脚本，安装用户不需要另外准备 JSON、Go 或生成器。在线安装先读取框架发行版索引，再下载所选发布包并核对索引里的 SHA256。

JSON 和 Go 生成器供维护者制作、更新安装脚本；生成后的 Bash 脚本不需要 Go 或 Python。

当前面向 **Windows x64 BMCShell 3.x 发布 ZIP**，在 **MobaXterm Bash、Git Bash 或 MSYS2 Bash（Bash 4 及以上）**中运行。解压使用 `unzip`，HTTPS 下载使用 `curl`，SHA256 校验使用 `sha256sum`、`shasum` 或 `openssl`。发行版索引由系统自带的 Windows PowerShell 5.1 或更高版本解析。

下载索引或发布包时，如果终端的 `curl` 因 CA 证书文件不可读而返回 77，安装器会尝试用 Windows 自带的 `curl.exe` 和系统证书库重新下载。下载安装脚本后直接运行即可，索引和发布包下载都保留 HTTPS 证书校验。

## 安装流程

1. 显示安装计划。在线索引模式先读取索引，选定框架版本，再由用户确认安装目录、框架和配置内容。
2. 取得框架发布 ZIP，核对 SHA256，安装并加载 `bin/bmcshell.sh`。符合所选版本的已有安装可交互选择复用，其他非空目录保留原有内容。
3. 扫描第三方命令，将确认的工具目录加入 PATH，设置全局环境变量和 `bmc setting` 参数，并保存配置中的插件源。
4. 执行 `bmc doctor -d`，按配置检查必须存在的命令；可以启用框架提供的交互修复。
5. 逐项选择并安装配置里的插件包，再逐项选择运行附加 Bash 脚本。
6. 安装插件后再次检查依赖，保存环境加载文件；按配置选择写入 `~/.bashrc` 的自动加载标记行。

每个可选项目支持选择安装或跳过。`required: true` 的项目必须选择并成功完成，否则取消安装。失败时可以重试、跳过可选项目或取消。退出和取消不会回滚此前已经完成的设置、插件安装或附加脚本操作。

安装必须在可交互终端运行；不提供 `--yes`。通过 `curl ... | bash` 启动时，Bash 先读取完整的安装代码，再从 `/dev/tty` 接收确认和输入；依赖修复、插件和附加脚本也使用该终端。没有可用终端时，安装器会退出并提示原因。附加脚本会继承安装时的环境，并作为独立 Bash 子进程执行；它修改的当前 Shell 变量不会自动回写。

## 快速开始

安装用户只需在 Bash 中运行发布方提供的一条命令：

```bash
curl -fsSL 'https://github.com/BMCShell/BMCShell-Install/releases/latest/download/install-bmcshell.sh' | bash
```

安装脚本从官方框架索引下载所选版本，并从公共软件源安装已选插件。

维护者在项目根目录构建生成器：

```powershell
go build -trimpath -o build/BMCShell-Install.exe ./cmd/bmcshell-install
```

内部在线配置 `profiles/install.online.json` 安装时自动设置 `official` 和 `sbmc`。脚本每次运行先读官方索引，取 `latest` 指向的版本及其 Windows x64 ZIP 下载地址和 SHA256。需要固定版本时，可在配置中填写索引里已有的版本。SBMC 源的 `--insecure` 需要 `3.0.0-dev.9` 或后续支持此选项的版本。[官方框架发行版索引](https://github.com/BMCShell/BMCShell/releases/download/release-index/index.json)

```powershell
./build/BMCShell-Install.exe validate --config profiles/install.online.json
./build/BMCShell-Install.exe generate --config profiles/install.online.json --output dist/install-bmcshell.sh
./build/BMCShell-Install.exe version
```

`profiles/` 由 Git 忽略。生成的脚本包含配置里的完整源地址，含 SBMC 源的脚本应放在内部发布地址。公开分发可使用 `examples/install.online.json`，该示例使用同一个发行版索引及 `official` 插件源。脚本运行时无需配置文件。在线安装时，框架和文件形式的插件、附加脚本通过 HTTPS 获取，按名称安装的插件由已配置的插件源提供。

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
| `release_index` | 字符串 | 框架发行版索引的 HTTPS URL，与 `archive` 和 `sha256` 互斥。 |
| `archive` | 字符串 | 直接安装时填写 Windows x64 框架发布 ZIP 的本地路径或 HTTPS URL。 |
| `sha256` | 字符串 | 直接安装时填写 ZIP 的 64 位十六进制 SHA256。远程 ZIP 必填，本地 ZIP 可省略。 |
| `version` | 字符串 | 直接安装时必填，必须与 ZIP 的版本相同。索引安装时可选，省略时使用索引的 `latest`。 |
| `install_dir` | 字符串 | 安装根目录，默认 `$HOME/.local/share/BMCShell`。 |
| `home` | 字符串 | BMCShell 数据目录，默认等于安装目录；通过 `BMCSHELL_HOME` 加载。 |
| `autoload` | 布尔值 | 默认 `true`；交互选择是否将 `.bmcshell-install/load.sh` 加载入口作为标记行写入 `~/.bashrc`。设为 `false` 时不修改启动文件。 |

路径字段支持 `~`、`$HOME` 和 `${HOME}` 表示当前 Bash 的家目录，不执行命令替换或其他 Shell 表达式。URL 只支持 HTTPS。本地路径建议使用 Bash 路径，如 `/d/Tools/BMCShell`；空格和中文可以作为 JSON 字符串正常填写。

使用 `release_index` 时，安装器检查索引的格式版本和产品名称，并从所选版本里读取唯一的 `windows-amd64` ZIP 资产。ZIP 地址须指向 `BMCShell/BMCShell` 的官方 GitHub Releases，下载后按索引提供的 SHA256 校验。

首次安装目录须为空，或不存在。复用已有安装只适用于可识别且版本与所选框架版本一致的安装。新装和复用时，安装器会检查 `BMCShell.exe` 的执行权限并按需补齐。已有目录中的用户数据保留原样。

### globals

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `env` | 字符串映射 | 保存到环境加载文件的全局变量。值按字面使用，不执行 `$HOME` 展开、反引号或命令替换。 |
| `settings` | 字符串映射 | 逐项调用 `bmc setting --set <key> <value>` 持久保存。 |
| `plugin_sources` | 按源名称索引的对象 | 自动设置插件源。每个源填写 `url`，可选 `insecure`，默认 `false`。 |
| `scan.enabled` | 布尔值 | 是否扫描第三方命令，默认 `true`。 |
| `scan.commands` | 字符串数组 | 扫描命令名，默认 `curl`、`ssh`、`ipmitool`、`snmpget`、`python3`、`python`。 |
| `scan.directories` | 字符串数组 | PATH 之外的候选工具目录，支持目录 glob，例如 `/drives/c/Tools/*/bin`。 |
| `scan.required` | 字符串数组 | 扫描阶段结束时必须能找到的命令。 |

扫描先查当前 PATH，再查配置的候选目录及常用终端目录；运行时也可补充目录。必需命令会一并加入扫描列表，确认得到的目录随环境文件持久加载。扫描用于查找工具，不自动安装工具。`env` 禁止设置 `PATH`、`HOME`、`BMCSHELL_HOME`、`BMCSHELL_EXE` 等系统和启动控制变量；使用 `scan.directories`、`framework.home` 等对应字段。

环境变量名称使用大写字母、数字和下划线，并以大写字母开头。安装器内部状态名称也属于保留名称，例如 `WORK_DIR`、`INSTALL_DIR`、`STATE_DIR`，配置校验会拒绝这些名称。

当前框架的普通设置项使用已登记的环境变量名，例如 `BMCSHELL_TLS_MODE`。特殊设置项包括 `use.default_resolver`、`theme.active` 和 `plugin.source.<名称>`。框架对受管理变量有自己的读取规则，业务参数优先放入 `settings`。配置文件和生成的安装脚本会保留配置中的原始值。

`plugin_sources` 的源名以小写字母开头，可含小写字母、数字和连字符，最长 48 字符。`url` 支持 HTTPS 和回环地址的 HTTP。安装时通过 `bmc setting --set plugin.source.<名称> <URL>` 保存每个源，终端只显示源名称。`insecure: true` 会给该源追加 `--insecure`，要求支持此选项的框架版本，并且地址必须使用私网 IP 字面量和 HTTPS。该选项从 `3.0.0-dev.9` 起可用，只对源配置的 IP 和端口生效。`settings` 仍接受原有的 `plugin.source.<名称>` 字符串，生成器会拒绝两处重复配置同一个源。

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

本地包通过 `bmc plugin install --file` 安装，软件源包通过 `bmc plugin install <selector>` 安装。使用选择器前，应在 `globals.plugin_sources` 中配置可访问的软件源。实际包名、版本、依赖和软件源访问权限以发布方提供的内容为准。

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
- 从官方框架发行版索引选择版本，并使用 `official` 插件源的在线安装配置。[公开在线示例](examples/install.online.json)

生成器把配置写入脚本。本地附件单独分发，索引和远程文件在安装时下载。框架 ZIP、远程插件和附加脚本在解压或执行前按索引或配置提供的 SHA256 校验。

开发验证：

```powershell
go test ./...
./build/BMCShell-Install.exe validate --config examples/install.local.json
./build/BMCShell-Install.exe validate --config examples/install.online.json
```

生成脚本语法可在目标 Bash 中用 `bash -n ./dist/install-bmcshell.sh` 检查。

2026-11-11 已通过 Go 测试，并在本机 Git Bash 5.3.9 中使用独立家目录及真实 `3.0.0-dev.8` 发布包验证首次安装、含空格和引号的路径、设置保存、附加脚本参数、已有安装复用、本地插件目录安装、失败跳过及自动加载去重。

单命令入口已用 Git Bash 的 `curl file://… | bash` 和同一真实框架发布包完成隔离安装验证：安装器及附加脚本都能接收终端输入，完成后正常退出。测试也覆盖了脚本流被截断时不启动安装。

已用本地 `3.0.0-dev.9` 发布包在 Git Bash 中验证双源安装。安装后的源配置与输入地址一致，`official` 保持证书校验，只有 `sbmc` 保存了 `insecureTls`。

已在本机 MobaXterm 的 Bash 中使用官方 `3.0.0-dev.9` 发布包验证首次安装和已有安装复用。两种流程都能补齐缺少的 `BMCShell.exe` 执行权限，随后加载框架并完成 `bmc doctor`。
