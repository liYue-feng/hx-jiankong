@echo off
chcp 65001 >nul
cd /d "%~dp0"

echo ========================================
echo   Huayitong Auto Registration Assistant
echo ========================================
echo.

if not exist hx_jiankong.exe (
    echo [ERROR] hx_jiankong.exe not found!
    pause
    exit /b 1
)

echo Starting Go server...
echo Open browser to http://127.0.0.1:8088
echo.
start /min /b hx_jiankong.exe

echo Waiting for server...
ping 127.0.0.1 -n 4 >nul

echo.
echo Browser should open automatically.
echo If not, visit: http://127.0.0.1:8088
echo.
echo Close this window to stop the server.
pause
