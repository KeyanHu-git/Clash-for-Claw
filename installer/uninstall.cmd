@echo off
setlocal
set QUIET=
if /I "%~1"=="/quiet" set QUIET=-Quiet
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0uninstall.ps1" %QUIET%
exit /b %ERRORLEVEL%
