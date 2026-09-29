# imgConv fnOS 打包脚本（SSH 远程编译后端）
# 用法：在 PowerShell 中执行 .\build.ps1
# 说明：imgConv 后端依赖 cgo + libvips，无法像纯 Go 项目那样在 Windows 本地交叉编译，
#       因此脚本把 backend 源码通过 SSH 上传到飞牛机器编译（原生架构），再把二进制回传，
#       最后在 Windows 本地用 fnpack 打包成 .fpk。
# 产物：imgconv-x86-<version>.fpk 或 imgconv-arm-<version>.fpk（取决于目标架构）

param(
    [string]$FnosHost = "172.16.20.25",   # 飞牛机器地址
    [string]$User     = "admin",          # SSH 用户名
    [int]   $Port     = 22,               # SSH 端口
    [ValidateSet("auto", "amd64", "arm64")]
    [string]$Arch     = "auto",           # 目标架构：auto=按远端自动检测
    [string]$RemoteDir = "/tmp/imgconv-build", # 远端编译工作目录
    [string]$Fnpk     = "fnpack",         # fnpack 可执行文件（PATH 或绝对路径）
    [switch]$SkipFrontend                  # 跳过前端构建（复用已有 frontend/dist）
)

$ErrorActionPreference = "Stop"
$Root = $PSScriptRoot
$FpkDir = Join-Path $Root "imgConv"
$ManifestFile = Join-Path $FpkDir "manifest"

# manifest 为 UTF-8 无 BOM；显式编码读写以保持中文完整
$Utf8NoBom = [System.Text.UTF8Encoding]::new($false)

# 读取版本号与原始 platform
$manifestText = [System.IO.File]::ReadAllText($ManifestFile, $Utf8NoBom)
$Version = ([regex]::Match($manifestText, '(?m)^version\s*=\s*([^\r\n]+)')).Groups[1].Value.Trim()
$OriginalPlatform = ([regex]::Match($manifestText, '(?m)^platform\s*=\s*([^\r\n]+)')).Groups[1].Value.Trim()
if (-not $Version) { throw "无法从 manifest 读取 version" }
if (-not $OriginalPlatform) { throw "无法从 manifest 读取 platform" }

function Set-ManifestPlatform {
    param([string]$Platform)
    $text = [System.IO.File]::ReadAllText($ManifestFile, $Utf8NoBom)
    $text = [regex]::Replace($text, '(?m)^(platform\s*=\s*)[^\r\n]*', ('${1}' + $Platform))
    [System.IO.File]::WriteAllText($ManifestFile, $text, $Utf8NoBom)
}

# ---- SSH / SCP 封装（使用 Windows 自带 OpenSSH 客户端） ----
$SshTarget = "$User@$FnosHost"

function Invoke-Ssh {
    param([string]$Command)
    $out = & ssh -p $script:Port -o "StrictHostKeyChecking=accept-new" -o "ConnectTimeout=10" $script:SshTarget $Command 2>&1
    if ($LASTEXITCODE -ne 0) {
        $joined = $out -join "`n"
        throw "SSH 执行失败：$joined"
    }
    return $out
}

function Invoke-ScpUpload {
    param([string]$Local, [string]$Remote)
    $remotePath = "{0}:{1}" -f $script:SshTarget, $Remote
    & scp -P $script:Port -o "StrictHostKeyChecking=accept-new" -r $Local $remotePath
    if ($LASTEXITCODE -ne 0) { throw "SCP 上传失败：$Local" }
}

function Invoke-ScpDownload {
    param([string]$Remote, [string]$Local)
    $remotePath = "{0}:{1}" -f $script:SshTarget, $Remote
    & scp -P $script:Port -o "StrictHostKeyChecking=accept-new" $remotePath $Local
    if ($LASTEXITCODE -ne 0) { throw "SCP 下载失败：$Remote" }
}

# ---- 检查 fnpack 是否可用 ----
$fnpackOk = $false
if ($Fnpk -match '[\\/]') { $fnpackOk = Test-Path $Fnpk }
else { $fnpackOk = [bool](Get-Command $Fnpk -ErrorAction SilentlyContinue) }
if (-not $fnpackOk) {
    throw "未找到 fnpack（$Fnpk）。请下载 fnpack Windows 版（https://static2.fnnas.com/fnpack/fnpack-1.2.3-windows-amd64）并加入 PATH，或用 -Fnpk 指定路径。"
}

# ---- 1/4 构建前端 ----
Write-Host "==> 1/4 构建前端 ..."
if (-not $SkipFrontend) {
    Push-Location (Join-Path $Root "frontend")
    npm.cmd install | Out-Null
    npm.cmd run build
    if ($LASTEXITCODE -ne 0) { Pop-Location; throw "前端构建失败" }
    Pop-Location
}
$DistSrc = Join-Path $Root "frontend\dist"
if (-not (Test-Path $DistSrc)) { throw "未找到 frontend/dist，请先运行 npm run build" }
$DistDst = Join-Path $FpkDir "app\dist"
Remove-Item -Recurse -Force $DistDst -ErrorAction SilentlyContinue
Copy-Item -Recurse -Force $DistSrc $DistDst

# ---- 2/4 远程编译后端 ----
Write-Host "==> 2/4 远程编译后端 (cgo + libvips) ..."
$null = Invoke-Ssh "mkdir -p $RemoteDir && rm -rf $RemoteDir/backend"
Invoke-ScpUpload (Join-Path $Root "backend") "$RemoteDir/"

if ($Arch -eq "auto") {
    $raw = ((Invoke-Ssh "uname -m") -join "`n").Trim()
    switch -Regex ($raw) {
        'x86_64|amd64' { $Arch = "amd64" }
        'aarch64|arm64' { $Arch = "arm64" }
        default { throw "无法识别的远端架构：$raw" }
    }
    Write-Host "    检测到远端架构：$Arch"
}
$goarch   = if ($Arch -eq "amd64") { "amd64" } else { "arm64" }
$platform = if ($Arch -eq "amd64") { "x86" } else { "arm" }

$buildCmd = @"
set -e
export GOPROXY="https://goproxy.cn,direct"
command -v go >/dev/null 2>&1 || { echo "远端缺少 Go，请先在飞牛安装 Go 1.25+" >&2; exit 1; }
if ! pkg-config --exists vips 2>/dev/null; then
  echo "    安装 libvips 开发依赖 ..."
  apt-get update >/dev/null 2>&1 || true
  apt-get install -y --no-install-recommends build-essential pkg-config libvips-dev >/dev/null 2>&1 || { echo "libvips-dev 安装失败" >&2; exit 1; }
fi
cd "$RemoteDir/backend"
CGO_ENABLED=1 GOOS=linux GOARCH=$goarch go build -o imgconv .
"@
$null = Invoke-Ssh $buildCmd

# ---- 3/4 回传二进制 ----
Write-Host "==> 3/4 回传后端二进制 ..."
$binLocal = Join-Path $FpkDir "app\imgconv"
Invoke-ScpDownload "$RemoteDir/backend/imgconv" $binLocal

# ---- 4/4 打包 .fpk ----
Write-Host "==> 4/4 打包 .fpk ($platform) ..."
Push-Location $Root
Set-ManifestPlatform $platform
& $Fnpk build -d $FpkDir
if ($LASTEXITCODE -ne 0) { Pop-Location; Set-ManifestPlatform $OriginalPlatform; throw "fnpack 打包失败" }
$outFpk = Join-Path $Root "imgconv.fpk"
$finalFpk = Join-Path $Root "imgconv-$platform-$Version.fpk"
Move-Item -Force $outFpk $finalFpk

# 恢复原始 platform
Set-ManifestPlatform $OriginalPlatform
Pop-Location

Write-Host "完成！产物："
Write-Host "  $finalFpk"
