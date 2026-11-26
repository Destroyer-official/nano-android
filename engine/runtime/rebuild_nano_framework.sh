#!/usr/bin/env bash
set -euo pipefail

SRC_DIR="engine/runtime/nano_framework/src"
BIN_DIR="engine/runtime/nano_framework/bin"
JAR_FILE="engine/runtime/nano-framework.jar"
ASM_VER="9.11.1"

rm -rf "$BIN_DIR"
mkdir -p "$BIN_DIR"

ASM_JARS=(
  "tools/libs/asm-${ASM_VER}.jar"
  "tools/libs/asm-tree-${ASM_VER}.jar"
  "tools/libs/asm-analysis-${ASM_VER}.jar"
  "tools/libs/asm-commons-${ASM_VER}.jar"
  "tools/libs/asm-util-${ASM_VER}.jar"
)

for j in "${ASM_JARS[@]}"; do
  if [[ ! -f "$j" ]]; then
    echo "Missing pinned ASM jar: $j" >&2
    exit 1
  fi
done

SOURCES_FILE="engine/runtime/sources.txt"
find "$SRC_DIR" -name "*.java" | sort > "$SOURCES_FILE"

CP="engine/runtime/android.jar:tools/libs/sqlite-jdbc-3.54.1.0.jar:$(IFS=:; echo "${ASM_JARS[*]}")"
echo "Compiling $(wc -l < "$SOURCES_FILE") Java source files against ASM ${ASM_VER}..."
javac -cp "$CP" -d "$BIN_DIR" "@$SOURCES_FILE"
rm -f "$SOURCES_FILE"

(
  cd "$BIN_DIR"
  for j in "${ASM_JARS[@]}"; do
    jar xf "../../../$j"
  done
  rm -f module-info.class
)

jar cf "$JAR_FILE" -C "$BIN_DIR" .
echo "SUCCESS! $JAR_FILE updated with ASM ${ASM_VER}."
