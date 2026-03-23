param(
  [string]$CacheRoot = "$env:LOCALAPPDATA\tauri",
  [string]$ProjectRoot = "",
  [string]$NsisZipPath = "",
  [string]$ApplicationIdZipPath = "",
  [string]$NsisUtilsDllPath = "",
  [switch]$Repair
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.IO.Compression.FileSystem

if (-not $ProjectRoot) {
  $ProjectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
} else {
  $ProjectRoot = (Resolve-Path $ProjectRoot).Path
}

$nsisDir = Join-Path $CacheRoot "NSIS"
$toolsDir = Join-Path $ProjectRoot "tools"

$requiredFiles = @(
  "makensis.exe",
  "Bin/makensis.exe",
  "Stubs/lzma-x86-unicode",
  "Stubs/lzma_solid-x86-unicode",
  "Plugins/x86-unicode/additional/nsis_tauri_utils.dll",
  "Include/MUI2.nsh",
  "Include/FileFunc.nsh",
  "Include/x64.nsh",
  "Include/nsDialogs.nsh",
  "Include/WinMessages.nsh",
  "Include/Win/COM.nsh",
  "Include/Win/Propkey.nsh",
  "Include/Win/RestartManager.nsh"
)

function Write-Section {
  param([string]$Title)
  Write-Host ""
  Write-Host "== $Title ==" -ForegroundColor Cyan
}

function Resolve-FirstExistingPath {
  param([string[]]$Candidates)
  foreach ($candidate in $Candidates) {
    if (-not [string]::IsNullOrWhiteSpace($candidate) -and (Test-Path $candidate)) {
      return (Resolve-Path $candidate).Path
    }
  }
  return $null
}

function Get-MissingRequiredFiles {
  param([string]$Root)

  $missing = New-Object System.Collections.Generic.List[string]
  foreach ($relativePath in $requiredFiles) {
    if (-not (Test-Path (Join-Path $Root $relativePath))) {
      $missing.Add($relativePath)
    }
  }
  return $missing
}

function Open-ZipArchive {
  param([string]$ZipPath)
  return [System.IO.Compression.ZipFile]::OpenRead($ZipPath)
}

function Get-ZipEntryNames {
  param([string]$ZipPath)
  $archive = Open-ZipArchive -ZipPath $ZipPath
  try {
    return @($archive.Entries | ForEach-Object { $_.FullName })
  } finally {
    $archive.Dispose()
  }
}

function Test-ZipContainsAll {
  param(
    [string]$ZipPath,
    [string[]]$RelativePaths,
    [string[]]$Prefixes
  )

  $entries = Get-ZipEntryNames -ZipPath $ZipPath
  foreach ($relativePath in $RelativePaths) {
    $found = $false
    foreach ($prefix in $Prefixes) {
      $candidate = if ($prefix) {
        "$prefix/$relativePath".Replace("\", "/")
      } else {
        $relativePath.Replace("\", "/")
      }
      if ($entries -contains $candidate) {
        $found = $true
        break
      }
    }
    if (-not $found) {
      return $false
    }
  }
  return $true
}

function Find-NsisZipRoot {
  param([string]$ZipPath)

  $entries = Get-ZipEntryNames -ZipPath $ZipPath
  foreach ($prefix in @("nsis-3.11", "NSIS", "")) {
    $candidate = if ($prefix) { "$prefix/Bin/makensis.exe" } else { "Bin/makensis.exe" }
    if ($entries -contains $candidate) {
      return $prefix
    }
  }
  return $null
}

function Expand-ZipToDirectory {
  param(
    [string]$ZipPath,
    [string]$Destination
  )

  if (Test-Path $Destination) {
    Remove-Item -Recurse -Force $Destination
  }
  [System.IO.Compression.ZipFile]::ExtractToDirectory($ZipPath, $Destination)
}

function Copy-DirectoryContents {
  param(
    [string]$Source,
    [string]$Destination
  )

  if (-not (Test-Path $Destination)) {
    New-Item -ItemType Directory -Path $Destination | Out-Null
  }

  Get-ChildItem -Path $Source -Force | ForEach-Object {
    $target = Join-Path $Destination $_.Name
    if ($_.PSIsContainer) {
      Copy-Item -Path $_.FullName -Destination $target -Recurse -Force
    } else {
      Copy-Item -Path $_.FullName -Destination $target -Force
    }
  }
}

function Ensure-ParentDirectory {
  param([string]$Path)
  $parent = Split-Path -Parent $Path
  if ($parent -and -not (Test-Path $parent)) {
    New-Item -ItemType Directory -Path $parent -Force | Out-Null
  }
}

function Copy-IfExists {
  param(
    [string]$Source,
    [string]$Destination
  )

  if (Test-Path $Source) {
    Ensure-ParentDirectory -Path $Destination
    Copy-Item -Path $Source -Destination $Destination -Force
    return $true
  }
  return $false
}

$defaultNsisZip = Resolve-FirstExistingPath -Candidates @(
  $NsisZipPath,
  (Join-Path $toolsDir "nsis-3.11.zip"),
  (Join-Path $toolsDir "tauri.zip"),
  (Join-Path $toolsDir "nsis.zip"),
  (Join-Path $CacheRoot "nsis-3.11.zip"),
  (Join-Path $CacheRoot "tauri.zip")
)

$defaultApplicationIdZip = Resolve-FirstExistingPath -Candidates @(
  $ApplicationIdZipPath,
  (Join-Path $toolsDir "NSIS-ApplicationID.zip"),
  (Join-Path $CacheRoot "NSIS-ApplicationID.zip")
)

$defaultNsisUtilsDll = Resolve-FirstExistingPath -Candidates @(
  $NsisUtilsDllPath,
  (Join-Path $toolsDir "nsis_tauri_utils.dll"),
  (Join-Path $nsisDir "Plugins\x86-unicode\additional\nsis_tauri_utils.dll")
)

Write-Section "Tauri NSIS Cache"
Write-Host "ProjectRoot: $ProjectRoot"
Write-Host "CacheRoot  : $CacheRoot"
Write-Host "NSISDir    : $nsisDir"
Write-Host "Repair     : $Repair"

Write-Section "Inputs"
Write-Host "NSIS zip            : $defaultNsisZip"
Write-Host "ApplicationID zip   : $defaultApplicationIdZip"
Write-Host "nsis_tauri_utils.dll: $defaultNsisUtilsDll"

Write-Section "Current Cache Check"
if (Test-Path $nsisDir) {
  $missingBefore = @(Get-MissingRequiredFiles -Root $nsisDir)
  if ($missingBefore.Count -eq 0) {
    Write-Host "NSIS cache is complete for current Tauri 2 requirements." -ForegroundColor Green
  } else {
    Write-Host "Missing required files:" -ForegroundColor Yellow
    $missingBefore | ForEach-Object { Write-Host "  - $_" }
  }
} else {
  $missingBefore = [System.Collections.Generic.List[string]]::new()
  $requiredFiles | ForEach-Object { $missingBefore.Add($_) }
  Write-Host "NSIS cache directory does not exist yet." -ForegroundColor Yellow
}

Write-Section "Zip Inspection"
if ($defaultNsisZip) {
  $nsisZipLooksUsable = Test-ZipContainsAll -ZipPath $defaultNsisZip -RelativePaths @(
    "Bin/makensis.exe",
    "Stubs/lzma-x86-unicode",
    "Stubs/lzma_solid-x86-unicode",
    "Include/MUI2.nsh",
    "Include/FileFunc.nsh",
    "Include/x64.nsh",
    "Include/nsDialogs.nsh",
    "Include/WinMessages.nsh",
    "Include/Win/COM.nsh",
    "Include/Win/Propkey.nsh",
    "Include/Win/RestartManager.nsh"
  ) -Prefixes @("nsis-3.11", "NSIS", "")
  Write-Host "NSIS zip contains current core files: $nsisZipLooksUsable"
  if (-not $nsisZipLooksUsable) {
    Write-Host "This zip is missing at least one file that current Tauri 2 requires." -ForegroundColor Yellow
  }
} else {
  Write-Host "No NSIS zip candidate found." -ForegroundColor Yellow
}

if ($defaultApplicationIdZip) {
  $appIdEntries = Get-ZipEntryNames -ZipPath $defaultApplicationIdZip
  $hasReleaseUnicodeDll = $appIdEntries -contains "ReleaseUnicode/ApplicationID.dll"
  Write-Host "ApplicationID zip has ReleaseUnicode/ApplicationID.dll: $hasReleaseUnicodeDll"
} else {
  Write-Host "No ApplicationID zip candidate found." -ForegroundColor Yellow
}

if ($Repair) {
  Write-Section "Repair"

  if (-not $defaultNsisZip) {
    throw "Repair requested, but no NSIS zip candidate was found."
  }

  $zipRoot = Find-NsisZipRoot -ZipPath $defaultNsisZip
  if ($null -eq $zipRoot) {
    throw "Could not find a usable NSIS root inside $defaultNsisZip"
  }

  $tempExtractDir = Join-Path $env:TEMP ("tauri-nsis-cache-" + [guid]::NewGuid().ToString("N"))
  Expand-ZipToDirectory -ZipPath $defaultNsisZip -Destination $tempExtractDir

  try {
    $extractedRoot = if ([string]::IsNullOrEmpty($zipRoot)) {
      $tempExtractDir
    } else {
      Join-Path $tempExtractDir $zipRoot
    }

    if (-not (Test-Path (Join-Path $extractedRoot "Bin\makensis.exe"))) {
      throw "Extracted NSIS folder does not contain Bin\\makensis.exe: $extractedRoot"
    }

    if (Test-Path $nsisDir) {
      Remove-Item -Recurse -Force $nsisDir
    }
    New-Item -ItemType Directory -Path $nsisDir -Force | Out-Null
    Copy-DirectoryContents -Source $extractedRoot -Destination $nsisDir

    if ($defaultNsisUtilsDll) {
      $utilsTarget = Join-Path $nsisDir "Plugins\x86-unicode\additional\nsis_tauri_utils.dll"
      if (Copy-IfExists -Source $defaultNsisUtilsDll -Destination $utilsTarget) {
        Write-Host "Copied nsis_tauri_utils.dll into additional plugins directory."
      }
    }

    if ($defaultApplicationIdZip) {
      $appIdTemp = Join-Path $env:TEMP ("tauri-applicationid-" + [guid]::NewGuid().ToString("N"))
      Expand-ZipToDirectory -ZipPath $defaultApplicationIdZip -Destination $appIdTemp
      try {
        $applicationIdDll = Join-Path $appIdTemp "ReleaseUnicode\ApplicationID.dll"
        $applicationIdTarget = Join-Path $nsisDir "Plugins\x86-unicode\ApplicationID.dll"
        if (Copy-IfExists -Source $applicationIdDll -Destination $applicationIdTarget) {
          Write-Host "Copied ApplicationID.dll into NSIS plugins directory."
        }
      } finally {
        if (Test-Path $appIdTemp) {
          Remove-Item -Recurse -Force $appIdTemp
        }
      }
    }

    $missingAfter = @(Get-MissingRequiredFiles -Root $nsisDir)
    if ($missingAfter.Count -eq 0) {
      Write-Host "Repair completed. NSIS cache now satisfies current Tauri 2 checks." -ForegroundColor Green
    } else {
      Write-Host "Repair completed, but cache is still incomplete:" -ForegroundColor Yellow
      $missingAfter | ForEach-Object { Write-Host "  - $_" }
    }
  } finally {
    if (Test-Path $tempExtractDir) {
      Remove-Item -Recurse -Force $tempExtractDir
    }
  }
}

Write-Section "Recommended Next Step"
Write-Host "Run:"
Write-Host "  powershell -ExecutionPolicy Bypass -File .\\tools\\prepare-nsis-cache.ps1"
Write-Host "or repair with:"
Write-Host "  powershell -ExecutionPolicy Bypass -File .\\tools\\prepare-nsis-cache.ps1 -Repair"
