@echo off
setlocal
set "GGA_PS1=%~dp0gga.ps1"
powershell -NoProfile -ExecutionPolicy Bypass -File "%GGA_PS1%" %*
exit /b %ERRORLEVEL%
