$ErrorActionPreference = "Stop"
$srcDir = "engine/runtime/nano_framework/src"
$binDir = "engine/runtime/nano_framework/bin"
$jarFile = "engine/runtime/nano-framework.jar"

if (Test-Path $binDir) {
    Remove-Item -Recurse -Force $binDir
}
New-Item -ItemType Directory -Path $binDir -Force | Out-Null

$asmVer = "9.11.1"
$asmJars = @(
    "tools/libs/asm-$asmVer.jar",
    "tools/libs/asm-tree-$asmVer.jar",
    "tools/libs/asm-analysis-$asmVer.jar",
    "tools/libs/asm-commons-$asmVer.jar",
    "tools/libs/asm-util-$asmVer.jar"
)
foreach ($j in $asmJars) {
    if (-not (Test-Path $j)) {
        Write-Error "Missing pinned ASM jar: $j"
        exit 1
    }
}

$files = Get-ChildItem -Path $srcDir -Filter "*.java" -Recurse | Select-Object -ExpandProperty FullName
$sourcesFile = "engine/runtime/sources.txt"
[System.IO.File]::WriteAllLines($sourcesFile, $files)

$cp = (@("engine/runtime/android.jar", "tools/libs/sqlite-jdbc-3.54.1.0.jar") + $asmJars) -join ";"
Write-Host "Compiling $($files.Count) Java source files against ASM $asmVer + sqlite-jdbc..."
javac -cp $cp -d $binDir "@$sourcesFile"
$rc = $LASTEXITCODE
Remove-Item $sourcesFile -Force -ErrorAction SilentlyContinue

if ($rc -ne 0) {
    Write-Error "javac compilation failed!"
    exit 1
}

Write-Host "Extracting ASM $asmVer into $binDir..."
$resolvedJars = $asmJars | ForEach-Object { (Resolve-Path $_).Path }
Push-Location $binDir
foreach ($rj in $resolvedJars) {
    jar xf $rj
}
if (Test-Path "module-info.class") { Remove-Item "module-info.class" -Force }
Pop-Location

Write-Host "Packaging $jarFile..."
jar cf $jarFile -C $binDir .
if ($LASTEXITCODE -ne 0) {
    Write-Error "jar packaging failed!"
    exit 1
}

Write-Host "SUCCESS! $jarFile updated with ASM $asmVer."