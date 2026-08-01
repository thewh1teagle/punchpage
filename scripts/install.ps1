# PunchPage installer for Windows.
#   powershell -c "irm https://punchpage.pages.dev/install.ps1 | iex"
$ErrorActionPreference = "Stop"

$repo = "thewh1teagle/punchpage"
$arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq "Arm64") { "arm64" } else { "amd64" }
$url = "https://github.com/$repo/releases/latest/download/punch_windows_$arch.zip"

$installDir = if ($env:PUNCHPAGE_INSTALL_DIR) { $env:PUNCHPAGE_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "punchpage" }
New-Item -ItemType Directory -Force -Path $installDir | Out-Null

$zip = Join-Path $env:TEMP "punch.zip"
Write-Host "Downloading $url"
Invoke-WebRequest -Uri $url -OutFile $zip
Expand-Archive -Path $zip -DestinationPath $installDir -Force
Remove-Item $zip

Write-Host "Installed punch to $installDir\punch.exe"

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$installDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$installDir", "User")
    Write-Host "Added $installDir to your PATH (restart your terminal to pick it up)."
}
