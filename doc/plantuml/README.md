PlantUML diagrams for cdmp-mini
````markdown
PlantUML diagrams for cdmp-mini

This folder contains sequence diagrams (PlantUML `.puml`) for key flows:

- `producer.puml`: Producer `sendWithRetry` flow (in-flight semaphore, write, retry, dead-letter)
- `consumer.puml`: Consumer fetcher -> worker -> batch -> DB -> commit flow
- `write_limiter.puml`: WriteRateLimiter middleware flow (local check, Redis override, Lua eval, fallback)

Render instructions

Requirements: Java + PlantUML jar, or Docker with `plantuml/plantuml` image, or VSCode PlantUML extension.

Quick render (PNG + SVG) using the helper script included here:

```bash
# download plantuml.jar into this directory (once)
curl -fSL -o plantuml.jar https://github.com/plantuml/plantuml/releases/latest/download/plantuml.jar
chmod +x render.sh
./render.sh
```

Notes

- If Graphviz `dot` is available in PATH, PlantUML will produce higher-quality layouts for complex diagrams. Install Graphviz (e.g., `sudo apt-get install graphviz`) and re-run `./render.sh`.
- The script outputs both PNG and SVG files next to their `.puml` sources.
- The diagrams reflect the current implementation as of 2025-10-05.

If you want me to add the generated PNG/SVG files to git and create a commit, or to produce only SVGs instead, tell me which option you prefer.

````
- If you want SVG/PNG outputs added to the repo, I can generate them and commit (requires plantuml runtime locally or in CI).
