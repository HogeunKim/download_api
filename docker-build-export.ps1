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

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Docker image build/export script" -ForegroundColor Cyan
Write-Host "Image     : $ImageName`:$Tag"
Write-Host "Platform  : $Platform"
Write-Host "Output tar: $OutputTar"
Write-Host "========================================" -ForegroundColor Cyan

docker version | Out-Null

Write-Host "[1/2] Build Linux image..." -ForegroundColor Green
docker buildx build --platform $Platform -t "$ImageName`:$Tag" --load .

Write-Host "[2/2] Save image to tar..." -ForegroundColor Green
docker save -o "$OutputTar" "$ImageName`:$Tag"

Write-Host ""
Write-Host "Done." -ForegroundColor Yellow
Write-Host "Created: $OutputTar"
Write-Host "Ubuntu import command:"
Write-Host "  docker load -i $OutputTar"
