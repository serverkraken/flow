# SCREENMAP — Screen → Route → Datei

Nummern sind die Screen-Nummern aus dem Konzeptdokument (Kapitel 04) und die
Referenz in Gesprächen. Die Zieldateien liegen unter
`internal/adapter/webui/`. „neu" heißt: die Fläche existiert im Repo noch nicht.

## Bau-Reihenfolge

### Slice 1 — Tokens
`web/tailwind.css` (`:root` + Dunkel-Block), danach `make web`, `make verify-css`.
Referenz: `TOKENS.md`. Keine Screen-Arbeit.

### Slice 2 — Schiene und Hülle
| Screen | Fläche | Datei |
| --- | --- | --- |
| — | Hülle, Ebenenstreifen | `components/appshell.templ`, `components/base.templ` |
| — | Schiene 264px, Einzug je Ebene, Auf/Zuklappen | `cockpit_rail.templ`, `components/sitenav.go` |
| — | Brotkrume | `components/breadcrumb.templ` |
| 02 | Register-Einstieg mit README | `nodes.templ`, `cockpit.templ` |
| 08 | Register-Einstellungen: Monogramm, Farbe, Banner | `nodes.templ`, `components/avatar.templ` |
| 23 | Neues Engagement / Vorhaben / Repo | `nodes.templ`, `components/form/` |

### Slice 3 — Listenzeile und Datumsspalte
Der Baustein aus Katalog 3.10/3.11, der in rund 20 Screens vorkommt.
| Was | Datei |
| --- | --- |
| Zeile mit Typ-Marke, Auswahlkante, Datumsspalte | `components/docrow/`, `activity_row.templ` |
| Sortierkopf (Spaltenüberschrift als Schalter, Pfeil, Menü) | neu: `components/sorthead.templ` |
| Filterchips mit Trefferzahl | `components/chip.templ` |
| Datumsstaffel `heute 14:32` / `Fr` / `11.08.` / `08.11.25` | `internal/timefmt` erweitern + `format.go` |
| Lesezeit (200 W/min, unter 2 min entfällt) und Aufwand | `format.go`, `document_vm.go` |

Regel: eine Bedeutung pro Spalte. Rechts steht **immer** ein Datum, standardmäßig
`zuletzt geändert`; Lesezeit und Aufwand stehen in der Herkunftszeile.

### Slice 4 — Lesesaal
| Screen | Fläche | Datei |
| --- | --- | --- |
| 01 | Wissen im Vorhaben gelesen | `wissen.templ`, `document.templ` |
| 10 | Langer Artikel mit Bildern und Tabellen | `document.templ`, `components/markdownprose.templ` |
| 06 | Bibliothek: Suche über alle Register | `wissen.templ` |
| 18 | Artefakt-Verwaltung über alle Register | `wissen_artifacts.templ`, `cockpit_artifacts.templ` |
| 34 | Artefakt ansehen, einbetten, ersetzen | `cockpit_artifacts.templ`, `artifact_embed.go` |
| 17 | Uploads, verlinkbar per `![[…]]` und `[[…]]` | `cockpit_artifacts.templ`, `components/insertpicker.templ` |

### Slice 5 — Zeit
| Screen | Fläche | Datei |
| --- | --- | --- |
| 09 | Heute mit Stempeluhr | `heute.templ`, `timerwidget.templ` |
| 05 | Woche | `woche.templ` |
| 32 | Historie: Monat für Monat über alle Register | `historie.templ` |
| 26 | Buchung korrigieren, teilen, nachtragen | `components/sessiondialog.templ`, `components/sessionrow.templ` |
| 25 | Abwesenheiten: Urlaub, Krank, Feiertage | `frei.templ` |
| 28 | Engagement-Zeitverlauf Monat für Monat | `historie.templ` oder neu `zeitverlauf.templ` |
| 24 | Startseite: Bestand, Auslastung, Erträge | `home.templ`, `components/stattile.templ` |
| 03 | Puls: was insgesamt passiert | `home.templ`, `activity_row.templ` |

### Slice 6 — Schreiben
| Screen | Fläche | Datei |
| --- | --- | --- |
| 12 | Neue Karte anlegen (⌘N, ohne Pfad-Tipperei) | `components/fuzzypicker.templ`, `editor.templ` |
| 11 | Markdown-Modus: Quelle links, Vorschau rechts | `editor.templ`, `mode_seg.templ` |
| 16 | Rich-Text-Modus, gleiche Quelle | `editor.templ` |
| 07 | Kontext kuratieren links, lesen rechts | `kontext.templ`, `cockpit_context_vm.go` |
| 04 | Heutige Tagesnotiz | neu: `tagebuch.templ` |
| 27 | Stellen markieren und je Register zuordnen | neu: `tagebuch.templ` |
| 14 | Archiv alter Tagesnotizen | neu: `tagebuch.templ` |

### Slice 7 — Ausstattung und Repo
| Screen | Fläche | Datei |
| --- | --- | --- |
| 13 | Ausstattung je Agent: was ausgerollt wird | neu: `ausstattung.templ` |
| 13 A | Skills: aufgelöste Sicht je Projekt, mit Herkunft | neu: `ausstattung_skills.templ` |
| 13 B | Weisungen: CLAUDE.md / AGENT.md über drei Ebenen | neu: `ausstattung_weisungen.templ` |
| 15 | Repo-Einstieg mit Branches und README | neu: `repo.templ` |
| 15 A | Zugang: nur öffentlicher Schlüssel, nie ein Geheimnis | neu: `repo_zugang.templ` |
| 33 | Datei, Commit und Diff im Lesesaal | neu: `repo_datei.templ` |

Vorhandene Bausteine dafür: `internal/gitremote`, `internal/gitworktree`,
`internal/clientauth`.

### Slice 8 — Zustände und Formate
| Screen | Fläche | Datei |
| --- | --- | --- |
| 19 | Dialoge: Bestätigen, Umbenennen, Eingeben | `components/dialog.templ`, `components/confirm/` |
| 20 | Fehlerseiten: 404, kein Zugriff, 500, offline | neu: `error.templ` |
| 29 | Anmelden, leerer Kasten, erstes Engagement | `auth.templ`, `components/emptystate.templ` |
| 30 | Konto, Sollzeiten, Erscheinungsbild, **Sprache** | `einstellungen.templ` |
| 31 | Suche als Overlay (⌘K) | `palette.templ`, `components/palette.templ` |
| 22 | Dunkelmodus | `web/tailwind.css` Dunkel-Block |
| 21 A–D | Tablet 768–1199px | alle betroffenen Templates, Tailwind-Umbrüche |
| 21 | Telefon 390px: Dock unten, Register als eigene Seite | `components/appshell.templ` |
| — | Leer, lädt, kaputt (Katalog 3.8) | `components/emptystate.templ` + Skeletons |
| — | Eingeben ohne Maus (Katalog 3.9) | `components/form/` |

## Katalog-Kapitel als Referenz

| Kapitel | Inhalt |
| --- | --- |
| 3.1–3.3 | Farbe, Schrift, Maß und Raster |
| 3.4–3.6 | Bausteine, Symbolik, Zeichen mit Bedeutung |
| 3.7 | Bewegung und Dauer |
| 3.8 | Leer, lädt, kaputt |
| 3.9 | Eingeben und Bedienen ohne Maus |
| 3.10 | Alter, Sortieren, Filtern — die vier Zeitpunkte |
| 3.11 | Lesezeit und Aufwand |
| 3.12 | Zwei Sprachen, eine Ordnung (i18n) |
| 3.13 | Von 1500 auf 390: wohin jede Fläche wandert |

## Navigationsbaum

Kapitel 02 B listet **46 Wege** von der Schiene aus, jeweils mit Ziel und
Zustand. Das ist die Prüfliste für Routen: jeder Weg im Baum braucht eine Route,
die den erwarteten Status liefert (Live-Done-Gate aus `AGENTS.md`).
