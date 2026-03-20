$ProjectName = "api-server"
$OutputDir = "bin"

if (!(Test-Path $OutputDir)) { New-Item -ItemType Directory -Path $OutputDir }

# 1. Windows 빌드
Write-Host "Building for Windows..." -ForegroundColor Green
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -o "$OutputDir/$ProjectName-windows.exe" ./cmd/server/main.go

# 2. Linux 빌드
Write-Host "Building for Linux (Ubuntu)..." -ForegroundColor Cyan
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -o "$OutputDir/$ProjectName-linux" ./cmd/server/main.go

Write-Host "`nBuild complete! Check the /bin folder." -ForegroundColor Yellow
