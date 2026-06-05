# Vendored static assets

Every JS/CSS file here is third-party; re-vendor with `make webui-vendor`
or by re-running the curl commands below. SHA-256 hashes verify integrity
across re-vendors.

| File                  | Version  | Source                                                                            |
| --------------------- | -------- | --------------------------------------------------------------------------------- |
| alpine.min.js         | 3.14.8   | `https://unpkg.com/alpinejs@3.14.8/dist/cdn.min.js`                               |
| htmx.min.js           | 2.0.4    | `https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js`                               |
| htmx-sse.min.js       | 2.2.2    | `https://unpkg.com/htmx-ext-sse@2.2.2/sse.js`                                     |
| apexcharts.min.js     | 4.3.0    | `https://cdn.jsdelivr.net/npm/apexcharts@4.3.0/dist/apexcharts.min.js`            |
| apexcharts.min.css    | 4.3.0    | `https://cdn.jsdelivr.net/npm/apexcharts@4.3.0/dist/apexcharts.css`               |
| codemirror.bundle.js  | (bundle) | `tools/codemirror/` esbuild output; bumps via `cd tools/codemirror && npm update` |
| styles.css            | (bundle) | `tools/tailwind/` Tailwind v4 output; rebuild with `make webui-css`               |
| favicon.svg           | (local)  | flow logo                                                                         |

## Re-vendor command

```bash
make webui-vendor
```

(Equivalent to running every `curl -fsSL ... -o ...` and the
`tools/codemirror/build.mjs` invocation back-to-back.)
