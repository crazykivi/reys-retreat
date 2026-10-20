@echo off
setlocal enabledelayedexpansion

for /f %%i in ('git describe --tags --abbrev^=0 HEAD^ 2^>nul') do set PREV=%%i
if "%PREV%"=="" (
    echo No previous tag found
    exit /b 1
)

echo ## Changes since %PREV%
echo.
echo ### Added
git diff --diff-filter=A --name-only %PREV% HEAD -- mods/ | findstr ".pw.toml"
echo.
echo ### Removed
git diff --diff-filter=D --name-only %PREV% HEAD -- mods/ | findstr ".pw.toml"
echo.
echo ### Updated (new filenames)
git diff --diff-filter=M %PREV% HEAD -- mods/ | findstr /r "^+filename"