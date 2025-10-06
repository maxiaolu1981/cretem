#!/usr/bin/env bash
set -euo pipefail
# Simple helper to render all .puml files in this directory to PNG and SVG
cd "$(dirname "$0")"
PLANTUML_JAR="./plantuml.jar"
if [ ! -f "$PLANTUML_JAR" ]; then
  echo "plantuml.jar not found in $(pwd)."
  echo "Download it with:"
  echo "  curl -fSL -o plantuml.jar https://github.com/plantuml/plantuml/releases/latest/download/plantuml.jar"
  exit 1
fi
CMD=(java -jar "$PLANTUML_JAR")
if command -v dot >/dev/null 2>&1; then
  echo "Found 'dot' in PATH — full layout support enabled."
else
  echo "Warning: 'dot' not found. Layouts may be simplified; install Graphviz for better results."
fi
for f in *.puml; do
  [ -f "$f" ] || continue
  echo "Rendering $f -> PNG && SVG"
  "${CMD[@]}" -tpng "$f" || echo "png render failed for $f"
  "${CMD[@]}" -tsvg "$f" || echo "svg render failed for $f"
done
for d in *.dot; do
  [ -f "$d" ] || continue
  echo "Rendering Graphviz $d -> PNG && SVG"
  if command -v dot >/dev/null 2>&1; then
    dot -Tpng -o "${d%.dot}.png" "$d" || echo "dot -> png failed for $d"
    dot -Tsvg -o "${d%.dot}.svg" "$d" || echo "dot -> svg failed for $d"
  else
    echo "dot not found: skipping Graphviz render for $d"
  fi
done
echo "Render complete. Generated files:"
ls -1 -- *.png *.svg 2>/dev/null || true
