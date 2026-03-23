param(
    [Parameter(Mandatory = $true)]
    [string]$VersionFile
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path -LiteralPath $VersionFile)) {
    exit 1
}

$content = Get-Content -LiteralPath $VersionFile -Raw
$match = [regex]::Match($content, '(?m)^\s*var\s+Version\s*=\s*"([^"]+)"')
if (-not $match.Success) {
    exit 1
}

Write-Output $match.Groups[1].Value
