@echo off
chcp 65001 >nul
title NapCat QQ
set "NAPCAT_DIR=%~dp0"
cd /d "%NAPCAT_DIR%"
echo Starting NapCat...
echo First launch still requires QQ login or QR scan.
call :find_shell
if defined NAPCAT_BOOT goto run_boot
if exist "%NAPCAT_DIR%NapCatInstaller.exe" goto run_installer
goto not_found
:run_installer
echo NapCat shell was not found. Running NapCat installer first...
"%NAPCAT_DIR%NapCatInstaller.exe"
call :find_shell
if defined NAPCAT_BOOT goto run_boot
goto install_failed
:find_shell
set "NAPCAT_BOOT="
if exist "%NAPCAT_DIR%bootmain\QQ.exe" if exist "%NAPCAT_DIR%bootmain\napcat.bat" set "NAPCAT_BOOT=%NAPCAT_DIR%bootmain"
if defined NAPCAT_BOOT exit /b
for /d %%D in ("%NAPCAT_DIR%NapCat*.Shell") do (
  if exist "%%~fD\napcat.bat" if exist "%%~fD\QQ.exe" set "NAPCAT_BOOT=%%~fD"
)
if defined NAPCAT_BOOT exit /b
for /d %%D in ("%NAPCAT_DIR%NapCat*.Shell") do (
  if exist "%%~fD\bootmain\napcat.bat" set "NAPCAT_BOOT=%%~fD\bootmain"
)
exit /b
:run_boot
echo NapCat boot directory: %NAPCAT_BOOT%
cd /d "%NAPCAT_BOOT%"
call napcat.bat
goto done
:install_failed
echo NapCat installer finished, but NapCat shell was still not found.
echo Please check whether the installer was blocked or cancelled.
goto done
:not_found
echo NapCat startup file was not found.
echo Please rebuild the release package or provide NapCat.Shell.Windows.OneKey files.
:done
pause
