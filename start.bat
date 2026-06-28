@echo off
chcp 65001 >nul
title MCQQ Bridge
set "ROOT=%~dp0"
cd /d "%ROOT%"
echo Starting MCQQ Bridge...
if exist "%ROOT%mcqq-bridge.exe" goto init_binary
goto init_source
:init_binary
"%ROOT%mcqq-bridge.exe" init >nul
goto start_napcat
:init_source
go run "%ROOT%cmd\mcqq-bridge" init >nul
goto start_napcat
:start_napcat
if not exist "%ROOT%napcat\start-napcat.bat" goto start_bridge
echo Starting NapCat QQ...
start "NapCat QQ" /D "%ROOT%napcat" cmd.exe /k start-napcat.bat
timeout /t 3 /nobreak >nul
goto start_bridge
:start_bridge
if exist "%ROOT%mcqq-bridge.exe" goto start_binary
goto start_source
:start_binary
"%ROOT%mcqq-bridge.exe" start
goto done
:start_source
echo mcqq-bridge.exe not found, trying source mode...
go run "%ROOT%cmd\mcqq-bridge" start
goto done
:done
pause
