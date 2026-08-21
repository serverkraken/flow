// roundtrip.mjs — Markdown → Milkdown (echter Editor, jsdom) → Markdown.
// Die Frage ist nicht "klappt es", sondern WELCHE Syntax überlebt unverändert.
import { readFileSync, writeFileSync } from 'node:fs';
import { JSDOM } from 'jsdom';

const dom = new JSDOM('<!doctype html><div id="app"></div>', { pretendToBeVisual: true });
for (const k of ['window','document','navigator','HTMLElement','Node','Element','MutationObserver','getComputedStyle','Range','Selection','DOMParser','requestAnimationFrame','cancelAnimationFrame','customElements','KeyboardEvent','MouseEvent','Event','InputEvent','DragEvent','ClipboardEvent','DocumentFragment','Text','HTMLDivElement','HTMLSpanElement','HTMLTableElement','HTMLTableCellElement','HTMLImageElement','HTMLAnchorElement','HTMLParagraphElement','HTMLHeadingElement','HTMLPreElement','HTMLUListElement','HTMLOListElement','HTMLLIElement','HTMLBRElement','HTMLHRElement','HTMLInputElement','HTMLLabelElement','HTMLButtonElement','SVGElement','CSSStyleDeclaration']) {
  if (!(k in globalThis) && k in dom.window) globalThis[k] = dom.window[k];
}
if (!dom.window.Range.prototype.getClientRects) {
  dom.window.Range.prototype.getClientRects = () => ({ length: 0, item: () => null, [Symbol.iterator]: function*(){} });
  dom.window.Range.prototype.getBoundingClientRect = () => ({ x:0,y:0,top:0,left:0,bottom:0,right:0,width:0,height:0 });
}
dom.window.Element.prototype.getClientRects ||= () => ({ length: 0, item: () => null, [Symbol.iterator]: function*(){} });
dom.window.Element.prototype.getBoundingClientRect ||= () => ({ x:0,y:0,top:0,left:0,bottom:0,right:0,width:0,height:0 });
dom.window.Element.prototype.scrollIntoView ||= () => {};
// Milkdown/ctx ruft addEventListener & Co. als GLOBALE auf (wie im Browser,
// wo window der globale Scope ist). jsdom hängt sie nur ans window.
for (const k of ['addEventListener','removeEventListener','dispatchEvent','setTimeout','clearTimeout','CustomEvent','queueMicrotask']) {
  if (!(k in globalThis) && k in dom.window) globalThis[k] = dom.window[k].bind ? dom.window[k].bind(dom.window) : dom.window[k];
}
globalThis.addEventListener = dom.window.addEventListener.bind(dom.window);
globalThis.removeEventListener = dom.window.removeEventListener.bind(dom.window);
globalThis.dispatchEvent = dom.window.dispatchEvent.bind(dom.window);
globalThis.CustomEvent = dom.window.CustomEvent;

const { Editor, rootCtx, defaultValueCtx, editorViewCtx, serializerCtx, remarkStringifyOptionsCtx } = await import('@milkdown/kit/core');
const { commonmark } = await import('@milkdown/kit/preset/commonmark');
const { gfm, remarkGFMPlugin } = await import('@milkdown/kit/preset/gfm');
const { flowSyntax } = await import('./flow-syntax.mjs');

const file = process.argv[2] ?? 'fixtures/hart.md';
const input = readFileSync(file, 'utf8');

const editor = await Editor.make()
  .config(ctx => {
    ctx.set(rootCtx, dom.window.document.getElementById('app'));
    ctx.set(defaultValueCtx, input);
    // Tabellen-Stil ist eine remark-gfm-Option, keine Stringify-Option.
    ctx.set(remarkGFMPlugin.options.key, { tablePipeAlign: false, tableCellPadding: true });
    // Schreibweise an den Bestand angleichen: '-' statt '*', harte Umbrüche
    // als zwei Leerzeichen statt '\\'. Das sind Serializer-Optionen, keine
    // Modell-Verluste.
    ctx.update(remarkStringifyOptionsCtx, o => ({
      ...o, bullet: '-', listItemIndent: 'one', emphasis: '*', strong: '*', rule: '-',
      // Tabellen nicht auf gleiche Breite ausrichten — sonst ändert jede Zelle jede Zeile.
      tablePipeAlign: false, tableCellPadding: true,
      // Harter Umbruch als zwei Leerzeichen, wie der Bestand ihn schreibt —
      // remarks Standard ist '\\', weil Leerzeichen unsichtbar sind.
      handlers: { ...(o.handlers ?? {}), break: () => '  \n' },
    }));
  })
  .use(commonmark).use(gfm).use(flowSyntax)
  .create();

const types = new Set();
editor.action(ctx => { ctx.get(editorViewCtx).state.doc.descendants(n => { types.add(n.type.name); }); });
console.log('Knotentypen im Dokument:', [...types].filter(t => t.startsWith('flow_')).join(', ') || '(keine flow_-Knoten!)');
const output = editor.action(ctx => {
  const view = ctx.get(editorViewCtx);
  const serializer = ctx.get(serializerCtx);
  return serializer(view.state.doc);
});
writeFileSync(file.replace(/\.md$/, '.out.md'), output);

const norm = s => s.replace(/\s+$/gm, '').replace(/\n{3,}/g, '\n\n').trim();
const same = norm(input) === norm(output);

// Je Syntax: taucht die Probe im Output noch wörtlich auf?
const probes = {
  'Frontmatter (---)':        /^---\ntype: spec/m,
  '[[wikilink]]':             /\[\[wikilink\]\]/,
  '[[ziel|Anzeigetext]]':     /\[\[ziel\|Anzeigetext\]\]/,
  'Fußnote [^1] + Definition':/\[\^1\][\s\S]*\[\^1\]: Die Fußnote/,
  '~~Strikethrough~~':        /~~durchgestrichenes~~/,
  '> [!NOTE] Callout':        /> \[!NOTE\]\s*\n?> ?Ein Callout/,
  '> [!WARNING] Callout':     /> \[!WARNING\]/,
  'Task-Liste [ ] / [x]':     /- \[ \] offene[\s\S]*- \[x\] erledigte/,
  'Tabelle mit Ausrichtung':  /\|\s*-+\s*\|\s*-+:\s*\|/,
  '```go Fence':              /```go\n/,
  '```mermaid Fence':         /```mermaid\n/,
  'Tab-Einrückung im Code':   /\n\tfmt\.Println/,
  '![[artefakt]] Einbettung': /!\[\[architektur\.png\]\]/,
  'Normales Bild':            /!\[alt\]\(https:\/\/example\.test\/bild\.png\)/,
  'Harter Umbruch (2 Leerz.)':/Zeilenumbrüche  \n/,
  'Verschachtelte Liste':     /2\. zweitens\n {2,3}- verschachtelt/,
};
console.log(`\n${same ? '✓' : '✗'} Gesamt byte-gleich (normalisiert): ${same}`);
// Zweite Wahrheit: gleicher mdast-Baum = nichts verloren, nur anders geschrieben.
const { unified } = await import('unified');
const remarkParse = (await import('remark-parse')).default;
const remarkGfm = (await import('remark-gfm')).default;
const remarkFm = (await import('remark-frontmatter')).default;
const strip = n => { if (n && typeof n === 'object') { delete n.position; for (const k of Object.keys(n)) strip(n[k]); } return n; };
const ast = t => JSON.stringify(strip(unified().use(remarkParse).use(remarkGfm).use(remarkFm, 'yaml').parse(t)));
const semSame = ast(input) === ast(output);
console.log(`${semSame ? '✓' : '✗'} Semantisch gleich (mdast ohne Positionen): ${semSame}\n`);
console.log('Syntax'.padEnd(28), 'überlebt');
for (const [name, re] of Object.entries(probes)) {
  const inOk = re.test(input), outOk = re.test(output);
  console.log(name.padEnd(28), inOk ? (outOk ? '✓' : '✗  VERLOREN/VERÄNDERT') : '(nicht in Fixture)');
}
