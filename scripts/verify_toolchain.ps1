$ErrorActionPreference = "Stop"
$lockPath = "tools/toolchain.lock.json"
if (-not (Test-Path $lockPath)) {
    Write-Error "Missing $lockPath"
    exit 1
}

$lock = Get-Content -Raw $lockPath | ConvertFrom-Json
$failed = 0

foreach ($art in $lock.artifacts) {
    $p = $art.path
    if (-not (Test-Path $p)) {
        Write-Host "FAIL: Missing pinned artifact $p ($($art.name))" -ForegroundColor Red
        $failed++
        continue
    }
    $actual = (Get-FileHash -Path $p -Algorithm SHA256).Hash.ToUpperInvariant()
    $expected = $art.sha256.ToUpperInvariant()
    if ($actual -ne $expected) {
        Write-Host "FAIL: SHA-256 mismatch for $p`n  expected: $expected`n  actual:   $actual" -ForegroundColor Red
        $failed++
    } else {
        Write-Host "OK: $($art.name) ($expected)" -ForegroundColor Green
    }
}

if ($failed -gt 0) {
    Write-Error "Toolchain verification failed with $failed error(s)."
    exit 1
}
Write-Host "SUCCESS: All $($lock.artifacts.Count) pinned toolchain artifacts verified."
