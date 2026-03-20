@echo off
setlocal

set SCRIPT_DIR=%~dp0
pushd "%SCRIPT_DIR%"

echo [build] Running build-all.ps1 ...
powershell -ExecutionPolicy Bypass -File ".\build-all.ps1"
set EXIT_CODE=%ERRORLEVEL%

if not "%EXIT_CODE%"=="0" (
    echo [build] Failed with exit code %EXIT_CODE%.
    popd
    exit /b %EXIT_CODE%
)

echo [build] Completed successfully.
popd
exit /b 0
