# Runs everything CI runs, locally: gofmt check, go vet, go test, a short fuzz pass.
#   ./scripts/test.ps1            # all
#   ./scripts/test.ps1 -Fuzz 0    # skip fuzzing
param(
    [int]$Fuzz = 10  # seconds per fuzz target; 0 to skip
)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
Push-Location $root
try {
    $files = gofmt -l .
    if ($files) { $files; throw 'gofmt needed' }
    go vet ./...
    if ($LASTEXITCODE) { throw 'go vet failed' }
    go test ./...
    if ($LASTEXITCODE) { throw 'go test failed' }

    if ($Fuzz -gt 0) {
        $targets = @(
            ,@('./internal/doc', 'FuzzParse')
        )
        foreach ($t in $targets) {
            go test $t[0] -run '^$' -fuzz "^$($t[1])$" -fuzztime "$($Fuzz)s"
            if ($LASTEXITCODE) { throw "fuzz $($t[1]) failed" }
        }
    }
} finally {
    Pop-Location
}
