$ErrorActionPreference = "Stop"
$srcDir = "engine/runtime/nano_framework/src"
$binDir = "engine/runtime/nano_framework/bin"
$jarFile = "engine/runtime/nano-framework.jar"

if (Test-Path $binDir) {
    Remove-Item -Recurse -Force $binDir
}
New-Item -ItemType Directory -Path $binDir -Force | Out-Null

$files = Get-ChildItem -Path $srcDir -Filter "*.java" -Recurse | Select-Object -ExpandProperty FullName
$sourcesFile = "engine/runtime/sources.txt"
[System.IO.File]::WriteAllLines($sourcesFile, $files)

Write-Host "Compiling $($files.Count) Java source files..."
javac -cp "engine/runtime/android.jar;tools/dex-tools-v2.4/lib/asm-9.5.jar;tools/dex-tools-v2.4/lib/asm-tree-9.5.jar;tools/dex-tools-v2.4/lib/asm-analysis-9.5.jar" -d $binDir "@$sourcesFile"
$rc = $LASTEXITCODE
Remove-Item $sourcesFile -Force -ErrorAction SilentlyContinue

if ($rc -ne 0) {
    Write-Error "javac compilation failed!"
    exit 1
}

$asmJar = (Resolve-Path "tools/dex-tools-v2.4/lib/asm-9.5.jar").Path
$asmTreeJar = (Resolve-Path "tools/dex-tools-v2.4/lib/asm-tree-9.5.jar").Path
$asmAnalysisJar = (Resolve-Path "tools/dex-tools-v2.4/lib/asm-analysis-9.5.jar").Path
Write-Host "Extracting ASM from $asmJar, $asmTreeJar, and $asmAnalysisJar into $binDir..."
Push-Location $binDir
jar xf $asmJar
jar xf $asmTreeJar
jar xf $asmAnalysisJar
if (Test-Path "module-info.class") { Remove-Item "module-info.class" -Force }
Pop-Location

Write-Host "Packaging $jarFile..."
jar cf $jarFile -C $binDir .

Write-Host "SUCCESS! $jarFile updated successfully."
