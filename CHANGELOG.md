# Changelog

## [1.5.0](https://github.com/serverkraken/flow/compare/v1.4.3...v1.5.0) (2026-05-30)


### Features

* **tui:** UX-review cleanup — all 25 findings, 10 phases ([#43](https://github.com/serverkraken/flow/issues/43)) ([f008fbb](https://github.com/serverkraken/flow/commit/f008fbbb0c978755f669f8d87ee83e265caaa883))


### Bug Fixes

* **tui:** resolve round-2 design-review findings (German UI, color se… ([#41](https://github.com/serverkraken/flow/issues/41)) ([79c9bc5](https://github.com/serverkraken/flow/commit/79c9bc556c22021eb8f735390e401bec1d3f11dc))
* **worktime:** Phase-10 follow-up — sub-tab title + complete footer hints ([#44](https://github.com/serverkraken/flow/issues/44)) ([65dc4d1](https://github.com/serverkraken/flow/commit/65dc4d1d30f2e28c95c06285368cb571dc1c989f))
* **worktime:** restore tab strip in titlebox title ([#45](https://github.com/serverkraken/flow/issues/45)) ([b0ba60e](https://github.com/serverkraken/flow/commit/b0ba60e44fe9cf8173685e4447e2aff0b603c580))

## [1.4.3](https://github.com/serverkraken/flow/compare/v1.4.2...v1.4.3) (2026-05-30)


### Bug Fixes

* **tui:** design-review follow-ups (progress bar, picker chrome, all-dim footer) ([#38](https://github.com/serverkraken/flow/issues/38)) ([91c7772](https://github.com/serverkraken/flow/commit/91c7772463f8fc3d26f4e62667d374f35d16e0e7))
* **tui:** resolve all 16 UI/UX design-review findings ([#40](https://github.com/serverkraken/flow/issues/40)) ([5e0cbb6](https://github.com/serverkraken/flow/commit/5e0cbb622b8f9a330b83234ef62e11e7c83b8244))

## [1.4.2](https://github.com/serverkraken/flow/compare/v1.4.1...v1.4.2) (2026-05-29)


### Bug Fixes

* **tui:** resolve design-review findings (heatmap, glyphs, writepicker, callouts) ([#36](https://github.com/serverkraken/flow/issues/36)) ([8e9a87f](https://github.com/serverkraken/flow/commit/8e9a87f0653df6109d21e03004eded80952933e4))

## [1.4.1](https://github.com/serverkraken/flow/compare/v1.4.0...v1.4.1) (2026-05-29)


### Bug Fixes

* **tui:** scale worktime tabs to terminal height ([#34](https://github.com/serverkraken/flow/issues/34)) ([04d02ba](https://github.com/serverkraken/flow/commit/04d02ba1f93bbaf1ea69dd30644bcea683a6fc3a))

## [1.4.0](https://github.com/serverkraken/flow/compare/v1.3.0...v1.4.0) (2026-05-29)


### Features

* **tui:** german UI strings, design-review fixes, session nachbuchen ([#32](https://github.com/serverkraken/flow/issues/32)) ([8096c30](https://github.com/serverkraken/flow/commit/8096c30cae09d0f3981e245f9cabd1bd473a5a25))

## [1.3.0](https://github.com/serverkraken/flow/compare/v1.2.2...v1.3.0) (2026-05-22)


### Features

* **tui:** migrate to lipgloss/bubbletea/bubbles v2 ([#25](https://github.com/serverkraken/flow/issues/25)) ([0a85d3b](https://github.com/serverkraken/flow/commit/0a85d3b497a918412209f22c9bb9cb5da95097de))

## [1.2.2](https://github.com/serverkraken/flow/compare/v1.2.1...v1.2.2) (2026-05-22)


### Bug Fixes

* **deps:** update module github.com/charmbracelet/x/ansi to v0.11.7 ([#7](https://github.com/serverkraken/flow/issues/7)) ([ad3aa83](https://github.com/serverkraken/flow/commit/ad3aa83c6acee68813c7d7b7c43ef73444f445fd))

## [1.2.1](https://github.com/serverkraken/flow/compare/v1.2.0...v1.2.1) (2026-05-18)


### Bug Fixes

* **worktime/frei:** tighten add-dialog footer hints ([6923b88](https://github.com/serverkraken/flow/commit/6923b88a078ad6fe18fe8eabcec3c6f5dcabf16b))
* **worktime/heute:** help keys use Strong (Fg+Bold), not Highlight ([f259f08](https://github.com/serverkraken/flow/commit/f259f08bd5e2b55511ced5c3741902cf83f09c7c))
* **worktime/heute:** running headline uses Cyan, drop bold from total ([8f8ddf9](https://github.com/serverkraken/flow/commit/8f8ddf98f88830f81c35cd20363598a416cf4b4c))
* **worktime/history:** heatmap colour discipline + legend rhythm ([d0481f0](https://github.com/serverkraken/flow/commit/d0481f0c4e5201aca0cbd5b016f7454dfd11a574))


### Performance

* **worktime/history:** historyStyles cache for heatmap & month hot paths ([0e3376b](https://github.com/serverkraken/flow/commit/0e3376bd49e988cb413ef08a57d12534ece30df1))


### Refactoring

* **domain:** single decision tree for pace dots via PaceDotFor ([1932b4f](https://github.com/serverkraken/flow/commit/1932b4fc4d1f6dcc84821c29d478ffcb3e2d41eb))
* **theme:** single-source Kind→Color via theme.KindColor ([f53d4f4](https://github.com/serverkraken/flow/commit/f53d4f4274d4ba1c4935c098da03725ebea993e9))


### Dokumentation

* **theme/status_adapter:** Cyan slot reads as Active, not Info ([c591ff7](https://github.com/serverkraken/flow/commit/c591ff7a8f3808906fe17d3e27a4a61472b677d8))

## [1.2.0](https://github.com/serverkraken/flow/compare/v1.1.0...v1.2.0) (2026-05-14)


### Features

* **domain/status:** filled pace-dots with distinct per-kind colours ([e7da013](https://github.com/serverkraken/flow/commit/e7da013b90d4c9ba696ff4d648af79690ad8e2ed))
* **domain/status:** kindStatusColor helper (Kind → tmux palette slot) ([e684ece](https://github.com/serverkraken/flow/commit/e684eced85788ea10be4a5155c6aa284aa1f08d2))
* **domain/status:** unify day-off hues across TUI + tmux via Sem tokens ([1b0e21b](https://github.com/serverkraken/flow/commit/1b0e21bc53e6eb8663ad147f7bc7bf9e3f7ba8c1))
* **kompendium:** Stufe 7+8 — Live-Palette-Wiring (A7) + Italic raus (B7) ([9a8d31f](https://github.com/serverkraken/flow/commit/9a8d31f45305a2ed2a7f0a16629a57cc74b626be))
* **lint:** Hue-Direktzugriff in Screens verbieten + Cleanup (T2+T3) ([fbbaca5](https://github.com/serverkraken/flow/commit/fbbaca5c49eba68938fa828f41a98dfe56716e97))
* **lint:** Hue-Lint auf kompendium/frontend ausweiten (F3) ([f76dfb6](https://github.com/serverkraken/flow/commit/f76dfb650fc536ad74a08916923628ea9121caa7))
* **markdown_overlay:** chrome (frame + title + sep + footer + statusBar) (F4.3) ([1449ce8](https://github.com/serverkraken/flow/commit/1449ce8966ff126a3f55220b810c26cb7661c42f))
* **markdown_overlay:** close-keys + ExitMsg + Update routing (F4.4) ([e20d8f4](https://github.com/serverkraken/flow/commit/e20d8f4ab787aa1b13ab5c40ad46c3598aaaa98e))
* **markdown_overlay:** code-copy + OSC52 + clipboard fallback (F4.6) ([d316e39](https://github.com/serverkraken/flow/commit/d316e3949036bba85492d080f8f3fa32599e6b7f))
* **markdown_overlay:** package skeleton + tea.Model-style contract (F4.1) ([ae29978](https://github.com/serverkraken/flow/commit/ae2997888cc697931e01f1f203dd628219678cec))
* **markdown_overlay:** search mode + match highlight + counter (F4.5) ([f63e2df](https://github.com/serverkraken/flow/commit/f63e2dfffb4daaeb86f897e27587f23065309ef5))
* **markdown_overlay:** SetError + WithFooterExtras (F4.7) ([d47ec2c](https://github.com/serverkraken/flow/commit/d47ec2c01a710fdfb93cfa93f4d4e0dde6792a9c))
* **markdown_overlay:** WithTitle/WithSource + SetSize/SetTitle/SetSource (F4.2) ([05835a3](https://github.com/serverkraken/flow/commit/05835a3e82f7d3ee88983178dcdf72251577a76c))
* **palette,projects,cli:** Stufe 12 — tmux-Plugin-Migration (palette + projects) ([361257f](https://github.com/serverkraken/flow/commit/361257ff1a083ebed3f2f2ac061d61143e9a67a1))
* **tui/worktime:** unify pace-dot glyph + colour across all surfaces ([ccbe550](https://github.com/serverkraken/flow/commit/ccbe550170a545cbc9dc3dc960526079a1878650))
* **tui:** UI/UX-Review-Pass — A11y, Komponenten-Vokabular, Note-Viewer ([c01fba2](https://github.com/serverkraken/flow/commit/c01fba2fa3b07a34f4224586812fa15df4ddcb3c))
* **worktime/frei:** Kind-Picker mit führendem ○ in Sem-Farbe ([6542bf3](https://github.com/serverkraken/flow/commit/6542bf38a056f1b6684f43ab6638bf7829cc419a))
* **worktime/history:** angehaengte Notes im Drill sichtbar + `o` zum Anzeigen ([4254622](https://github.com/serverkraken/flow/commit/425462242f2db6f4aa3ad2a2e8ae25008f091d0f))
* **worktime/history:** Note an vergangene Tage anhaengen via `n` (drill) ([7de23e1](https://github.com/serverkraken/flow/commit/7de23e1483c73c1183d82968b3e44a94d21b6b94))
* **worktime/history:** Notes-Marker pro Tag in List/Heatmap/Month ([17b2c43](https://github.com/serverkraken/flow/commit/17b2c437a3b0408b286fca730ea3f29a6666eeeb))
* **worktime/history:** O-Key Editor + R-Key Detach im Drill ([5e5e388](https://github.com/serverkraken/flow/commit/5e5e388917cca47d4394749090b59df9df1b118a))
* **worktime:** Brief-Split öffnet integrierten Overlay statt glow (G3) ([63cc390](https://github.com/serverkraken/flow/commit/63cc390ad150030961cadd9f0daf574c434cb593))


### Bug Fixes

* add .claude to .gitignore ([054c02d](https://github.com/serverkraken/flow/commit/054c02dbc7f30eea2d0569e4f3ddfbfa76c5f5b3))
* **confirm,kompendium:** Stufe 5 — Sem()-Sweep (B3) ([601b461](https://github.com/serverkraken/flow/commit/601b4617ba07e417e324a0172569afa532e661ed))
* **dayoffs:** AddRange atomar via Store.AddBatch (L5) ([dfd8706](https://github.com/serverkraken/flow/commit/dfd87065174b7ed7d26cb9148804a1c1157d1675))
* **dayoffstsv:** Cross-Process-Lock analog linkstsv (S3) ([e9df9cb](https://github.com/serverkraken/flow/commit/e9df9cb7f9c1cdc553c6b2e549f2633fd732f83c))
* **domain,palette,worktime:** correctness bugs (round4-B) ([6bc7615](https://github.com/serverkraken/flow/commit/6bc76151ad8457e68162d04c5510818b34aa5311))
* **domain:** parseHumanDuration lehnt negative h/m ab (L1) ([d214079](https://github.com/serverkraken/flow/commit/d214079cb551c58ec3ccdca7903ebac5bfcd9580))
* **glyphs,worktime,markdown,kompendium,palette:** Stufe 3 — Glyph-Whitelist (B1) ([84b0864](https://github.com/serverkraken/flow/commit/84b0864e8e475639e0019d47cd838e79a76bcf72))
* **kompendium,markdown_overlay:** context-cancel + race-fix (round4-D) ([39d3439](https://github.com/serverkraken/flow/commit/39d3439bec607e536641e4f934dc9ce1871b17e3))
* **kompendium/tarsnapshot:** cumulative-bytes-Cap gegen many-entries-Bombs (F6) ([571c6a1](https://github.com/serverkraken/flow/commit/571c6a18b767be7c9548dad3146c45a2f312fa7d))
* **kompendium:** Stufe 6 — DE-Strings (B4) ([57a8158](https://github.com/serverkraken/flow/commit/57a81582f4d1afb3e66dd71386fae5a5636934e8))
* **kompendium:** Stufe 9 — Komponenten-Integration partial (B5) ([34ff0f6](https://github.com/serverkraken/flow/commit/34ff0f667c8374e66f7afe1cadc6de3fad84d5ca))
* **markdown_overlay:** truncate long titles + progressive footer degrade (review-C) ([37b77cf](https://github.com/serverkraken/flow/commit/37b77cf20cf0ab64007c7e710aa5f226bfe3b758))
* **markdown_overlay:** wire Top/Bottom keys (g/G/home/end) (F4-review) ([3a627e4](https://github.com/serverkraken/flow/commit/3a627e4b06508a4dadb0a52701fcf73f0270d254))
* **markdown,theme:** Stufe 2 — A11y (A2, A3) ([d236a4b](https://github.com/serverkraken/flow/commit/d236a4b534bbb3f9a3c452410a39668329beb30e))
* **security:** Palette-Action / Pager-Viewer / Git-Remote-URL härten (S1+S2+S3) ([d995fd3](https://github.com/serverkraken/flow/commit/d995fd3bbe6162a1406bf9449b5dad2b4f046b4c))
* **security:** Tar-Bombe + YAML-Alias-Amplification kappen (S4) ([a77da34](https://github.com/serverkraken/flow/commit/a77da344415350e9148815a6168b82730d68cf15))
* **status:** bannerThreshold-Konstanten extrahieren (L2) ([aeb6f4d](https://github.com/serverkraken/flow/commit/aeb6f4d2fa236fad76b47df35fad7c6637157f5e))
* **strings,worktime,palette,projects:** Stufe 4 — Strings-Sweep (B2) ([224f4b3](https://github.com/serverkraken/flow/commit/224f4b3d37214999cd000c9111e33fc9cb7783fe))
* **theme,docs:** Stufe 11 — Sem().Border Doku + Test (B8) ([2634510](https://github.com/serverkraken/flow/commit/263451071c7024a3ca568ccb5bc91ceab1be6bb1))
* **usecase,kompendium,adapter:** data integrity (round4-C) ([3746151](https://github.com/serverkraken/flow/commit/37461516ed64a33b1d59b0f4547fcf6d2b760682))
* **usecase:** SessionWriter.Stop idempotenter Retry (round4-F) ([d477743](https://github.com/serverkraken/flow/commit/d477743893cb751ba10bd5fe139a41460cc6a793))
* **worktime,kompendium:** Datenintegrität — atomare Multi-Midnight-Persistenz, Validierung, Lock-Fehler-Surfacing (B1+B2+B3+Q1+Q5+Q3+Q4) ([9ebe6ba](https://github.com/serverkraken/flow/commit/9ebe6ba659f4ca16b8b2d1a9ff2ad9b51b993e4f))
* **worktime,markdown:** Stufe 1 — Logik-Bugs (A1, A4, A5, A6, A8) ([9d4b4f5](https://github.com/serverkraken/flow/commit/9d4b4f5c228501c03919e05576136f43225c9518))
* **worktime:** heute-Edit akzeptiert Uppercase-Duration (S1) ([8bbd941](https://github.com/serverkraken/flow/commit/8bbd941f80dd03a3d08b6a076ba333cd41899770))
* **worktime:** Heute-Note nach tmux-Pane-Resize re-rendern (F1) ([8d15281](https://github.com/serverkraken/flow/commit/8d15281c771df9680f35382ed62adad3e1740399))
* **worktime:** Note-Viewer rendert ohne Doppel-Border (FullScreen-Opt-In) ([2c794d8](https://github.com/serverkraken/flow/commit/2c794d8f007f06019a86cf3ac1656def60056181))
* **worktime:** resolve formatting and robustify integration tests ([f1dd30a](https://github.com/serverkraken/flow/commit/f1dd30aebce95a79e9edc91e80450cd2aeba96b7))
* **worktime:** scheduleTick consults ports.Clock not time.Now (review-B) ([52e9487](https://github.com/serverkraken/flow/commit/52e9487948e7705848e06e30546d8a402d0f5ccc))
* **worktime:** session.Date bei Midnight-Cross-Stop (S2) ([61d91c1](https://github.com/serverkraken/flow/commit/61d91c15cf43f19432c86bd9aea290cec5146873))
* **worktime:** SyncGermanHolidays nimmt Location-Parameter (L3) ([2f9bd7e](https://github.com/serverkraken/flow/commit/2f9bd7eff9e63255fbbbe01aac4f7d44881456ea))


### Performance

* **palette,worktime:** hot-path style cache (round4-E) ([42a7ccd](https://github.com/serverkraken/flow/commit/42a7ccd62903584d77cc9bae2e27d3bc191704c7))
* **worktime:** promote NewStyle bases out of render hot path (review-D) ([a489fec](https://github.com/serverkraken/flow/commit/a489fec39b40cbe6fba6267ecb427665b38e9161))


### Refactoring

* **adapters:** bash-safety + flock-acquire zentralisiert + Pager-Leak (F2+M1+M3) ([721c1a9](https://github.com/serverkraken/flow/commit/721c1a9aef40f929611cbf6ee278e8c07f420e02))
* **cheatsheet:** markdown_overlay als 5. Caller (round4-H) ([bba3638](https://github.com/serverkraken/flow/commit/bba363853e514dabeb44291ac5c2f9c105fbaf3f))
* **cmd/flow,kompendium/cli:** share kompendium write-cmd dispatch (review-E) ([3b81846](https://github.com/serverkraken/flow/commit/3b81846110c03dd183395064ed6758dd403e901e))
* **composition:** Env-Var-Disziplin — alle os.Getenv im Composition Root (A1) ([b0e49b8](https://github.com/serverkraken/flow/commit/b0e49b8bce97f566f23fb48939c6a92579f6a0d9))
* **domain/status:** banner uses ○ + kindStatusColor ([824d933](https://github.com/serverkraken/flow/commit/824d933eb0efcfbce626e69194a7c4c1f9f75990))
* **domain/status:** pace-dots use ○ + kindStatusColor ([e07bcb7](https://github.com/serverkraken/flow/commit/e07bcb7df98eac249282a481ba0313bcbcf71943))
* **editor,ports:** NoteLauncher.View + externer Note-Viewer entfernt (G2) ([b87ffe3](https://github.com/serverkraken/flow/commit/b87ffe3e84c933d1ff4120a7f113869c07b1d5c7))
* **kompendium/browse:** markdown_overlay statt eigener view.Model (F4.9) ([d5e75c6](https://github.com/serverkraken/flow/commit/d5e75c6501ac2d593f426b2ea9254ccd85756c7b))
* **kompendium/browse:** Stufe 10 — model.go Monolith splitten (B6) ([4c2c80c](https://github.com/serverkraken/flow/commit/4c2c80c60c1722d2aa62568b37adc79b4e7cefa1))
* **kompendium/browse:** view.go → render_root.go (Q1) ([964eab6](https://github.com/serverkraken/flow/commit/964eab63bd091668b05b95f88e748dc4895dcca4))
* **kompendium:** delete view/ package nach markdown_overlay-Migration (F4.10) ([ee709e2](https://github.com/serverkraken/flow/commit/ee709e2b5fbc3215c0da504e5be390d1a56607ce))
* **kompendium:** split browse/model.go in fokussierte Files (M1) ([30f9b18](https://github.com/serverkraken/flow/commit/30f9b1834f228720982d5b802d1241f250ce7784))
* **markdown:** WikiLink-AST-Node-Name entkompendiumieren (C3) ([8558375](https://github.com/serverkraken/flow/commit/855837539d86f58ac3450989293843dad3795f92))
* **palette:** split palette/model.go in fokussierte Files (M2) ([88e9ebc](https://github.com/serverkraken/flow/commit/88e9ebc23a05815dc161c123cf2cfbfd4bcf5c18))
* **session_writer:** Pause idempotent auch für ErrStopBeforeStart (M2) ([691b615](https://github.com/serverkraken/flow/commit/691b615b05366ee6f5a62575583e74814fd99d23))
* **strings:** String-Duplikate in components/strings konsolidieren (C1) ([aa1478e](https://github.com/serverkraken/flow/commit/aa1478ea3b3acfc8c12317aab572dbdbc5d5d839))
* **usecase,shellsafe,domain:** design touches (round4-G) ([dc7cc81](https://github.com/serverkraken/flow/commit/dc7cc81325b13de5e4aa57dad59ec79339022c11))
* **usecase:** DayOffReader entfernen (L4) ([43d31db](https://github.com/serverkraken/flow/commit/43d31db8d0ac55381b29a94bcec7f5f06034c751))
* **worktime,docs:** Stufe 13 — Vereinheitlichung (C1 + C4) ([2591e66](https://github.com/serverkraken/flow/commit/2591e66f748e1dd803504dab25ec10662d631be2))
* **worktime,markdown_overlay,lint:** mechanical cleanup (review-A) ([551e58c](https://github.com/serverkraken/flow/commit/551e58c0f59ec3afb6c7f1798aa723ed137a5c19))
* **worktime/brief:** markdown_overlay statt eigener briefView-Struct (F4.11) ([cc206b4](https://github.com/serverkraken/flow/commit/cc206b4f04093ba87e4c6c27cee6079d1c7810ea))
* **worktime/frei:** Kind-Summary in Sem-Farben ([248d587](https://github.com/serverkraken/flow/commit/248d587ea83d8d741f3e961fb9dd440f70e0b5b8))
* **worktime/heatmap:** cell + legend use ○ + per-kind colour ([d8fef84](https://github.com/serverkraken/flow/commit/d8fef8450ecf0b67e8be04279cdab208a6aaf36b))
* **worktime/heute:** NoteAttach auf shared picker konsolidieren ([0f0d4e1](https://github.com/serverkraken/flow/commit/0f0d4e14b28aaea22ecd518a70c5a0d14a018073))
* **worktime/month:** cell uses ○ + per-kind colour ([e881265](https://github.com/serverkraken/flow/commit/e881265a2ccac42f00ccdfec238957855ebea33d))
* **worktime/note:** markdown_overlay statt eigener noteView-State (F4.12) ([78894e3](https://github.com/serverkraken/flow/commit/78894e3272140ce74a1d864a8f87a5225a2c0bcc))
* **worktime/week:** renderPace uses ○ + kindStyle for free days ([82f4a01](https://github.com/serverkraken/flow/commit/82f4a010c5a6c2d53fc41d8f30e8db2ffc4ad2dc))
* **worktime:** Heatmap-Legend-Glyphen aus Whitelist (C2) ([c7c539f](https://github.com/serverkraken/flow/commit/c7c539f81b8e3c7c58d32c2786cfb1986a5c6a15))
* **worktime:** Hue-Direktzugriffe auf Sem() umstellen (T1) ([8a0a3a5](https://github.com/serverkraken/flow/commit/8a0a3a513cfda4edf0eea7fa3772834ccd95bf52))
* **worktime:** split today_dialog.go in fokussierte Files (M3) ([90645b8](https://github.com/serverkraken/flow/commit/90645b8559b86759cc86f333e9a81ded0414b9f5))
* **worktime:** toten Glow-Fallback im Heute-NoteView entfernen (G1) ([73cb9c5](https://github.com/serverkraken/flow/commit/73cb9c50562ed00ec662e9a47f362a7f02332e64))


### Dokumentation

* A1-Plattform-Detection-Ausnahme dokumentieren (Q2) ([55344ca](https://github.com/serverkraken/flow/commit/55344caae7e3c70a816c6998bed18c342462c937))
* **domain/status:** document StatusPalette slot semantics ([4e109b8](https://github.com/serverkraken/flow/commit/4e109b8d250ef3c53f4e6a0b5f18e2b189699fda))
* **glow-migration:** glow als Dependency entfernt + Brief-Tests (G4) ([d2d3b04](https://github.com/serverkraken/flow/commit/d2d3b04f146cd38417e80c346c08f0a7c4e6f1d0))
* **markdown_overlay:** annotate SetTitle + isClosingFence (F4-review) ([22cea50](https://github.com/serverkraken/flow/commit/22cea50ec303615c97c57e9335b2ffd46ddf1aed))
* **markdown_overlay:** clarify init() palette lifecycle (F4-review) ([e9e82a0](https://github.com/serverkraken/flow/commit/e9e82a042d810e5f4eae5c55e844b20ed9fe55b9))
* **plans:** implementation plan for unified dayoff glyphs (12 tasks) ([a6ba9c5](https://github.com/serverkraken/flow/commit/a6ba9c5603f94a4b759c9238a151cb4f01f21767))
* **specs:** extend dayoff-glyphs spec to tmux status segment ([07ea152](https://github.com/serverkraken/flow/commit/07ea152024ca49b0c41d8cb9b697cd68c0275463))
* **specs:** markdown_overlay-Component-Design (F4) ([f4c70d8](https://github.com/serverkraken/flow/commit/f4c70d82e3f296918b8027373c318154267afa8c))
* **specs:** markdown_overlay-Implementation-Plan (F4) ([d003b08](https://github.com/serverkraken/flow/commit/d003b08b59c7db105e8eac07908426755b789978))
* **specs:** unified dayoff glyphs design (Pace/Heatmap/Monat/Frei) ([27089a1](https://github.com/serverkraken/flow/commit/27089a1433e477bf46b904727f4d20e2d6ff1f7b))
* **worktime,Makefile:** stale Kommentare nach A1/G3 aktualisiert (F7) ([0edee83](https://github.com/serverkraken/flow/commit/0edee8356df0a1550b19d5472a26b0bb57ea086a))

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
