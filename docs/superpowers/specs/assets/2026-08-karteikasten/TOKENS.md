# TOKENS — Karteikasten-Design → `web/tailwind.css`

Alle Werte sind aus dem Elementkatalog (Kapitel 03) des Konzeptdokuments.
Die linke Spalte ist der Name im Design, die rechte der bestehende Token-Name in
`web/tailwind.css` — wo einer existiert, wird **der Wert ersetzt, nicht der Name**.

## Papier — fünf Flächen, klare Rangfolge

| Design | Hex | RGB-Triplet | Token | Verwendung |
| --- | --- | --- | --- | --- |
| Tisch | `#E7E5DF` | `231 229 223` | *(neu)* `--desk` | außerhalb der App (Body hinter dem Rahmen) |
| Kasten | `#F1EDE2` | `241 237 226` | `--panel`, `--sunken` | Schiene und Liste |
| Lesesaal | `#F8F6F0` | `248 246 240` | `--canvas`, `--paper` | Lesen und Schreiben |
| Beleg | `#FDFCF7` | `253 252 247` | `--sheet` | Kacheln, Tabellenköpfe |
| Blatt | `#FFFFFF` | `255 255 255` | `--surface` | Codeblock, Eingabefeld |

Zwei-Flächen-Regel: pro Screen höchstens zwei dieser Flächen groß nebeneinander.

## Linien — fünf Stärken statt Schatten

| Design | Hex | Token | Verwendung |
| --- | --- | --- | --- |
| Rahmen | `#D9D4C6` | `--hair2` | Außenkante, Knopfrand |
| Spalte | `#E0DACB` | `--hairp` | Spaltentrenner |
| Abschnitt | `#E6E1D4` | `--line2` | Über- und Unterlinien |
| Zeile | `#EDE9DC` | `--line` | Listenzeilen, Balkenbett |
| Leerstelle | `#C9C3B2` | *(neu)* `--hair3` | **gestrichelt** = fehlt noch |

Schatten gibt es nur an zwei Stellen: Mockup-Rahmen und Dialog/Overlay
(`0 18px 50px -24px rgba(28,27,24,.25)` bzw. `0 28px 70px -28px rgba(28,27,24,.5)`).
Panels werfen `24px 0 60px -30px rgba(28,27,24,.35)` nach rechts. Sonst: Linien.

## Tinte — fünf Stufen, streng hierarchisch

| Design | Hex | Token |
| --- | --- | --- |
| Titel | `#26241E` | `--ink` |
| Lesetext | `#33312A` | `--body` |
| Sekundär | `#5C5748` | *(neu)* `--body2` |
| Meta | `#8A8578` | `--muted`, `--meta` |
| Beschriftung | `#B5AFA0` | `--faint` |

## Ebenenfarben — die drei Register

| Ebene | Ton | Wash | Dunkelmodus |
| --- | --- | --- | --- |
| Engagement | `#8A5A18` | `#F8ECD4` | `#D9A860` |
| Vorhaben | `#8A4F7A` | `#F3E6F0` | unverändert |
| Repo | `#4A6B8A` | `#E7EBFA` | `#7FA5F5` |

Der **3px-Ebenenstreifen** sitzt oben über dem Arbeitsbereich und beginnt erst
bei `left: 264px` (Tablet: `left: 76px`) — die Schiene bleibt neutral.

## Kartentypen und Bereiche

| Typ / Bereich | Ton | Wash | Verwendung |
| --- | --- | --- | --- |
| Plan | `#7A4FD0` | `#F1EAFE` | Vorhaben, Ausstattung |
| Spec | `#0B8A7B` | `#E2F5F2` | Festlegungen |
| Notiz | `#3D5EDB` | `#E7EBFA` | Notiz, Instruktion, Bibliothek |
| Erinnerung | `#B4452F` | `#F6E8E4` | Erinnerung, Fehler, „nie" |
| Kontext | `#B8720F` | `#F8ECD4` | **Akzent**, Start, Puls, Links |
| Zeit · live | `#0F8A46` | `#E3F0E6` | laufende Uhr, Soll erfüllt |
| Tagebuch | `#5A7A2E` | `#E3F0E6` | Tagesnotizen |

Zuordnung auf bestehende Tokens: `--accent` = `#B8720F`, `--accent-wash` =
`#F8ECD4`, `--accent-deep` = `#8A5A18`, `--live` = `#0F8A46`, `--live-wash` =
`#E3F0E6`, `--warn` = `#B8720F`, `--red` = `#B4452F`, `--purple` = `#7A4FD0`,
`--teal`/`--cyan` = `#0B8A7B`, `--blue` = `#3D5EDB`.

Die Auswahl in Listen zeigt sich als **3px-Kante links** in der Typfarbe plus
Wash als Zeilenhintergrund — nie als Rahmen, nie als Radius.

Ergänzende Flächen: `#F2F8F3` (Timer-Karte, Rand `#D8E8DC`), `#F8F1E2`
(Warnband, Rand `#E8D9B8`), `#F5EAF2` (Vorhaben-Auswahl im Panel).

## Code-Syntax — drei Farben, mehr nicht

Schlüsselwort `#D9480F` · Zahl `#1864AB` · String `#0F8A46` · Kommentar `#8A8578`
· Text `#33312A` auf `#FFFFFF`. Das ist der Ziel-Zustand für
`internal/adapter/webui/gen/chromacss` — Chroma-Theme entsprechend beschneiden.

## Dunkelmodus — gespiegelte Tokens, gleiche Struktur

| Fläche | Hell | Dunkel |
| --- | --- | --- |
| Lesesaal | `#F8F6F0` | `#1C1B17` |
| Kasten | `#F1EDE2` | `#24231D` |
| Beleg / Kachel | `#FDFCF7` | `#302D25` |
| Linie | `#EDE9DC` | `#3A3730` |
| Titel-Tinte | `#26241E` | `#F1EDE2` |
| Lesetext | `#33312A` | `#C9C3B2` |
| Akzent | `#B8720F` | `#D99A2B` |
| Live-Grün | `#0F8A46` | `#46C97D` |

Regel: Chroma bleibt, Lightness kippt. Ebenenfarben bleiben identisch. Meta-
Tinten wandern eine Stufe nach unten — `#8A8578` wird Text, `#5C5748` wird
Beschriftung.

## Schrift

Zwei Familien, beide lokal als variable woff2 (liegen in `design/fonts/`, gehören
nach `internal/adapter/webui/static/fonts/`):

- **Schibsted Grotesk**, 400–900 — alles Geschriebene und Gelesene. Gewichte:
  500 Navigation, 600/650 Betonung und Knöpfe, 700 Beschriftungen, 750 alle
  Überschriften. Negative Laufweite ab 19px `−.015em`, ab 26px `−.02em`.
- **JetBrains Mono**, 400–800 — alles Gemessene und Adressierte: Uhrzeiten,
  Zähler, Pfade, Branches, Tastenkürzel, Code, Screen-Beschriftungen. Immer
  `font-variant-numeric: tabular-nums`.

### Typo-Leiter — zehn Stufen, keine Zwischengrößen

| px / Gewicht | Verwendung |
| --- | --- |
| 30 / 750 / −.02em / 1.15 | Karten- und Seitentitel |
| 26 / 750 / −.02em | Bereichstitel, Editor-Titelzeile |
| 20 / 750 / −.015em | Kastenkopf, Artikelabschnitt (H2) |
| 19 / 750 | Unterabschnitt (H3) |
| 16.5–18 / 400 / 1.82 | Lesetext (16.5 Karte, 18 README/Langtext) |
| 14.5 / 650 / 1.35 | Titel einer Listenzeile |
| 14 / 500 → 700 aktiv | Navigationszeile in der Schiene |
| 12–12.5 / 400–600 | Meta, Auszug, Bildunterschrift, Knopfschrift |
| 10.5 / 700 / .14em / caps | Beschriftung über einem Block |
| 10 / 700 / .1em / caps / Ton | Typ-Marke in der Zeile |

## Maß und Raster

| Maß | Wert |
| --- | --- |
| Dreispalter | `264px` Schiene · `372px` Kasten · `1fr` Lesesaal |
| Lesesaal-Polster | `30px` oben/unten, `56px` seitlich |
| Satzbreite | volle Spaltenbreite (Soenne, 22.08.2026 — vorher `660–720px`); Tabellen und Code scrollen in sich, optional `236px` Randspalte |
| Listenzeile | `11–13px` senkrecht, `16px` waagerecht, Auswahlkante `3px` |
| Schienen-Einzug je Ebene | `+15px` |
| Ebenenstreifen | `3px`, ab `left:264px` |
| Radien | `0` — überall. Ausnahmen: Telefonrahmen `34px`, Tabletrahmen `22px`, Live-Punkt `999px` |
| Dialog | `480px` breit, mittig |
| Suche-Overlay | `660px` breit, `86px` von oben, Schleier `rgba(28,27,24,.42)` |
| Trefffläche Berührung | min. `44px` |
| Umbrüche | `768px` und `1200px` |

## Bewegung

| Vorgang | Dauer |
| --- | --- |
| Auswahl, Hover-Wash | `0ms` — sofort |
| Auf-/Zuklappen in der Schiene | `140ms` |
| Panel, Overlay-Schleier | `180ms` |
| Dialog | `200ms` |
| Ladeplatzhalter | erst nach `400ms`, versetzt pulsierend |

Kurve überall `cubic-bezier(.2,0,.3,1)`. **Nicht** animiert: Uhr, Live-Punkt,
Zähler — die springen.

## Zeichen mit Bedeutung (keine Emoji)

`●` Live/laufend · `◆` Vorhaben · `⬡` Repo · `▶`/`▾`/`▴` auf-/zugeklappt und
Sortierrichtung · `■` Register-Marke · `✓` gesetzt · `⌕` Suche · `×` Filter
entfernen · `‹` zurück. Alles andere ist SVG aus
`internal/adapter/webui/icons/`.
