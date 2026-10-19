# Copies an example project from testdata/examples into a scratch git
# repository with a short history and one uncommitted change, so diff
# review, History, the blast radius and the Tests panel have something to
# show. Usage: scripts/example.ps1 <name> [destination]
param(
    [string]$Name = "",
    [string]$Destination = ""
)
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$examples = Join-Path $root "testdata/examples"
$source = Join-Path $examples $Name
if (-not $Name -or -not (Test-Path $source -PathType Container)) {
    Write-Host "usage: example.ps1 <example> [destination]"
    Write-Host "examples:"
    Get-ChildItem $examples -Directory | ForEach-Object { Write-Host "  $($_.Name)" }
    exit 2
}
if (-not $Destination) {
    $Destination = Join-Path ([System.IO.Path]::GetTempPath()) ("codediver-" + [guid]::NewGuid().ToString("N").Substring(0, 8)) | Join-Path -ChildPath $Name
}
if ((Test-Path $Destination) -and (Get-ChildItem $Destination -Force | Select-Object -First 1)) {
    Write-Error "$Destination already exists and isn't empty"
    exit 1
}
New-Item -ItemType Directory -Force -Path $Destination | Out-Null
Get-ChildItem $source -Force | Where-Object { $_.Name -ne ".demo" } | Copy-Item -Destination $Destination -Recurse -Force

$demo = Join-Path $source ".demo"
$first, $second = "Initial order service", "Reject orders over 1000 items"
$commits = Join-Path $demo "commits"
if (Test-Path $commits) {
    $lines = Get-Content $commits
    $first, $second = $lines[0], $lines[1]
}

function Invoke-Git {
    & git @args
    if ($LASTEXITCODE -ne 0) { throw "git $args failed" }
}
function Save-Commit([string]$Message) {
    Invoke-Git add -A
    Invoke-Git -c "user.name=CodeDiver Example" -c "user.email=example@codediver.app" -c core.autocrlf=false commit -q -m $Message
}

Push-Location $Destination
try {
    Invoke-Git init -q
    Invoke-Git apply -R (Join-Path $demo "history.patch")
    Save-Commit $first
    Invoke-Git apply (Join-Path $demo "history.patch")
    Save-Commit $second
    Invoke-Git apply (Join-Path $demo "change.patch")
    Write-Host "Example $Name is ready in $Destination"
    Write-Host "  2 commits, plus an uncommitted change:"
    git status --short | ForEach-Object { Write-Host "    $_" }
    Write-Host "  Open it in CodeDiver:  codediver --add `"$Destination`"   (or: codediver serve --add `"$Destination`")"
} finally {
    Pop-Location
}
