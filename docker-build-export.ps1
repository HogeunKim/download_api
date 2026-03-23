param(
    [string]$ImageName = "go-api-server",
    [string]$Tag = "latest",
    [string]$Platform = "linux/amd64",
    [string]$OutputTar = ""
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($OutputTar)) {
    $OutputTar = "$ImageName`_$Tag.tar"
}

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "docker 명령을 찾을 수 없습니다. Docker Desktop 설치/실행 및 PATH 설정을 확인하세요."
}

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Docker image build/export script" -ForegroundColor Cyan
Write-Host "Image     : $ImageName`:$Tag"
Write-Host "Platform  : $Platform"
Write-Host "Output tar: $OutputTar"
Write-Host "========================================" -ForegroundColor Cyan

docker version | Out-Null
if ($LASTEXITCODE -ne 0) {
    throw "Docker daemon 연결 실패. Docker Desktop(Linux Engine) 실행 상태를 확인하세요."
}

Write-Host "[1/2] Build Linux image..." -ForegroundColor Green
docker buildx build --platform $Platform -t "$ImageName`:$Tag" --load .
if ($LASTEXITCODE -ne 0) {
    throw "docker buildx build 실패"
}

Write-Host "[2/2] Save image to tar..." -ForegroundColor Green
docker save -o "$OutputTar" "$ImageName`:$Tag"
if ($LASTEXITCODE -ne 0) {
    throw "docker save 실패"
}

if (-not (Test-Path "$OutputTar")) {
    throw "출력 tar 파일이 생성되지 않았습니다: $OutputTar"
}

Write-Host ""
Write-Host "Done." -ForegroundColor Yellow
Write-Host "Created: $OutputTar"
Write-Host "Ubuntu import command:"
Write-Host "  docker load -i $OutputTar"
