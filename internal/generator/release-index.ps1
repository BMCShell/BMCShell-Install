$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)

try {
    try {
        $indexFile = Get-Item -LiteralPath $env:BMCI_INDEX_PATH
    } catch {
        throw '无法打开下载的版本索引。'
    }
    if ($indexFile -isnot [IO.FileInfo] -or $indexFile.Length -gt 1MB) {
        throw '版本索引必须是大小不超过 1 MiB 的文件。'
    }

    try {
        $json = [IO.File]::ReadAllText($indexFile.FullName)
        if (!$json.TrimStart().StartsWith('{')) { throw '索引必须是 JSON 对象。' }
        $index = ConvertFrom-Json -InputObject $json
    } catch {
        throw '版本索引必须是有效的 JSON 对象。'
    }
    if (($index.schema -isnot [int] -and $index.schema -isnot [long]) -or $index.schema -ne 1 -or
        $index.product -isnot [string] -or $index.product -cne 'BMCShell' -or $index.releases -isnot [array]) {
        throw '版本索引格式不受支持。'
    }

    $version = if ($env:BMCI_INDEX_VERSION) { $env:BMCI_INDEX_VERSION } else { $index.latest }
    $versionPattern = '\A(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?\z'
    if ($version -isnot [string] -or $version.Length -gt 128 -or $version -cnotmatch $versionPattern) {
        throw '所选版本号缺失或无效。'
    }

    $releases = @($index.releases | Where-Object { $_.version -is [string] -and $_.version -ceq $version })
    if ($releases.Count -ne 1) {
        throw '版本索引必须恰好包含一条所选版本的记录。'
    }
    $release = $releases[0]
    $tag = $release.tag
    if ($tag -isnot [string] -or ($tag -cne $version -and $tag -cne "v$version")) {
        throw '发布标签与所选版本不一致。'
    }
    if ($release.artifacts -isnot [array]) {
        throw '所选版本缺少发布文件列表。'
    }
    $artifacts = @($release.artifacts | Where-Object { $_.platform -is [string] -and $_.platform -ceq 'windows-amd64' })
    if ($artifacts.Count -ne 1) {
        throw '所选版本必须恰好包含一个 Windows x64 发布文件。'
    }
    $artifact = $artifacts[0]

    # Match the original URL before decoding path segments to reject URI normalization tricks.
    $urlPattern = '\Ahttps://github\.com/BMCShell/BMCShell/releases/download/([0-9A-Za-z.+%-]+)/([0-9A-Za-z.+%-]+)\z'
    if ($artifact.url -isnot [string]) {
        throw '发行 ZIP 的 URL 缺失或无效。'
    }
    $urlMatch = [regex]::Match($artifact.url, $urlPattern)
    if (!$urlMatch.Success -or [uri]::UnescapeDataString($urlMatch.Groups[1].Value) -cne $tag -or
        [uri]::UnescapeDataString($urlMatch.Groups[2].Value) -cne "BMCShell-$version-windows-amd64.zip") {
        throw 'ZIP 的 URL 必须指向 BMCShell 官方 GitHub 仓库中对应版本的发布文件。'
    }
    if ($artifact.sha256 -isnot [string] -or $artifact.sha256 -cnotmatch '\A[0-9A-Fa-f]{64}\z') {
        throw '发行 ZIP 的 SHA-256 缺失或无效。'
    }

    [Console]::WriteLine($version)
    [Console]::WriteLine($artifact.url)
    [Console]::WriteLine($artifact.sha256.ToLowerInvariant())
} catch {
    [Console]::Error.WriteLine('BMCShell 版本索引：' + $_.Exception.Message)
    exit 1
}
