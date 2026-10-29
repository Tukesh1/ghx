# ghx installer for Windows — downloads a prebuilt release binary.
#
#   irm https://raw.githubusercontent.com/Tukesh1/ghx/main/install.ps1 | iex
#
# Options: $env:GHX_VERSION = "v0.1.0"; $env:GHX_INSTALL_DIR = "C:\tools"

$ErrorActionPreference = "Stop"
# Windows PowerShell 5.1 redraws Invoke-WebRequest's progress bar for every packet, which makes downloads
# many times slower; it also defaults to TLS 1.0, which GitHub rejects.
$ProgressPreference = "SilentlyContinue"
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
$repo = "Tukesh1/ghx"
$version = if ($env:GHX_VERSION) { $env:GHX_VERSION } else { "latest" }
$arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { throw "ghx needs a 64-bit Windows" }
$asset = "ghx_windows_$arch.zip"
$base = if ($version -eq "latest") { "https://github.com/$repo/releases/latest/download" } else { "https://github.com/$repo/releases/download/$version" }
$dir = if ($env:GHX_INSTALL_DIR) { $env:GHX_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "Programs\ghx" }

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("ghx-" + [Guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    Write-Host "==> Downloading ghx ($version) for windows/$arch"
    Invoke-WebRequest -UseBasicParsing -TimeoutSec 60 "$base/$asset" -OutFile "$tmp\$asset"
    try {
        Invoke-WebRequest -UseBasicParsing -TimeoutSec 30 "$base/checksums.txt" -OutFile "$tmp\checksums.txt"
        $want = (Select-String -Path "$tmp\checksums.txt" -Pattern " $asset$").Line.Split(" ")[0]
        $got = (Get-FileHash "$tmp\$asset" -Algorithm SHA256).Hash.ToLower()
        if ($want -and $want -ne $got) { throw "checksum mismatch for $asset" }
        Write-Host "==> Checksum verified"
    } catch [System.Net.WebException] { Write-Warning "checksums.txt not found; skipping verification" }

    Expand-Archive "$tmp\$asset" -DestinationPath $tmp -Force
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    Move-Item -Force "$tmp\ghx.exe" "$dir\ghx.exe"

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if (($userPath -split ";") -notcontains $dir) {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$dir", "User")
        Write-Host "==> Added $dir to your PATH (restart the terminal)"
    }
    Write-Host "==> Installed to $dir\ghx.exe — run: ghx   (or ghx --demo)"
} finally {
    Remove-Item -Recurse -Force $tmp
}
