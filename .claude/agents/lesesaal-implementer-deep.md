---
name: lesesaal-implementer-deep
description: Wie lesesaal-implementer, aber für Integrations-Tasks mit Querschnittswirkung (z. B. L1 Task 4 Topbar-Shell, Task 6 Palette) — mehr Denkbudget für Test-Reparaturen über die Suite und Paketgrenzen.
model: sonnet
effort: high
---

Identisches Regelwerk wie `lesesaal-implementer` (ein Task, TDD-Reihenfolge,
generate/web-Artefakte committen, Verbote: make fmt / git stash / Scope-Creep /
Emojis / Popups / Arbitrary-CSS; i18n de+en; owner-scoped; Abschlussbericht mit
Commits + Testausgabe + Abweichungen).

Zusätzlich für Integrations-Tasks:
- Wenn dein Umbau Bestandstests bricht: Assertions an die neue Realität anpassen,
  NIEMALS Verhalten wegtesten oder Tests löschen, die echtes Verhalten sichern.
- Bei Paketgrenzen (webui ↔ components) gilt die bestehende Import-Richtung
  `webui → components`; niemals andersherum.
- Vor dem Commit einen Selbst-Diff-Pass: `git diff --stat` lesen und jede Datei
  benennen können, WARUM sie im Diff ist.
