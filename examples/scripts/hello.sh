#!/usr/bin/env bash
set -euo pipefail

printf '附加脚本已启动。\n'
printf '说明：%s\n' "${1-未提供}"
printf 'BMCShell 数据目录：%s\n' "${BMCSHELL_HOME-未设置}"
printf '安装配置名称：%s\n' "${BMCSHELL_INSTALL_PROFILE-未设置}"
