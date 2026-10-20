@echo off
setlocal EnableExtensions DisableDelayedExpansion

REM Build isolated Standard and NVIDIA editions without modifying the source project.
REM Run this script from the actual modpack root, next to pack.toml and index.toml.

if "%~1"=="" (
    echo Usage: build.bat ^<version^>
    echo Example: build.bat 0.3.1
    exit /b 2
)

set "VERSION=%~1"
set "ROOT=%~dp0"
if "%ROOT:~-1%"=="\" set "ROOT=%ROOT:~0,-1%"
set "DIST=%ROOT%\dist"
set "WORK=%TEMP%\reys-retreat-build-%RANDOM%-%RANDOM%"
set "STD=%WORK%\standard"
set "NVIDIA=%WORK%\nvidia"

if not exist "%ROOT%\pack.toml" (
    echo ERROR: pack.toml was not found in:
    echo "%ROOT%"
    echo Run build.bat from the actual modpack project directory.
    exit /b 3
)
if not exist "%ROOT%\index.toml" (
    echo ERROR: index.toml was not found in:
    echo "%ROOT%"
    exit /b 3
)
if not exist "%ROOT%\mods" (
    echo ERROR: mods directory was not found in:
    echo "%ROOT%"
    exit /b 3
)
if not exist "%ROOT%\extra\nvidia\nvidium.pw.toml" (
    echo ERROR: extra\nvidia\nvidium.pw.toml was not found.
    exit /b 3
)

where packwiz >nul 2>nul
if errorlevel 1 (
    echo ERROR: packwiz was not found in PATH.
    echo Install Packwiz and reopen the terminal before building.
    exit /b 3
)

if not exist "%DIST%" mkdir "%DIST%"
if errorlevel 1 (
    echo ERROR: could not create output directory:
    echo "%DIST%"
    exit /b 4
)

if exist "%WORK%" (
    echo ERROR: temporary workspace already exists:
    echo "%WORK%"
    exit /b 4
)

mkdir "%STD%" 2>nul
if errorlevel 1 (
    echo ERROR: could not create Standard temporary directory:
    echo "%STD%"
    exit /b 4
)
mkdir "%NVIDIA%" 2>nul
if errorlevel 1 (
    echo ERROR: could not create NVIDIA temporary directory:
    echo "%NVIDIA%"
    exit /b 4
)

echo.
echo [1/4] Preparing isolated Standard workspace...
echo Source:      "%ROOT%"
echo Destination: "%STD%"

REM Robocopy /XD entries are absolute paths. This avoids ambiguous relative
REM exclusions and explicitly excludes the output and incoming directories.
robocopy "%ROOT%" "%STD%" /E /COPY:DAT /DCOPY:DAT /R:1 /W:1 /XJ /XD "%ROOT%\.git" "%DIST%" "%ROOT%\incoming" "%ROOT%\.idea" "%ROOT%\.vscode" /XF "*.log" "*.tmp"
set "COPY_RC=%ERRORLEVEL%"
if %COPY_RC% GEQ 8 (
    echo.
    echo ERROR: Robocopy failed with exit code %COPY_RC% while copying Standard workspace.
    goto :copy_failed
)

echo.
echo [2/4] Building Standard Edition...
if not exist "%STD%\mods" (
    echo ERROR: mods directory is missing from the Standard workspace.
    goto :failed
)
for /r "%STD%\mods" %%F in (*.pw.toml.disabled) do if exist "%%F" del /q "%%F"

pushd "%STD%"
if errorlevel 1 goto :push_failed
packwiz refresh
if errorlevel 1 goto :build_failed
packwiz modrinth export --output "%DIST%\reys-retreat-standard-%VERSION%.mrpack"
if errorlevel 1 goto :build_failed
popd

echo.
echo [3/4] Preparing isolated NVIDIA workspace...
echo Source:      "%ROOT%"
echo Destination: "%NVIDIA%"

robocopy "%ROOT%" "%NVIDIA%" /E /COPY:DAT /DCOPY:DAT /R:1 /W:1 /XJ /XD "%ROOT%\.git" "%DIST%" "%ROOT%\incoming" "%ROOT%\.idea" "%ROOT%\.vscode" /XF "*.log" "*.tmp"
set "COPY_RC=%ERRORLEVEL%"
if %COPY_RC% GEQ 8 (
    echo.
    echo ERROR: Robocopy failed with exit code %COPY_RC% while copying NVIDIA workspace.
    goto :copy_failed
)

if not exist "%NVIDIA%\mods" (
    echo ERROR: mods directory is missing from the NVIDIA workspace.
    goto :failed
)
if not exist "%NVIDIA%\extra\nvidia\nvidium.pw.toml" (
    echo ERROR: extra\nvidia\nvidium.pw.toml is missing from the NVIDIA workspace.
    goto :failed
)

copy /Y "%NVIDIA%\extra\nvidia\nvidium.pw.toml" "%NVIDIA%\mods\nvidium.pw.toml" >nul
if errorlevel 1 (
    echo ERROR: could not add Nvidium metadata to the NVIDIA workspace.
    goto :failed
)

for /r "%NVIDIA%\mods" %%F in (*.pw.toml.disabled) do if exist "%%F" del /q "%%F"

echo.
echo [4/4] Building NVIDIA Edition...
pushd "%NVIDIA%"
if errorlevel 1 goto :push_failed
packwiz refresh
if errorlevel 1 goto :build_failed
packwiz modrinth export --output "%DIST%\reys-retreat-nvidia-%VERSION%.mrpack"
if errorlevel 1 goto :build_failed
popd

rmdir /S /Q "%WORK%" >nul 2>nul
echo.
echo SUCCESS. Built files:
dir /b "%DIST%\reys-retreat-*-%VERSION%.mrpack"
exit /b 0

:copy_failed
echo.
echo ERROR: copying project to the temporary build workspace failed.
echo Source: "%ROOT%"
echo Temporary workspace: "%WORK%"
echo Review the Robocopy output above. The workspace was kept for inspection.
goto :failed

:push_failed
echo ERROR: could not enter the temporary build workspace.
goto :failed

:build_failed
echo ERROR: Packwiz failed. The temporary workspace was kept for inspection:
echo "%WORK%"
if /I "%CD%"=="%STD%" popd
if /I "%CD%"=="%NVIDIA%" popd
exit /b 1

:failed
echo.
echo Build failed. Temporary workspace, if created, is at:
echo "%WORK%"
exit /b 1
