# Build installer script
# Requires Inno Setup 6 or 7: https://jrsoftware.org/isdl.php
param(
    [string]$InnoSetupPath = "C:\Program Files (x86)\Inno Setup 7\ISCC.exe",
    [switch]$SkipWin7,
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root

try {
    # 1. Build app first (unless skipped)
    if (-not $SkipBuild) {
        Write-Host "Building application..."
        $buildArgs = @()
        if ($SkipWin7) {
            $buildArgs += "-SkipWin7"
        }
        & powershell -ExecutionPolicy Bypass -File scripts/build-wpf.ps1 @buildArgs
        if ($LASTEXITCODE -ne 0) { throw "Application build failed" }
    } else {
        Write-Host "Skipping build, using existing binaries..."
    }

    # 2. Find Inno Setup
    if (-not (Test-Path -LiteralPath $InnoSetupPath)) {
        $possiblePaths = @(
            "C:\Program Files (x86)\Inno Setup 7\ISCC.exe",
            "C:\Program Files (x86)\Inno Setup 6\ISCC.exe"
        )
        $found = $false
        foreach ($path in $possiblePaths) {
            if (Test-Path -LiteralPath $path) {
                $InnoSetupPath = $path
                $found = $true
                break
            }
        }
        if (-not $found) {
            Write-Host "Inno Setup not found. Please install Inno Setup 6 or 7 from https://jrsoftware.org/isdl.php"
            Write-Host "If installed to custom path, use -InnoSetupPath parameter to specify ISCC.exe location"
            exit 1
        }
    }

    # 3. Create LICENSE if not exists
    if (-not (Test-Path "LICENSE")) {
        $year = Get-Date -Format yyyy
        $licenseText = "Policy Guard (XCLing)`r`nCopyright (C) $year`r`nThis software is provided 'as-is', without any express or implied warranty."
        Set-Content -Path "LICENSE" -Value $licenseText -Encoding UTF8
    }

    # 4. Create output directory
    $installerDir = "build\installer"
    if (-not (Test-Path -LiteralPath $installerDir)) {
        New-Item -ItemType Directory -Force -Path $installerDir | Out-Null
    }

    # 5. Compile installer
    Write-Host "Compiling installer..."
    $innoArgs = @("setup.iss")
    if ($SkipWin7) {
        $innoArgs += "/dIncludeWin7=0"
    }
    & $InnoSetupPath @innoArgs
    if ($LASTEXITCODE -ne 0) { throw "Installer compilation failed" }

    Write-Host ""
    Write-Host "Installer built successfully!"
    Write-Host "Output directory: $(Resolve-Path $installerDir)"
    Get-ChildItem "$installerDir\*.exe" | Select-Object Name, @{Name="Size";Expression={"{0:N2} MB" -f ($_.Length/1MB)}}
} finally {
    Pop-Location
}
