// One-shot esbuild invocation. Outputs an IIFE bundle named `CM` into the
// embedded /static directory. Re-run after bumping a dep in package.json.
import { build } from "esbuild";

await build({
  entryPoints: ["entry.mjs"],
  outfile: "../../internal/webui/static/codemirror.bundle.js",
  bundle: true,
  minify: true,
  sourcemap: false,
  format: "iife",
  globalName: "CM",
  target: ["es2020"],
  legalComments: "none",
}).catch((err) => {
  console.error(err);
  process.exit(1);
});
console.log("[codemirror] bundle written");
