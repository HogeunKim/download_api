@echo off
setlocal

REM Usage:
REM   docker_export_win.bat [VERSION] [IMAGE_NAME] [PLATFORM]
REM Example:
REM   docker_export_win.bat 1.0.1 go-api-server linux/amd64

set "SCRIPT_DIR=%~dp0"
set "PS1_PATH=%SCRIPT_DIR%docker-build-export.ps1"
set "GET_VERSION_PS1=%SCRIPT_DIR%get-app-version.ps1"
set "VERSION_FILE=%SCRIPT_DIR%cmd\server\main.go"

set "VERSION=%~1"
if "%VERSION%"=="" (
  if exist "%VERSION_FILE%" (
    for /f "usebackq delims=" %%v in (`powershell -NoProfile -ExecutionPolicy Bypass -File "%GET_VERSION_PS1%" "%VERSION_FILE%"`) do (
      set "VERSION=%%v"
    )
  )
)
if "%VERSION%"=="" (
  echo [ERROR] Failed to detect app version from "%VERSION_FILE%".
  echo         Please pass version explicitly. Example: docker_export_win.bat 1.0.1
  exit /b 1
)

set "IMAGE_NAME=%~2"
if "%IMAGE_NAME%"=="" set "IMAGE_NAME=go-api-server"

set "PLATFORM=%~3"
if "%PLATFORM%"=="" set "PLATFORM=linux/amd64"

set "OUTPUT_TAR=%IMAGE_NAME%_%VERSION%.tar"

if not exist "%PS1_PATH%" (
  echo [ERROR] Script not found: "%PS1_PATH%"
  exit /b 1
)
if not exist "%GET_VERSION_PS1%" (
  echo [ERROR] Script not found: "%GET_VERSION_PS1%"
  exit /b 1
)

echo ========================================
echo Docker export batch (Windows)
echo Image     : %IMAGE_NAME%:%VERSION%
echo AppVersion: %VERSION%
echo Platform  : %PLATFORM%
echo OutputTar : %OUTPUT_TAR%
echo ========================================

powershell -NoProfile -ExecutionPolicy Bypass -File "%PS1_PATH%" ^
  -ImageName "%IMAGE_NAME%" ^
  -Tag "%VERSION%" ^
  -Platform "%PLATFORM%" ^
  -OutputTar "%OUTPUT_TAR%" ^
  -AppVersion "%VERSION%"

if errorlevel 1 (
  echo [ERROR] Docker export failed.
  exit /b 1
)

echo [OK] Docker export completed: %OUTPUT_TAR%
endlocal
