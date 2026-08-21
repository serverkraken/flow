# web/editor — der Wissens-Editor (Milkdown)

Quelle des Editor-Bundles. `make editor` baut daraus
`internal/adapter/webui/static/vendor/milkdown/editor.min.js`, das **committet**
wird — wie `app.css`. Zur Laufzeit braucht der Server kein Node; Node ist nur
Build-Voraussetzung für alle, die den Editor ändern.

- `editor.mjs` — bindet Milkdown an `<textarea id="editor-body">` und die
  htmx-Vorschau.
- `flow-syntax.mjs` — flows Syntax als Milkdown-Plugins: `[[ziel|anzeige]]`,
  `![[slug]]`, `> [!NOTE]`, Frontmatter, plus Input-Rules fürs Tippen.

`make verify-editor` schlägt fehl, wenn das committete Bundle nicht zur Quelle
passt. Abhängigkeiten sind exakt gepinnt; `npm ci` nutzt die Lock-Datei.

Entstanden aus dem Spike `docs/superpowers/spikes/2026-08-21-milkdown/`, der
die Frage beantwortet hat, ob Milkdown flows Markdown verlustfrei zurückschreibt.
