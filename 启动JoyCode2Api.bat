@echo off
chcp 65001 >nul
title JoyCode2Api Daemon

cd /d "%~dp0"

rem ---- already healthy? then nothing to do ----
curl.exe -s -o NUL -m 2 http://127.0.0.1:34891/health 2>nul
if %errorlevel%==0 exit /b 0

rem ---- daemon may be half-alive (supervisor up, serve dead): stop it cleanly ----
.\JoyCode2Api.exe daemon stop >nul 2>&1
timeout /t 2 /nobreak >nul

.\JoyCode2Api.exe daemon start --port 34891 --skip-validation

rem ---- wait up to 60s for /health ----
set /a tries=0
:wait_health
ping -n 4 127.0.0.1 >nul
curl.exe -s -o NUL -m 2 http://127.0.0.1:34891/health 2>nul
if %errorlevel%==0 exit /b 0
set /a tries+=1
if %tries% lss 20 goto wait_health

color 4F
echo.
echo ========================================================
echo  FAILED: service did not become healthy in 60 seconds.
echo  Possible causes:
echo   1. Antivirus killed the process
echo   2. Another session is rebuilding / stopping the service
echo  Check logs:
echo    %USERPROFILE%\.joycode-proxy\logs\daemon.log
echo    %USERPROFILE%\.joycode-proxy\logs\serve.log
echo ========================================================
echo.
pause
