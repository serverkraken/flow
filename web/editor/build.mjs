// build.mjs — der eine esbuild-Aufruf für Bundle und Prüfung (verify-editor.sh).
// Aufruf: node build.mjs [--out <pfad-ohne-endung>]. Ohne --out landet das
// Ergebnis als editor.min.js/.css im statischen Vendor-Verzeichnis.
import { build } from 'esbuild';

const outArg = process.argv.indexOf('--out');
const out = outArg >= 0 ? process.argv[outArg + 1] : '../../internal/adapter/webui/static/vendor/milkdown/editor.min';

await build({
  entryPoints: ['editor.mjs'],
  bundle: true,
  minify: true,
  format: 'esm',
  target: 'es2022',
  outfile: out + '.js',
  logLevel: 'warning',
  // Crepe zieht das ganze CodeMirror-Sprachverzeichnis und KaTeX statisch
  // mit; beides braucht flow nicht — die Sprachen kommen aus editor.mjs,
  // Formeln gibt es nicht. Die Stubs halten das Bundle klein.
  alias: {
    '@codemirror/language-data': './stub-language-data.mjs',
    katex: './stub-katex.mjs',
  },
});
