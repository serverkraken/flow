# Changelog

## [1.1.0](https://github.com/serverkraken/flow/compare/v1.0.0...v1.1.0) (2026-05-09)


### Features

* **tui:** Note-Picker, Tab-Underline (A11y-2), b-as-global-back; split today.go/history.go (No-Monoliths) ([168ff06](https://github.com/serverkraken/flow/commit/168ff063579ba44eae7bde0f55ae193c5f8f0f02))


### Bug Fixes

* **tui:** UI/UX-Review-Findings — Glyphen, Truncation, Toast-Slot, Sidekick-Strip ([7f934ee](https://github.com/serverkraken/flow/commit/7f934ee47993dc95cd6a43257b5657aa2276d804))


### Refactoring

* **tui:** P4-Closure (canonical theme.Palette) + Help-Aggregation ([b4b64a3](https://github.com/serverkraken/flow/commit/b4b64a337a2a56794ebae2e6f8893c187be9195f))

## 1.0.0 (2026-05-08)


### Features

* **cli:** flow markdown view --raw mode für Diagnostics + Pager ([6f5c31c](https://github.com/serverkraken/flow/commit/6f5c31c257f4eaaf819fc125fb2b5db59693ad79))
* **cli:** flow markdown view &lt;file&gt; — full-screen TUI viewer ([6a671f5](https://github.com/serverkraken/flow/commit/6a671f5875f544171de21024948a3c91fd9f447f))
* **cli:** flow worktime today TUI verb ([ad39c89](https://github.com/serverkraken/flow/commit/ad39c896d299a0312c6b1c70fe39d0cf3e8a082b))
* code-review improvements across worktime, palette, projects ([8b8f725](https://github.com/serverkraken/flow/commit/8b8f72536d29d6bf1adae2fe424579e2e834da5e))
* **components,screens:** P4b — components/theme builders + screen rollout ([814a2fb](https://github.com/serverkraken/flow/commit/814a2fbf5c042c9d87f35e0243073b7adc8bbb1b))
* **components:** P3 design-system — kit expand (chip, card, tabs, modal, variants, glyph + strings constants) ([f703a03](https://github.com/serverkraken/flow/commit/f703a03a5edb4affbca6252fe794bc7c0a60689e))
* flow v0.1 — workspace TUI sidekick ([b55d94f](https://github.com/serverkraken/flow/commit/b55d94f94b689fb01d191bb56be161e13cf12d5e))
* integrate kompendium notebook into flow as a subcommand (K1-K5) ([ccd11d7](https://github.com/serverkraken/flow/commit/ccd11d7e6cefe0700997f4ed9030b18380c58ea8))
* **kompendium:** Project always offered in browse write picker ([5e210b9](https://github.com/serverkraken/flow/commit/5e210b9fb523c27ba4f5c79bdabeee5e3682bbb1))
* **markdown:** P2 design-system — decouple markdown/theme from globals ([8f205d5](https://github.com/serverkraken/flow/commit/8f205d57914ef483f4a88712f31a9a66c0955e17))
* **theme:** P1 design-system — canonical Palette + Sem + tokens + WCAG-AA test ([e94a5c1](https://github.com/serverkraken/flow/commit/e94a5c18144f787eed34f1eaf6bf112958aa1df3))
* **tui:** F-WAVE polish — cheatsheet cmd, palette in-process dispatch, help refactor ([195acbc](https://github.com/serverkraken/flow/commit/195acbc31a71a42c6a0ade030b7057cf65f6d977))
* **worktime:** : öffnet aktions-menü, q quittet überall ([03ff855](https://github.com/serverkraken/flow/commit/03ff8552e47ace49e36e373b18bb8bd101816243))
* **worktime/history:** edit/add/delete sessions in drill view ([66bd813](https://github.com/serverkraken/flow/commit/66bd81326a1124d4aaf9d658af0eebf2ee8a9833))
* **worktime:** full lifecycle — toggle, correct start, stop-at, edit/delete sessions ([6393906](https://github.com/serverkraken/flow/commit/639390611bbb24e73ea1a5568b16b7f1dc98d676))
* **worktime:** Heute `?` Help-Overlay (F4.3 wave-B+ slice 3) ([44a0456](https://github.com/serverkraken/flow/commit/44a045628ff45b8d6c8347ae053ebc2c10afc03c))
* **worktime:** Heute Kompendium edit + detach (F4.3 wave-B+ slice 2) ([5d85451](https://github.com/serverkraken/flow/commit/5d8545143074f0db44f9964d1465b6bb719bb65f))
* **worktime:** Heute Kompendium-attach trio (F4.3 wave-B+ slice 1) ([da44eae](https://github.com/serverkraken/flow/commit/da44eae53702b42b12b54ff735be598c6b06c5b7))
* **worktime:** UI/UX review batch 1 — small visual tweaks ([968776c](https://github.com/serverkraken/flow/commit/968776c0c0a12e0352831b03614106da042ea8e3))
* **worktime:** UI/UX review batch 2 — stats & projections ([3c5272d](https://github.com/serverkraken/flow/commit/3c5272d404ce51fe89f98c97d2165ea33e76877f))
* **worktime:** UI/UX review batch 3 — form/input UX ([24d7705](https://github.com/serverkraken/flow/commit/24d7705daa37136b18a947f0635ae34074d599c3))
* **worktime:** UI/UX review batch 4 — toast & key drift guard ([d4c24df](https://github.com/serverkraken/flow/commit/d4c24df8ef1fda6ad342aed37148a51ae1a2b247))
* **worktime:** UI/UX review batch 5 — pomodoro, tagclock, ical ([ef69e5c](https://github.com/serverkraken/flow/commit/ef69e5cbecacd743bbd1603ce7d55e490037d813))
* **worktime:** UI/UX review batch 6 — month grid, filters, polish ([2e7387e](https://github.com/serverkraken/flow/commit/2e7387e8a2a0cf1db1bce85027f1daccc925619b))
* **worktime:** vollständiger Go-Lifecycle, 3 Sub-Views mit live Updates ([e90e26d](https://github.com/serverkraken/flow/commit/e90e26def4681f75bdf0948794a5b6274ce5a2f4))


### Bug Fixes

* **adapter,kompendium:** atomic-write parent-dir fsync, kompendium UTC→Local, dayoff legacy fallback, TSV tab-escape ([9d515f1](https://github.com/serverkraken/flow/commit/9d515f1c36f2e0ad03750e14df6d37e5cfd7805e))
* **adapter,kompendium:** security/data-loss findings from batch review ([50efcfb](https://github.com/serverkraken/flow/commit/50efcfb739c4135625d3e4bf8a96bfc20c0b4e40))
* **adapter,kompendium:** tmuxbridge HasSession via list, editor shell-quoting, scanner buffer, kompendium stderr/sync verb ([f5a783c](https://github.com/serverkraken/flow/commit/f5a783cd80b44230fe4c59a9a5681f39d16dd4b4))
* **adapter:** lock timeout, TOCTOU, cache fallback, error wraps ([4f897ad](https://github.com/serverkraken/flow/commit/4f897ad19de6e855a7c7d2a1ab6ffdf73db8fb5f))
* **adapters:** atomic file writes via temp+fsync+rename ([3b4375e](https://github.com/serverkraken/flow/commit/3b4375e0da6b53851cc8829eb2e2c239c683c536))
* **ci:** release-please config-file ohne führenden Punkt ([68f8ee2](https://github.com/serverkraken/flow/commit/68f8ee218a76e89dd0eb2ce4886be8b28a4b1a35))
* **ci:** release-please mit User-PAT statt GITHUB_TOKEN ([a57b16d](https://github.com/serverkraken/flow/commit/a57b16d191c9b056d92a1652cfd3cae9ba686360))
* **cli:** extract preflightSidekick to keep tests TTY-safe ([5310e71](https://github.com/serverkraken/flow/commit/5310e71abe3728f6a07d02b67af4176e3f3a9b29))
* **cli:** idempotent start, refresh wiring, signal-aware TUI ([35c5f4c](https://github.com/serverkraken/flow/commit/35c5f4c8011ed9d139cd45e85c70546c6b61aac7))
* close TOCTOU windows in writer paths ([6f843da](https://github.com/serverkraken/flow/commit/6f843daf66a0327fd1228663c1c1cbe864399b5d))
* **domain,kompendium,usecase,cli:** rounding, ICS UID, snapshot/sanitize/reindex/tarsnapshot/doctor/brief ([fe10eab](https://github.com/serverkraken/flow/commit/fe10eab81f26d0e17c96dfd03883ab7398a4b1b1))
* **domain,usecase,cli:** saldo, target overrides, status rounding ([b6c6a86](https://github.com/serverkraken/flow/commit/b6c6a8687306d96a0c0fb189b3ed962b26350824))
* **domain:** ParseHM range, Stats avg/min over days-with-sessions, location injection, SplitAtMidnight rejects inverted ([d0f1e71](https://github.com/serverkraken/flow/commit/d0f1e7132854ad437ea33d8934848d095176dbad))
* idempotency, conventions, and tagger cross-month edge ([98f8ff1](https://github.com/serverkraken/flow/commit/98f8ff1d0062fbc66ebc70f6b4d8f56c91a42c3a))
* **kompendium:** browse buildWriteCmd prefix mit »kompendium« ([7aefa46](https://github.com/serverkraken/flow/commit/7aefa46a8900c618d298a1241d73b44b3350e153))
* **kompendium:** browse capture stderr aus tea.ExecProcess subprocess ([bc55cb3](https://github.com/serverkraken/flow/commit/bc55cb35375cbfb89a8ef200510c68608fd2bfcb))
* **kompendium:** close-during-query race, BOM/CRLF parser, detached HEAD ([4bb43a8](https://github.com/serverkraken/flow/commit/4bb43a8e390d5c25e8cfdb74bcd45b7520c7a0a7))
* **kompendium:** wikilink cache, gitsnapshot cleanup, dangling links, legacy rewrite ([59267e5](https://github.com/serverkraken/flow/commit/59267e5f4e5c0323790ace5711aa86f47084904b))
* **kompendium:** writepicker in-process im browse — kein Subprocess mehr ([25dec89](https://github.com/serverkraken/flow/commit/25dec89e14a7e86bfc507534333bb0a9a5435d51))
* **markdown:** design-review batch — OSC 8, strike, blockquote, lists, theme polish ([03d677d](https://github.com/serverkraken/flow/commit/03d677d33b620a064b0582e91373d3e1a3476c8c))
* **markdown:** GFM-Tabellen wrappen statt rechts abschneiden ([bc063d8](https://github.com/serverkraken/flow/commit/bc063d823cbb9c98974a9c8d1ffb209b702f808f))
* **markdown:** re-apply cell SGR nach jedem internen [0m in Wrap-Zellen ([409c17d](https://github.com/serverkraken/flow/commit/409c17d26b6ae856f83b090dc43d7df4ee5ab78c))
* **nav:** b → zurück zur Palette; Start immer auf Palette ([e510eb4](https://github.com/serverkraken/flow/commit/e510eb47349abd979e4db34265833e773f4b287a))
* **palette:** leere Icon-Spalte (führender Tab) korrekt parsen und rendern ([50855b0](https://github.com/serverkraken/flow/commit/50855b09c4e005613f847f784ab85c0c12719ea9))
* parse, doc, recovery-policy, fsprojects diagnostics ([19fad6b](https://github.com/serverkraken/flow/commit/19fad6bf60ff4a01a85cd91a0d414a74fde3bba5))
* **projects:** Git-Repos per .git-Erkennung statt alle Unterordner ([7ee8ed8](https://github.com/serverkraken/flow/commit/7ee8ed859ff61bf8dfdbc40cfd5b66113eddf2c9))
* raw-Split auf sc.Text(), TrimSpace nur für Leerzeilen-Check. ([50855b0](https://github.com/serverkraken/flow/commit/50855b09c4e005613f847f784ab85c0c12719ea9))
* **toast:** atomic nextToastID so parallel tests don't race ([366af07](https://github.com/serverkraken/flow/commit/366af07558e77a549c89b46c99b9ac365b19b7e5))
* **tui/palette,tui/heute:** async I/O in cmds, error toasts, dialog refresh suppression ([7a528a2](https://github.com/serverkraken/flow/commit/7a528a2ba2175b918a722e22cd2b882e3af1b53c))
* **tui:** help-key passthrough, palette filter blur on esc, frei kindIdx, simple-input tab ([97db7fd](https://github.com/serverkraken/flow/commit/97db7fd29c3fdc6c46ee7d05506564b6afc271e7))
* **tui:** palette esc no-op, worktime persistence, toast id, dialog gates ([ec0845f](https://github.com/serverkraken/flow/commit/ec0845fb4317b929bf67165e75e5b5d170683cdf))
* **tui:** UI bugs found in review ([a6ddd3c](https://github.com/serverkraken/flow/commit/a6ddd3c9a8134e56018aece9413a841001476511))
* **usecase,cli,tui:** lock discipline, signal handling, dialog UX, sidekick key forwarding ([4baaa25](https://github.com/serverkraken/flow/commit/4baaa25f128c68ef8ad5e4114f4a41e504241679))
* **usecase:** surface swallowed errors, fix Delete silent no-op ([62dd6d9](https://github.com/serverkraken/flow/commit/62dd6d9db3e4d6ddcab396509dc38537b31a20b6))
* **worktime:** "doubled overview" in narrow tmux panes ([f811656](https://github.com/serverkraken/flow/commit/f8116566ee8880798861a82aabb115eff4046148))
* **worktime/history:** drill-edit Stop akzeptiert past day + uppercase Duration ([f3e6e50](https://github.com/serverkraken/flow/commit/f3e6e504a37450237fb7e88eab7cbd1ffc8c83f8))
* **worktime:** tmux refresh-client -S after start/stop/toggle ([bff7211](https://github.com/serverkraken/flow/commit/bff7211e56b3644ba08cc41c46bd528ff5b0faa6))
* **worktime:** wrap chip lines and footers at terminal width ([4597fd2](https://github.com/serverkraken/flow/commit/4597fd27ed4275ddf0917607ea4f8f40abbb7fd0))


### Refactoring

* hexagonal architecture (F0-F5) ([664dc75](https://github.com/serverkraken/flow/commit/664dc75791d911eb12666dad1a8e60e36d093fc2))
* **kompendium:** P4a — migrate screens off markdown/theme shims, drop the shims ([63d843c](https://github.com/serverkraken/flow/commit/63d843c84cb3b20c639fa52a184090f98535bff5))
* **screens:** P4c — inline NewStyle rollout + kompendium modal lift ([818b18e](https://github.com/serverkraken/flow/commit/818b18ed5dea7a9e8543c6c2db7efd5993ed460d))


### Dokumentation

* design-system audit + .DS_Store ignore ([2930882](https://github.com/serverkraken/flow/commit/29308821856fc2e92b67b1a6030e3741c2fc3fb1))
* **readme:** konkrete install-anleitung für macOS und Debian ([cf29339](https://github.com/serverkraken/flow/commit/cf29339d7f6a0122b55fd04980e9e7a1638b160a))
