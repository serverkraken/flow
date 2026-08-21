---
type: spec
tags: [editor, spike]
---

# Härtetest für den Roundtrip

Ein Absatz mit einem [[wikilink]] und einem [[ziel|Anzeigetext]] darin.
Dazu eine Fußnote[^1] und ein ~~durchgestrichenes~~ Wort.

> [!NOTE]
> Ein Callout mit **Fett** und `Code` darin.
> Zweite Zeile des Callouts.

> [!WARNING]
> Ein zweiter Callout-Typ.

- [ ] offene Aufgabe
- [x] erledigte Aufgabe
- normaler Punkt mit [[verweis]]

| Spalte A | Spalte B |
|----------|---------:|
| links    |    rechts |
| `code`   | [[link]]  |

```go
func main() {
	fmt.Println("Chroma kennt go")
}
```

```mermaid
graph TD
  A --> B
```

Ein eingebettetes Artefakt: ![[architektur.png]]

Und ein normales Bild: ![alt](https://example.test/bild.png)

Harte Zeilenumbrüche  
mit zwei Leerzeichen am Ende.

1. erstens
2. zweitens
   - verschachtelt

[^1]: Die Fußnote selbst.
