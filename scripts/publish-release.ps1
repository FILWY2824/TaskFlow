param(
  [ValidateSet("all", "windows", "android", "android-debug", "android-release")]
  [string] $Platform = "all",
  [string] $Version = "1.3.0"
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$releaseRoot = Join-Path $repoRoot "releases"
$windowsRelease = Join-Path $releaseRoot "windows"
$androidRelease = Join-Path $releaseRoot "android"

function Ensure-Dir([string] $Path) {
  if (-not (Test-Path -LiteralPath $Path)) {
    New-Item -ItemType Directory -Force -Path $Path | Out-Null
  }
}

function Copy-Required([string] $Source, [string] $Destination) {
  if (-not (Test-Path -LiteralPath $Source)) {
    throw "Missing build artifact: $Source"
  }
  Ensure-Dir (Split-Path -Parent $Destination)
  Copy-Item -LiteralPath $Source -Destination $Destination -Force
  $item = Get-Item -LiteralPath $Destination
  Write-Host "Published $($item.FullName) ($($item.Length) bytes)"
}

function Publish-Windows {
  $installerName = "TaskFlow_${Version}_x64-setup.exe"
  $installer = Join-Path $repoRoot "windows/src-tauri/target/release/bundle/nsis/$installerName"
  Copy-Required $installer (Join-Path $windowsRelease $installerName)

  $exe = Join-Path $repoRoot "windows/src-tauri/target/release/taskflow-tauri.exe"
  if (Test-Path -LiteralPath $exe) {
    Copy-Required $exe (Join-Path $windowsRelease "TaskFlow-debug.exe")
  }
}

function Publish-AndroidDebug {
  $apk = Join-Path $repoRoot "android/app/build/outputs/apk/debug/TaskFlow-debug.apk"
  Copy-Required $apk (Join-Path $androidRelease "TaskFlow-debug.apk")
}

function Publish-AndroidRelease {
  $apk = Join-Path $repoRoot "android/app/build/outputs/apk/release/TaskFlow-release-unsigned.apk"
  Copy-Required $apk (Join-Path $androidRelease "TaskFlow-release-unsigned.apk")
}

Ensure-Dir $windowsRelease
Ensure-Dir $androidRelease

switch ($Platform) {
  "all" {
    Publish-Windows
    Publish-AndroidDebug
    Publish-AndroidRelease
  }
  "windows" { Publish-Windows }
  "android" {
    Publish-AndroidDebug
    Publish-AndroidRelease
  }
  "android-debug" { Publish-AndroidDebug }
  "android-release" { Publish-AndroidRelease }
}
