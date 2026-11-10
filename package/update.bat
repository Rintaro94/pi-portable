@echo off
rem Download and install the latest Pi Portable release into this folder.
rem Your login, sessions and settings are kept.
"%~dp0pi.exe" update %*
set "rc=%errorlevel%"
if not defined PI_PORTABLE_NO_PAUSE pause
exit /b %rc%
