@echo off
rem Double-click launcher for the Commander Power Check server.
rem Opens the app in your default browser; no terminal stays behind on success.
setlocal
call "%~dp0start.ps1"
exit /b %errorlevel%
