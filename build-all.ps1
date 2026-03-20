param(
    [string]$OutputDir = "bin"
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$projectRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
Push-Location $projectRoot

try {
    New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null

    $targets = @(
        @{ GOOS = "windows"; GOARCH = "amd64"; Suffix = "windows-amd64.exe" },
        @{ GOOS = "linux";   GOARCH = "amd64"; Suffix = "linux-amd64" }
    )

    $apps = @(
        @{ Name = "go-api-server"; BuildPath = "./cmd/server" },
        @{ Name = "callback-server"; BuildPath = "./cmd/callback-server" }
    )

    foreach ($target in $targets) {
        foreach ($app in $apps) {
            $outputPath = Join-Path $OutputDir ("{0}-{1}" -f $app.Name, $target.Suffix)
            Write-Host ("[build] {0}/{1} -> {2}" -f $target.GOOS, $target.GOARCH, $outputPath)

            $env:GOOS = $target.GOOS
            $env:GOARCH = $target.GOARCH
            go build -o $outputPath $app.BuildPath
        }
    }

    Write-Host ""
    Write-Host "Build completed."
    Write-Host ("Output directory: {0}" -f (Resolve-Path $OutputDir))
}
finally {
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    Pop-Location
}
