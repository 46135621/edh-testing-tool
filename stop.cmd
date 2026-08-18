@echo off
rem Double-click launcher that stops the Commander Power Check server.
setlocal
call "%~dp0start.ps1" -Stop
exit /b %errorlevel%
