# Runs the jq lessons in learn/ (DESIGN §17).
#   ./scripts/learn.ps1          # write each query's output into its lesson, from real jq if on the PATH
#   ./scripts/learn.ps1 -Check   # don't write: fail where an output block says something else
param(
    [switch]$Check
)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
Push-Location $root
try {
    if ($Check) {
        go test ./learn -count=1
    } else {
        go test ./learn -count=1 -update
        git --no-pager diff --stat -- learn
    }
    if ($LASTEXITCODE) { throw 'lessons failed' }
} finally {
    Pop-Location
}
