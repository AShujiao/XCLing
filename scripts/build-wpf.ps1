
# 构建 WPF 原生壳 + xcling-core sidecar 的发布产物。
#
# 输出：
#   build/bin/wpf-win10/  Win10/11 部署包（主 Go 工具链 sidecar）
#   build/bin/wpf-win7/   Win7 SP1 部署包（Go 1.20.14 + go.win7.mod sidecar）
#
# 目标机要求：
#   - 两者都需要 .NET Framework 4.8（Win10/11 系统内置；Win7 SP1 需一次性安装官方离线包）
#   - Win7 SP1 另需 SHA-2 支持更新（KB4490628、KB4474419）
#   - 不需要 WebView2 运行时

param(
    [string]$GoRoot = "C:\tmp\policyguard-go1.20.14\go",
    [switch]$SkipWin7
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$appName = "星陈守护"
# 版本号唯一来源：仓库根 VERSION 文件（WPF csproj 与 setup.iss 各自直接读取，Go 侧经 ldflags 注入）。
$version = (Get-Content -Raw (Join-Path $root "VERSION")).Trim()
$goLdflags = "-H windowsgui -X XCLing/internal/model.AppVersion=$version"

Push-Location $root
try {
    Write-Host "Building version $version"
    # 0) 自动从 logo/ 目录的多尺寸 PNG 生成 icon.ico（如果 logo 更新了）
    $iconIco = "build\windows\icon.ico"
    $logoFiles = Get-ChildItem "logo\*.png" -ErrorAction SilentlyContinue
    if ($logoFiles) {
        $newestLogo = ($logoFiles | Measure-Object -Property LastWriteTime -Maximum).Maximum
        if ((-not (Test-Path $iconIco)) -or ($newestLogo -gt (Get-Item $iconIco).LastWriteTime)) {
            & (Join-Path $PSScriptRoot "sync-icon.ps1")
        }
    }

    # 1) WPF 壳（net48，一份二进制同时用于 Win7 与 Win10/11 包）
    dotnet build wpf\XCLing.Wpf -c Release
    if ($LASTEXITCODE -ne 0) { throw "WPF build failed" }

    # 2) sidecar：Win10/11（主工具链 + go.mod）
    $env:GOFLAGS = ""
    go build -buildvcs=false -ldflags $goLdflags -o build\bin\xcling-core-win10.exe .\cmd\core
    if ($LASTEXITCODE -ne 0) { throw "core (win10) build failed" }

    # 3) sidecar：Win7（Go 1.20.14 + go.win7.mod；新工具链产物无法在 Win7 运行）
    #    -tags win7 让 admin_assumption_windows_win7.go（assumeAdmin=true）生效：
    #    Win7 的 UAC 令牌探测（TokenIsElevated）对部分令牌配置会误报，故不在
    #    写 SRP 前用管理员检测拦截，改由注册表 ACL 在实际写入时把关。
    if (-not $SkipWin7) {
        $go = Join-Path $GoRoot "bin\go.exe"
        if (-not (Test-Path -LiteralPath $go)) {
            throw "Go 1.20.14 not found: $go （下载 go1.20.14.windows-amd64.zip 解压到该目录，或传 -SkipWin7 跳过）"
        }
        $env:GOPROXY = if ($env:GOPROXY) { $env:GOPROXY } else { "https://goproxy.cn,direct" }
        $env:GOFLAGS = "-modfile=go.win7.mod -mod=readonly"
        & $go build -tags win7 -buildvcs=false -ldflags $goLdflags -o build\bin\xcling-core-win7.exe .\cmd\core
        if ($LASTEXITCODE -ne 0) { throw "core (win7) build failed" }
        $env:GOFLAGS = ""
    }

    # 4) 组装部署目录
    $wpfOut = "wpf\XCLing.Wpf\bin\Release\net48"
    $targets = @(@{ Dir = "build\bin\wpf-win10"; Core = "build\bin\xcling-core-win10.exe" })
    if (-not $SkipWin7) {
        $targets += @{ Dir = "build\bin\wpf-win7"; Core = "build\bin\xcling-core-win7.exe" }
    }
    foreach ($target in $targets) {
        $dir = $target.Dir
        if (Test-Path -LiteralPath $dir) { Remove-Item -Recurse -Force -LiteralPath $dir }
        New-Item -ItemType Directory -Force -Path $dir | Out-Null

        # GUI 壳按品牌名重命名；net48 的绑定重定向配置文件必须跟随同名。
        Copy-Item "$wpfOut\XCLing.Wpf.exe" "$dir\$appName.exe"
        if (Test-Path "$wpfOut\XCLing.Wpf.exe.config") {
            Copy-Item "$wpfOut\XCLing.Wpf.exe.config" "$dir\$appName.exe.config"
        }
        Copy-Item "$wpfOut\Newtonsoft.Json.dll" "$dir\"
        Copy-Item $target.Core "$dir\xcling-core.exe"
        Copy-Item "app.config.json" "$dir\"
    }

    Write-Host ""
    Write-Host "构建完成："
    foreach ($target in $targets) { Write-Host "  $($target.Dir)" }
} finally {
    $env:GOFLAGS = ""
    Pop-Location
}
