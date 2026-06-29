# B3d Memory-Migration Manifest — Review Summary

**Generated:** 2026-06-29 (classification subagent, sonnet) · **Manifest:** `manifest.tsv` (115 rows)
**Review this file, then edit `manifest.tsv` directly before the import runs.**

## Counts
- **Scope:** flow=96 · homelab-study=10 · global=8 · privat=1 · rtl-extern=0
- **Pinned:** 12 · **Skipped (not imported):** 23 · **Hard-to-classify (flagged):** 8

## Skipped clusters (23) — confirm you agree these stay out of flow
- **Abandoned `next`-branch milestones (7):** project_flow_server_only_rebuild, project_next_poc_review, project_next_poc_fixes_done, project_plan_b_progress, project_plan_e_webui, project_r2a_done, project_r2b_done
- **Old pre-rebuild code-review passes on `main` (4):** project_review_followups, project_review_open_points, project_review_round3, project_review_round4
- **Old `main` pre-rebuild milestones (3):** project_dayoff_glyph_unification, project_hexagonal_refactor, project_kompendium_integration
- **Planned→done superseded pairs (8):** project_flow_rebuild_b1_planned, ..._keyboard_grammar_planned, ..._m3_planned, ..._m3d_docs_plan, ..._m4_pm_planned, ..._m4_slice3_tui_planned, ..._nachbuchen_planned, ..._webui_slice1_planned
- **Explicitly superseded (1):** project_flow_rebuild_worktime_subtab_strip (MEMORY.md says "SUPERSEDED by worktime_parity")

## Hard-to-classify (8) — please eyeball the scope choice
- `feedback_htmx_hxboost_blocks_oidc_redirect` — flow WebUI tech, but a general OIDC gotcha (flow vs global?)
- `feedback_go_keyring_base64_prefix` + `feedback_macos_keychain_2kb_limit` — flow-specific vs. macOS cross-project (flow vs global?)
- `project_flow_oidc_multi_provider` — flow code + homelab Authentik (flow vs homelab?)
- `project_flow_homelab_deploy` — deploy/homelab boundary (flow vs homelab?)
- `project_flow_client_server_phase1_spec` — pre-rebuild origin spec, borderline skip
- `project_tmux_flow_migration` — flow vs dotfiles scope
- `reference_b1_followups_rollup` — no standard frontmatter (title/tags may need a manual touch)

## How to review
1. Open `manifest.tsv`. Columns: `file ⇥ scope ⇥ tags ⇥ pin ⇥ keep`.
2. Fix any `scope` you disagree with (valid: `global`, `github-com-serverkraken-flow`, `github-com-serverkraken-homelab-study`, `privat`, `rtl-extern`).
3. Check the 12 `pin=y` rows are the ones you actually want always-loaded, and the 23 `keep=skip` rows are truly droppable.
4. Tags are optional topical hints; the `metadata.type` (user/feedback/project/reference) is added automatically by the importer.
