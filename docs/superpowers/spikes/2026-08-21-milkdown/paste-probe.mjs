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

const { Editor, rootCtx, defaultValueCtx, editorViewCtx, serializerCtx, pasteRulesCtx } = await import('@milkdown/kit/core');
const { commonmark } = await import('@milkdown/kit/preset/commonmark');
const { gfm } = await import('@milkdown/kit/preset/gfm');
const { Slice, Fragment } = await import('@milkdown/kit/prose/model');
const { flowSyntax } = await import('./flow-syntax.mjs');
const editor = await Editor.make().config(ctx => { ctx.set(rootCtx, dom.window.document.getElementById('app')); ctx.set(defaultValueCtx, 'Start.\n'); }).use(commonmark).use(gfm).use(flowSyntax).create();
const out = editor.action(ctx => {
  const view = ctx.get(editorViewCtx);
  const schema = view.state.schema;
  const pasted = 'Eingefügt: [[ziel|Anzeige]] und ![[bild.png]].';
  // So baut ProseMirror einen Klartext-Slice beim Einfügen:
  const plain = new Slice(Fragment.from(schema.nodes.paragraph.create(null, schema.text(pasted))), 1, 1);
  let slice = plain;
  for (const rule of ctx.get(pasteRulesCtx)) slice = rule.run(slice, view, true);
  const types = new Set(); slice.content.descendants(n => { types.add(n.type.name); });
  view.dispatch(view.state.tr.replaceSelection(slice));
  return { nodeTypes: [...types].filter(t => t.startsWith('flow_')), markdown: ctx.get(serializerCtx)(view.state.doc) };
});
console.log('flow-Knoten nach Paste:', out.nodeTypes.join(', ') || '(keine!)');
console.log('Markdown danach:', JSON.stringify(out.markdown));
console.log(out.markdown.includes('[[ziel|Anzeige]]') && out.markdown.includes('![[bild.png]]') && !out.markdown.includes('\\[') ? '✓ Paste verlustfrei' : '✗ Paste escaped oder verloren');
