param(
    [string]$InstallDir = ""
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    if ([string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
        throw "LOCALAPPDATA is not set. Pass -InstallDir with a user-writable directory."
    }
    $InstallDir = Join-Path $env:LOCALAPPDATA "Programs\twitch-live-cli"
}
$InstallDir = [IO.Path]::GetFullPath($InstallDir)

$nativeArchitecture = $env:PROCESSOR_ARCHITEW6432
if ([string]::IsNullOrWhiteSpace($nativeArchitecture)) {
    $nativeArchitecture = $env:PROCESSOR_ARCHITECTURE
}
if ([string]::IsNullOrWhiteSpace($nativeArchitecture)) {
    throw "Could not detect the Windows processor architecture."
}
switch ($nativeArchitecture.ToUpperInvariant()) {
    "AMD64" { $architecture = "amd64" }
    "ARM64" { $architecture = "arm64" }
    default { throw "Unsupported Windows architecture: $nativeArchitecture" }
}

$asset = "twitch-live-cli_windows_${architecture}.exe"
$url = "https://github.com/thomasjdelaney/TwitchLiveCLI/releases/latest/download/${asset}.tar.gz"
$temporaryDirectory = Join-Path ([IO.Path]::GetTempPath()) "twitch-live-cli-install-$([guid]::NewGuid())"
New-Item -ItemType Directory -Path $temporaryDirectory | Out-Null

try {
    $archive = Join-Path $temporaryDirectory "release.tar.gz"
    Invoke-WebRequest -Uri $url -OutFile $archive -UseBasicParsing -ErrorAction Stop

    $tar = Get-Command tar.exe -ErrorAction SilentlyContinue
    if ($null -eq $tar) {
        throw "tar.exe is required to extract the release archive. Install a recent Windows version or extract the archive manually."
    }
    & $tar.Source -xzf $archive -C $temporaryDirectory $asset
    if ($LASTEXITCODE -ne 0) {
        throw "Could not extract the release archive (tar exit code $LASTEXITCODE)."
    }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $target = Join-Path $InstallDir "twitch-live-cli.exe"
    Copy-Item -LiteralPath (Join-Path $temporaryDirectory $asset) -Destination $target -Force

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $pathEntries = @($userPath -split ";" | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    $alreadyOnPath = $false
    foreach ($entry in $pathEntries) {
        $entryPath = $entry.TrimEnd([char[]]@('\', '/'))
        $installPath = $InstallDir.TrimEnd([char[]]@('\', '/'))
        if ([string]::Equals($entryPath, $installPath, [StringComparison]::OrdinalIgnoreCase)) {
            $alreadyOnPath = $true
            break
        }
    }
    if (-not $alreadyOnPath) {
        $newUserPath = if ([string]::IsNullOrWhiteSpace($userPath)) { $InstallDir } else { "$userPath;$InstallDir" }
        [Environment]::SetEnvironmentVariable("Path", $newUserPath, "User")
    }

    Write-Host "Installed twitch-live-cli to $target"
    if (-not $alreadyOnPath) {
        Write-Host "Added $InstallDir to your user PATH. Open a new terminal to use the command."
    } else {
        Write-Host "Open a new terminal to use the command."
    }
}
finally {
    Remove-Item -LiteralPath $temporaryDirectory -Recurse -Force -ErrorAction SilentlyContinue
}
