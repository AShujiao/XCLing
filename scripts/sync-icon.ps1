# 从 logo/ 目录的多尺寸 PNG 生成 build/windows/icon.ico。
# logo/ 提供 16/32/64/128/256/512/1024 现成切图；ICO 需要的 48 等缺失尺寸从 256.png 缩放补齐。
param(
    [string]$LogoDir = "logo"
)

$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Drawing

$root = Split-Path -Parent $PSScriptRoot
$logoPath = Join-Path $root $LogoDir
if (-not (Test-Path -LiteralPath $logoPath)) {
    throw "Logo directory not found: $logoPath"
}

$iconSizes = @(16, 32, 48, 64, 128, 256)

function Get-ScaledPngBytes {
    param(
        [System.Drawing.Image]$Source,
        [int]$Size
    )
    $bitmap = New-Object System.Drawing.Bitmap $Size, $Size, ([System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
    $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
    $stream = New-Object System.IO.MemoryStream
    try {
        $graphics.CompositingQuality = [System.Drawing.Drawing2D.CompositingQuality]::HighQuality
        $graphics.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
        $graphics.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::HighQuality
        $graphics.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
        $graphics.DrawImage($Source, 0, 0, $Size, $Size)
        $bitmap.Save($stream, [System.Drawing.Imaging.ImageFormat]::Png)
        return ,$stream.ToArray()
    } finally {
        $stream.Dispose()
        $graphics.Dispose()
        $bitmap.Dispose()
    }
}

$fallbackSource = $null
try {
    $iconImages = @()
    foreach ($size in $iconSizes) {
        $pngFile = Join-Path $logoPath "$size.png"
        $exactBytes = $null
        if (Test-Path -LiteralPath $pngFile) {
            $bytes = [System.IO.File]::ReadAllBytes($pngFile)
            $checkStream = New-Object System.IO.MemoryStream (,$bytes)
            $checkImage = [System.Drawing.Image]::FromStream($checkStream)
            try {
                if ($checkImage.Width -eq $size -and $checkImage.Height -eq $size) {
                    $exactBytes = $bytes
                }
            } finally {
                $checkImage.Dispose()
                $checkStream.Dispose()
            }
        }
        if ($null -ne $exactBytes) {
            $iconImages += ,$exactBytes
        } else {
            if ($null -eq $fallbackSource) {
                $fallbackPath = Join-Path $logoPath "256.png"
                if (-not (Test-Path -LiteralPath $fallbackPath)) {
                    throw "Logo file not found: $fallbackPath (needed to scale missing size $size)"
                }
                $fallbackSource = [System.Drawing.Image]::FromFile($fallbackPath)
            }
            $iconImages += ,(Get-ScaledPngBytes -Source $fallbackSource -Size $size)
        }
    }

    $icoPath = Join-Path $root "build/windows/icon.ico"
    $icoDirectory = Split-Path -Parent $icoPath
    if (-not (Test-Path -LiteralPath $icoDirectory)) {
        New-Item -ItemType Directory -Force -Path $icoDirectory | Out-Null
    }
    $icoStream = [System.IO.File]::Open($icoPath, [System.IO.FileMode]::Create)
    $writer = New-Object System.IO.BinaryWriter $icoStream
    try {
        $writer.Write([uint16]0)
        $writer.Write([uint16]1)
        $writer.Write([uint16]$iconSizes.Count)
        $offset = 6 + (16 * $iconSizes.Count)
        for ($i = 0; $i -lt $iconSizes.Count; $i++) {
            $encodedSize = if ($iconSizes[$i] -eq 256) { 0 } else { $iconSizes[$i] }
            $writer.Write([byte]$encodedSize)
            $writer.Write([byte]$encodedSize)
            $writer.Write([byte]0)
            $writer.Write([byte]0)
            $writer.Write([uint16]1)
            $writer.Write([uint16]32)
            $writer.Write([uint32]$iconImages[$i].Length)
            $writer.Write([uint32]$offset)
            $offset += $iconImages[$i].Length
        }
        foreach ($iconImage in $iconImages) {
            $writer.Write([byte[]]$iconImage)
        }
    } finally {
        $writer.Dispose()
        $icoStream.Dispose()
    }
} finally {
    if ($null -ne $fallbackSource) { $fallbackSource.Dispose() }
}

Write-Host "Synced $LogoDir/*.png -> build/windows/icon.ico ($($iconSizes.Count) sizes: $($iconSizes -join ', '))"
