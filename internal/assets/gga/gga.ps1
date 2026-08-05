$gitCmd = Get-Command git -ErrorAction SilentlyContinue
if (-not $gitCmd) {
    Write-Error "Git not found on PATH. Install Git for Windows to use gga from PowerShell."
    exit 1
}
$gitDir = Split-Path $gitCmd.Source
$gitRoot = Split-Path $gitDir
if ((Split-Path $gitDir -Leaf) -ieq "bin" -and (Split-Path $gitRoot -Leaf) -ieq "mingw64") {
    $gitRoot = Split-Path $gitRoot
}

$bash = @(
    (Join-Path $gitRoot "bin\bash.exe")
    (Join-Path $gitRoot "usr\bin\bash.exe")
) | Where-Object { Test-Path -LiteralPath $_ -PathType Leaf } | Select-Object -First 1
if (-not $bash) {
    Write-Error "Git Bash not found under '$gitRoot'. Reinstall Git for Windows."
    exit 1
}
& $bash -c "gga $args"
exit $LASTEXITCODE
