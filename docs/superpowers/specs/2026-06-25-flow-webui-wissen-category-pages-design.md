# flow WebUI - Wissen-Kategorien als Unterseiten (Design Spec)

Datum: 2026-06-25
Status: Review

## 1. Kontext

Die Wissen-Fläche ist aktuell eine Sammelseite unter `/wissen`. Sie rendert
Daily, Projekt-Notizen, Frei und Agent/System als Sektionen auf einer Seite.
Das ist für kleine Datenmengen brauchbar, wird mit realen Notizen aber
unübersichtlich. Zusätzlich steht das Inhaltsverzeichnis auf mobilen
Dokumentseiten derzeit nach dem gesamten Artikel, was für lange Dokumente wenig
hilfreich ist.

Dieser Slice baut die vorhandene Wissen-Fläche nicht neu, sondern splittet die
Informationsarchitektur:

- `/wissen` wird eine ruhige Übersicht.
- Kategorie-Listen bekommen eigene Unterseiten.
- Artikelkarten auf Kategorie-Unterseiten zeigen eine kurze Inhaltsvorschau.
- Das Inhaltsverzeichnis steht auf mobilen Dokumentseiten vor dem Artikel.

Bestehende Detail-, Editor-, Preview-, Delete- und Reembed-Routen bleiben
unverändert.

## 2. Ziele

1. Wissen ist nicht mehr als vier lange Kategorien auf einer Seite organisiert.
2. Jede Kategorie ist direkt verlinkbar und browserfreundlich adressierbar.
3. Nutzer sehen vor dem Öffnen eines Artikels genug Kontext, um den richtigen
   Treffer zu erkennen.
4. Mobile Dokumentseiten bieten das Inhaltsverzeichnis vor dem langen Inhalt.
5. SSE, Suche, Tags, Pagination und vorhandene Markdown-Darstellung bleiben
   erhalten.

## 3. Routen und Navigation

Neue Kategorie-Routen:

| Route | Inhalt |
| --- | --- |
| `/wissen` | Übersicht mit Suche, Neu-Button und vier Kategorie-Kacheln |
| `/wissen/daily` | Daily-Notizen |
| `/wissen/projekte` | Projekt-Notizen, weiterhin nach Projekt gruppiert |
| `/wissen/frei` | freie Notizen |
| `/wissen/system` | Agent/System-Notizen (`agent`, `memory`, `instruction`, `skill`, `plan`) |

HTMX/SSE-Fragmente bekommen parallele Fragment-Routen:

| Route | Inhalt |
| --- | --- |
| `/ui/wissen/overview` | Übersicht-Fragment |
| `/ui/wissen/list/{category}` | Kategorie-Listen-Fragment |

`/wissen/{id}` bleibt die Dokument-Detailroute. Die neuen Kategorie-Slugs dürfen
daher nicht als Dokument-IDs behandelt werden. Die HTTP-Routen müssen so
registriert sein, dass `/wissen/daily`, `/wissen/projekte`, `/wissen/frei` und
`/wissen/system` vor `/wissen/{id}` gematcht werden.

Die linke Hauptnavigation bleibt unverändert: "Wissen" zeigt auf `/wissen`.
Innerhalb der Wissen-Fläche ersetzt eine Kategorie-Navigation den bisherigen
Scroll-Spy-Tabstrip. Sie verlinkt auf die vier Unterseiten und markiert die
aktive Kategorie. Auf der Übersicht sind alle Kategorie-Kacheln gleichwertige
Einstiege.

## 4. `/wissen` Übersicht

Die Übersicht ist kein "Alle Dokumente"-Dump mehr. Sie zeigt:

- Header "Wissen" mit "Neu"-Button.
- Globale Suche. Bei `?q=...` rendert `/wissen` weiterhin globale
  Suchergebnisse statt der Übersicht, damit der bisherige Suchpfad nicht
  verloren geht.
- Tag-Chips. Tag-Auswahl bleibt als Query-Filter erhalten.
- Vier Kategorie-Kacheln mit:
  - Kategorie-Label,
  - Count,
  - kurzer Beschreibung,
  - Link zur Kategorie-Unterseite,
  - 2-3 neuesten Dokumenttiteln als Preview-Liste, aber ohne Body-Vorschau.

Die Übersicht ist bewusst ruhig. Body-Vorschauen erscheinen erst auf
Kategorie-Unterseiten, weil die Übersicht sonst wieder zu dicht wird.

## 5. Kategorie-Unterseiten

Kategorie-Unterseiten behalten die Bedienmuster der bisherigen `/wissen`-Liste:

- Header mit Kategorie-Label, Count, "Neu"-Button und Rücklink/Navigation zur
  Übersicht.
- Suche und Tag-Chips.
- Pagination mit erhaltener Query.
- SSE-Refresh bei `document.created`, `document.updated`, `document.deleted`.

Bei `?q=...` sucht die Unterseite innerhalb der Kategorie. Das Ergebnis bleibt
eine flache Ergebnisliste mit markiertem Snippet. Wenn die bestehende
Search-Usecase-Signatur keinen Typfilter unterstützt, wird der Typfilter im
Web-Handler nachgelagert angewendet oder der Store/Usecase gezielt erweitert;
die Implementierungsplanung entscheidet anhand der kleinsten sauberen Änderung.

Kategorie-Mapping:

- `daily`: `domain.DocDaily`
- `projekte`: `domain.DocProject`
- `frei`: `domain.DocFree`
- `system`: alle nicht-humanen/Systemtypen (`DocAgent`, `DocMemory`,
  `DocInstruction`, `DocSkill`, `DocPlan`)

Projekt-Notizen bleiben unter Projekt-Headern gruppiert. Alle anderen
Kategorien verwenden kompakte vertikale Artikelkarten.

## 6. Artikelvorschau

Jeder Artikel auf einer Kategorie-Unterseite zeigt zusätzlich zum Titel,
Pfad/Projekt und Tags eine kurze Vorschau aus dem Body.

Vorschau-Regeln:

- ungefähr die ersten 5 sinnvollen Textzeilen,
- Frontmatter wird entfernt,
- Markdown-Headings, Listenpunkte und normaler Text werden als lesbarer Plain
  Text dargestellt,
- Wikilinks werden ohne Rohsyntax angezeigt,
- Codeblöcke werden nicht vollständig ausgebreitet; sie erscheinen höchstens als
  kurze Code-Hinweiszeile oder werden ausgelassen,
- HTML wird nicht als HTML gerendert,
- mehrere Leerzeilen werden verdichtet,
- Ausgabe wird serverseitig escaped,
- die UI begrenzt die Vorschau zusätzlich auf maximal 5 sichtbare Zeilen.

Die Vorschau ist ein Orientierungstext, kein Mini-Markdown-Renderer. Sie soll
keine Tabellen, Callouts oder Syntaxhighlighting nachbauen.

## 7. Mobile Dokumentseite

Desktop bleibt wie bisher:

- Markdown-Inhalt links,
- Inhaltsverzeichnis und Backlinks in der rechten Rail.

Mobile ändert die Reihenfolge:

1. Dokument-Header
2. Inhaltsverzeichnis als kompakter Block
3. Markdown-Inhalt
4. "Referenziert von"

Das mobile Inhaltsverzeichnis soll nicht nach dem gesamten Artikel stehen. Es
kann als normaler Block oder als aufklappbarer `<details>`-Block gerendert
werden. Empfehlung: `<details open>` nur dann, wenn die ToC kurz ist; ansonsten
kompakt geschlossen mit klarer Überschrift "Inhalt". Die Implementierung darf
eine einfache immer sichtbare Variante wählen, wenn sie stabiler und klarer ist.

Backlinks bleiben auf Mobile nach dem Artikel, weil sie nachgelagerter Kontext
sind.

## 8. ViewModel- und Datenfluss

Die bestehende Gruppierungslogik (`GroupDocsByCategory`) bleibt der Startpunkt,
wird aber um eine explizite Kategorie-View ergänzt:

- `WissenOverviewVM`: Counts, Kategorie-Kacheln, neueste Titel,
  Suche/Tags/Query.
- `WissenCategoryVM`: aktive Kategorie, Listeninhalt, Preview-Text,
  Suche/Tags/Pagination/Query.

Wenn möglich, bleibt die Domain-Schicht unverändert. Typfilterung ist eine
WebUI-/Usecase-Frage, keine neue Dokumenteigenschaft.

## 9. Tests

TDD-Abdeckung:

- HTTP-Routen:
  - `GET /wissen` rendert Übersicht statt aller vier langen Sektionen.
  - `GET /wissen/daily`, `/wissen/projekte`, `/wissen/frei`,
    `/wissen/system` liefern nur passende Dokumenttypen.
  - Kategorie-Routen werden nicht von `/wissen/{id}` verschluckt.
- ViewModel:
  - Kategorie-Mapping für daily/project/free/system.
  - Overview-Counts und neueste 2-3 Titel.
  - Preview-Text entfernt Frontmatter, HTML und Markdown-Rohsyntax
    ausreichend für die Kartenansicht.
- Templates:
  - Übersicht enthält Kategorie-Kacheln mit Links.
  - Kategorie-Unterseite enthält Preview-Text und Pagination.
  - Mobile Dokumentlayout rendert ToC vor Markdown-Inhalt und Backlinks danach.
- Existing gates:
  - `make generate`
  - `make web`
  - `make ci`

Wenn `NO_COLOR=1` in der lokalen Shell gesetzt ist, wird CI für die bestehenden
TUI-Markdown-Tests mit `env -u NO_COLOR make ci` ausgeführt.

## 10. Nicht-Ziele

- Keine neue Dokumenttyp-Taxonomie.
- Keine Migration bestehender Dokument-IDs oder Pfade.
- Kein clientseitiges SPA-Routing.
- Keine Vorschau auf der `/wissen`-Übersicht.
- Keine vollständige Markdown-Rendering-Engine für Vorschau-Text.
- Kein Redesign der linken Hauptnavigation.

## 11. Akzeptanzkriterien

- `/wissen` ist eine Übersicht mit vier Kategorie-Einstiegen und wirkt nicht wie
  eine lange Sammelliste.
- Jede Kategorie ist über eine eigene URL erreichbar.
- Kategorie-Unterseiten zeigen Artikelvorschauen mit maximal 5 sichtbaren
  Zeilen.
- Suche, Tags, Pagination und SSE funktionieren auf Übersicht und
  Kategorie-Unterseiten passend zur jeweiligen Seite.
- Auf Mobile steht das Inhaltsverzeichnis der Dokumentseite vor dem Artikel.
- `make ci` ist grün, bevor der Implementierungs-Commit erstellt wird.
