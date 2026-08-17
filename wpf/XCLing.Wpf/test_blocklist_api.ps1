# Test if blocklist API is working.
# Usage: .\test_blocklist_api.ps1 [-CorePath <path to xcling-core.exe>]
param(
    [string]$CorePath = (Join-Path $PSScriptRoot "..\..\build\bin\xcling-core-win10.exe")
)

if (-not (Test-Path -LiteralPath $CorePath)) {
    Write-Error "Core not found: $CorePath (build it first with scripts/build-wpf.ps1)"
    exit 1
}

$proc = Start-Process -FilePath $CorePath -ArgumentList "serve","--stdio" -NoNewWindow -PassThru -RedirectStandardInput "stdin.txt" -RedirectStandardOutput "stdout.txt" -RedirectStandardError "stderr.txt"
Start-Sleep -Seconds 1
$request = @{
    jsonrpc = "2.0"
    id = 1
    method = "BlocklistService.GetBlocklistStatus"
    params = @()
} | ConvertTo-Json -Compress
Set-Content -Path "stdin.txt" -Value $request
Start-Sleep -Seconds 2
Stop-Process -Id $proc.Id -Force
Get-Content "stdout.txt"
Remove-Item "stdin.txt","stdout.txt","stderr.txt" -ErrorAction SilentlyContinue
