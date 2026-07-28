# `flow_patch_doc`: Größendelta + Schrumpf-Guard (Design Spec)

- **Datum:** 2026-07-28
- **Branch:** `patch-doc-shrink-guard` (Worktree `../flow-patch-doc-shrink-guard`, von `main` @ `6f7dfd8`)
- **Status:** Draft — zur Review
- **Umfang:** eine in sich geschlossene Anpassung an `cmd/flow-mcp`.
  Kein Slice eines größeren Programms.
- **Auslöser:** [[bugs/mcp-patch-doc-replace-section-truncation]] (flow-Kompendium)
- **Verwandt:** [[feature-requests/document-revisions-und-papierkorb]] — dieser
  Slice **verhindert** den Fehlgriff, der FR **behebt** ihn im Nachhinein.
  Beide greifen ineinander: das Größendelta hier ist dieselbe Zahl, die
  `flow_doc_history` später je Revision ausweisen soll.

---

## 1. Kontext & Ziel

Ein `replace_section` auf die **H1** eines Dokuments ersetzt nicht den Abschnitt
bis zur nächsten Überschrift, sondern alles von der H1 bis zum Dateiende. Der
Aufrufer bekommt eine normale Erfolgsantwort ohne jeden Hinweis auf den Umfang.

Ursache ist `markdownSection` (`cmd/flow-mcp/write.go:186-193`): der Abschnitt
endet erst bei einer Überschrift mit `gotLevel <= level`. Bei `level == 1`
erfüllt keine H2/H3 diese Bedingung, `end` bleibt `len(lines)`.

Am 2026-07-28 hat das im Straßenfuchs-Kontext ein Code-Review mit 51 Findings in
vier Kapiteln (~14 KB) auf H1 plus Einleitung reduziert. Das Dokument war zu dem
Zeitpunkt die einzige Kopie; gerettet hat es allein, dass der Agent den Volltext
noch im Kontext hielt.

**Die Baum-Semantik selbst bleibt.** Dass ein Abschnitt seine Unterabschnitte
einschließt, ist als Markdown-Semantik vertretbar und wird von bestehenden
Aufrufern zu Recht erwartet. Der Bug ist, dass ihre destruktivste Ausprägung
weder dokumentiert noch abgesichert noch am Ergebnis erkennbar ist. Genau diese
drei Lücken schließt der Slice.

### Erfolgskriterien

1. Die Antwort von `flow_patch_doc` und `flow_update_doc` enthält ein
   Größendelta in Bytes und Zeilen, vorher und nachher.
2. Ein Schreibvorgang, der mehr als die Hälfte des Bodys **und** mehr als 1 KB
   entfernt, wird ohne `allowShrink=true` mit einer Meldung abgelehnt, die den
   Umfang beziffert und den Ausweg nennt.
3. Die Tool-Description dokumentiert die Subtree-Semantik und den Sonderfall der
   obersten Überschrift.
4. `make ci` bleibt grün (Coverage-Gate 75 %).

### Nicht-Ziele (bewusst YAGNI)

- **Die Abbruchbedingung in `markdownSection` ändern.** „Nächste Überschrift
  beliebiger Ebene" würde legitimes Ersetzen ganzer Teilbäume brechen und wäre
  eine stille Verhaltensänderung für bestehende Aufrufer.
- **Maßnahme D** aus dem Bug-Doc (H1-Sonderfall grundsätzlich ohne Flag
  ablehnen). Der Schrumpf-Guard deckt den realen Fall ab; D würde zusätzlich nur
  den Fall fangen, in dem ein *großer* Ersatz-Body Kapitel vernichtet, ohne die
  Gesamtgröße zu senken. Bei Bedarf nachrüstbar.
- **Revisionen/Papierkorb.** Eigener FR, eigener Slice.

---

## 2. Platzierung: warum flow-mcp und nicht der Server

flow-mcp rechnet den neuen Body **lokal** aus (`patchMarkdown`) und schickt
einen *vollen* Body an `PatchDocument`. Der Server sieht einen gewöhnlichen
Ganzkörper-Schreibvorgang, ununterscheidbar von einem WebUI-Save. Ein
serverseitiger Schrumpf-Guard würde deshalb bei jedem legitimen WebUI-Speichern
anschlagen, bei dem jemand Text kürzt.

Dazu kommt: `replace_section` existiert nirgends sonst. `markdownSection` und
`patchMarkdown` stehen ausschließlich in `cmd/flow-mcp/{write,tools_write,server}.go`;
die CLI (`cmd/flow`) hat kein Patch-Verb. Das AGENTS.md-Prinzip „generische
Features gehören in alle Hosts" greift hier nicht — es gibt nur einen Host.

**Guard und Delta leben daher vollständig in `cmd/flow-mcp`.** `internal/` wird
nicht angefasst.

---

## 3. Komponenten

### 3.1 `bodyDelta` (neu, `write.go`)

```go
type bodyDelta struct {
    BytesBefore int
    BytesAfter  int
    LinesBefore int
    LinesAfter  int
}

func newBodyDelta(before, after string) bodyDelta
```

Zeilen werden als `strings.Count(strings.TrimSuffix(s, "\n"), "\n") + 1`
gezählt, für den leeren String als `0`. Zwei Sonderfälle, beide bewusst:

- **Ein abschließender Newline erzeugt keine Extrazeile.**
  `replaceMarkdownSection` (`write.go:205`) schließt **immer** mit `\n` ab; die
  naive Formel `Count(s, "\n") + 1` zählte deshalb bei praktisch jedem Body eine
  Zeile zu viel.
- **Ein leerer Body hat 0 Zeilen**, obwohl `strings.Split` für `""` ein Ergebnis
  der Länge 1 liefert. `linesAfter: 0` ist genau das Signal, das den
  Totalverlust anzeigt.

### 3.2 Antwortfelder (`documentWriteResult`)

Vier neue Felder, als **`*int`**:

```go
BytesBefore *int `json:"bytesBefore,omitempty"`
BytesAfter  *int `json:"bytesAfter,omitempty"`
LinesBefore *int `json:"linesBefore,omitempty"`
LinesAfter  *int `json:"linesAfter,omitempty"`
```

Pointer statt `int` mit `omitempty` ist hier kein Stilpunkt, sondern
funktional: `omitempty` auf einem `int` verschluckt die `0` — also ausgerechnet
`"bytesAfter": 0`, den Totalverlust, den die Felder sichtbar machen sollen.

`documentResult` bekommt einen Parameter `delta *bodyDelta`. Die vier Call-Sites
in `tools_write.go`:

| Call-Site | Aktion | Delta |
|---|---|---|
| `:66` | `created` | `nil` — kein Vorher |
| `:112` | `updated` | gesetzt, **wenn `in.Body != nil`** |
| `:160` | `patched` | immer gesetzt |
| `:225` | `moved` | `nil` — Body unangetastet |

### 3.3 `checkShrink` (neu, `write.go`)

```go
const (
    shrinkRatio    = 0.5  // Anteil des Bodys, ab dem der Guard greift
    shrinkMinBytes = 1024 // absolute Untergrenze
)

func checkShrink(action string, d bodyDelta, allow bool) error
```

`action` ist `"patch"` oder `"update"`. Es prägt das erste Wort der Meldung und
entscheidet, ob `flow_update_doc` als Ausweg genannt wird (§3.4) — bei
`flow_update_doc` selbst wäre der Verweis zirkulär.

Greift, wenn **beide** Bedingungen zutreffen und `allow` false ist:

- `d.BytesBefore - d.BytesAfter > shrinkMinBytes`
- `float64(d.BytesBefore-d.BytesAfter) / float64(d.BytesBefore) > shrinkRatio`

Die absolute Untergrenze verhindert Fehlalarme bei kleinen Dokumenten, wo ein
legitimer Abschnittstausch schnell die halbe Datei ist. Ohne sie gewöhnen sich
Agenten an `allowShrink=true` und setzen es reflexhaft — der Guard wäre dann
schlimmer als keiner.

Randfälle:

- `BytesBefore == 0` → nie (kein Nenner, nichts zu verlieren).
- Wachstum oder Gleichstand → nie (Differenz ≤ 0 scheitert schon an der
  Untergrenze).

Der reale Fall: 14 204 → 1 021 Bytes, also 13 183 entfernt (92,8 %). Beide
Bedingungen erfüllt, der Guard greift.

### 3.4 Fehlermeldung

Nennt Umfang **und** Ausweg, weil ein Agent sonst nur „abgelehnt" liest:

```
patch would remove 13183 of 14204 bytes (93%), 312 lines to 25.
Pass allowShrink=true if intended, or use flow_update_doc with the full body.
```

Prozent gerundet auf ganze Zahlen. Bei `flow_update_doc` lautet das erste Wort
`update` statt `patch` und der Ausweg-Satz nennt nur `allowShrink=true`.

### 3.5 Neues Eingabefeld

Auf `patchDocIn` **und** `updateDocIn`:

```go
AllowShrink bool `json:"allowShrink,omitempty" jsonschema:"required (true) to apply a write that removes more than half the document body"`
```

Bewusst **getrennt von `confirm`**: jeder Patch auf eine human-owned Note
(daily/project/free) trägt heute schon zwingend `confirm=true`. Würde derselbe
Wert auch den Schrumpf-Guard entschärfen, wäre der Guard ausgerechnet auf
Soennes eigenen Notizen wirkungslos — dort, wo die unersetzlichsten Dokumente
liegen.

### 3.6 Tool-Description (`server.go:93`)

Ergänzt um die Semantik und den Ausweg, sinngemäß:

> A section spans its subsections, so replacing the topmost heading replaces the
> whole document. To edit the intro, target the first subheading instead, or use
> `flow_update_doc` with the full body. Writes that remove more than half the
> body require `allowShrink=true`. Returns … plus `bytesBefore`/`bytesAfter` and
> `linesBefore`/`linesAfter`.

`flow_update_doc` (`server.go`) bekommt den `allowShrink`- und Delta-Hinweis
entsprechend.

---

## 4. Datenfluss

`patchDoc` (`tools_write.go:133-167`), Reihenfolge unverändert bis auf den neuen
Schritt:

```
GetDocument
  → guardMutation(cur, in.Confirm)          bestehend
  → patchMarkdown(cur.Body, in)             bestehend
  → newBodyDelta(cur.Body, body)            NEU
  → checkShrink("patch", delta, in.AllowShrink)  NEU  → errGuard
  → expectedUpdatedAt / PatchDocument       bestehend (CAS)
  → documentResult(ctx, "patched", d, &delta)
```

Der Guard sitzt **nach** `patchMarkdown`, damit er nichts ablehnt, was
`patchMarkdown` ohnehin als Fehler abweist (`section not found`, `ambiguous`),
und **vor** dem Netzwerkaufruf, damit ein abgelehnter Schreibvorgang gar nicht
erst beim Server ankommt.

`updateDoc` (`tools_write.go:84-119`) analog, aber nur wenn `in.Body != nil`;
ein Update ohne Body lässt den Text unangetastet und hat kein Delta.

Beide Fehler als `errGuard{}`, damit sie wie die bestehenden Guards durch
`h.resultErr` laufen und als Tool-Fehler statt als Transportfehler ankommen.

---

## 5. Tests

Unit (`write_test.go`) — die ersten beiden nageln Semantik fest, die heute
**kein** Test abdeckt (`write_test.go:22-26` ersetzt `## Notes`, die letzte H2
ohne Unterabschnitte; dort sind „Subtree" und „bis zur nächsten Überschrift"
nicht unterscheidbar):

1. `replace_section` auf eine H2 **mit** H3-Unterabschnitten ersetzt den
   gesamten Teilbaum, und der nachfolgende H2-Abschnitt bleibt stehen.
2. `replace_section` auf die H1 eines mehrkapitligen Dokuments — Regression für
   genau diesen Vorfall.
3. `checkShrink` greift bei 93 % / 13 KB.
4. `checkShrink` lässt 54 % / 332 B durch (absolute Untergrenze).
5. `checkShrink` lässt Wachstum und `BytesBefore == 0` durch.
6. `newBodyDelta` zählt Zeilen korrekt, inklusive leerer String.

Loopback (`loopback_write_test.go`):

7. `flow_patch_doc` liefert die vier Delta-Felder in der Antwort.
8. Ein Patch, der den Body leert, liefert `"bytesAfter": 0` — die Regression
   gegen die `omitempty`-Falle aus §3.2.
9. Der Guard lehnt ab; die Meldung nennt Bytes und `allowShrink`.
10. `allowShrink=true` lässt denselben Aufruf durch.
11. `flow_update_doc` ohne `body` liefert kein Delta und keinen Guard.

---

## 6. Risiken

- **Verhaltensänderung für bestehende Aufrufer.** Ein Patch, der heute
  durchgeht, kann künftig abgelehnt werden. Das ist beabsichtigt und der ganze
  Zweck; die Meldung nennt das Flag, mit dem er weiterläuft.
- **Schwellwert-Kalibrierung.** 50 % / 1 KB ist begründet, aber nicht empirisch
  gemessen. Beide Werte stehen als Konstanten beieinander und lassen sich in
  einem Einzeiler nachziehen.
- **Der Guard schützt nicht gegen alles.** Ein Patch, der Kapitel durch
  gleich viel neuen Text ersetzt, bleibt unbemerkt. Dagegen hilft nur das
  Delta (das der Agent lesen muss) und, endgültig, der Revisionen-FR.
