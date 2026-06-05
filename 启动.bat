@echo off
cd /d "%~dp0"

echo ========================================
echo   Huayitong Auto Registration System
echo ========================================
echo.

REM Check for Python
python --version >nul 2>&1
if %errorlevel% neq 0 (
    py --version >nul 2>&1
    if %errorlevel% neq 0 (
        echo [ERROR] Python not found! Please install Python 3.
        echo.
        pause
        exit /b 1
    )
    set PYTHON=py
) else (
    set PYTHON=python
)

echo Starting desktop GUI...
echo Go server will be auto-started in background.
echo.

%PYTHON% hx_gui.py

echo.
echo Application closed.
pause
