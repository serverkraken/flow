// flow-syntax.mjs — die drei Eigenheiten des flow-Servers als Milkdown-Plugins.
// Ziel: der Editor VERSTEHT [[ziel|anzeige]], ![[slug]] und > [!NOTE], statt
// sie als Text zu escapen. Parser (remark → ProseMirror) und Serializer
// (ProseMirror → remark) je Knoten; ein $remark-Plugin zerlegt den Text vorab.
import { $node, $remark, $inputRule, $pasteRule, markdownToSlice } from '@milkdown/kit/utils';
import { InputRule } from '@milkdown/kit/prose/inputrules';
import { findAndReplace } from 'mdast-util-find-and-replace';
import remarkFrontmatter from 'remark-frontmatter';

// ── Frontmatter: nur erhalten, nie rendern ─────────────────────────────────
export const frontmatterRemark = $remark('flowFrontmatter', () => remarkFrontmatter, 'yaml');
export const frontmatterNode = $node('flow_frontmatter', () => ({
  group: 'block', atom: true, selectable: false, attrs: { value: { default: '' } },
  parseDOM: [{ tag: 'div[data-frontmatter]' }],
  toDOM: n => ['div', { 'data-frontmatter': '', style: 'display:none' }, n.attrs.value],
  parseMarkdown: { match: n => n.type === 'yaml', runner: (s, n, t) => s.addNode(t, { value: n.value }) },
  toMarkdown:    { match: n => n.type.name === 'flow_frontmatter', runner: (s, n) => s.addNode('yaml', undefined, n.attrs.value) },
}));

// ── [[ziel]] / [[ziel|anzeige]] und ![[slug]] als eigene mdast-Knoten ─────
const WIKI = /(!?)\[\[([^\[\]\n|]+)(?:\|([^\[\]\n]+))?\]\]/g;
export const wikiRemark = $remark('flowWiki', () => () => (tree) => {
  findAndReplace(tree, [[WIKI, (_, bang, target, display) =>
    ({ type: bang ? 'flowEmbed' : 'flowWikilink', target: target.trim(), display: display?.trim() ?? '' })]]);
});
export const wikilinkNode = $node('flow_wikilink', () => ({
  group: 'inline', inline: true, atom: true,
  attrs: { target: { default: '' }, display: { default: '' } },
  parseDOM: [{ tag: 'span[data-wikilink]', getAttrs: el => ({ target: el.dataset.wikilink, display: el.dataset.display ?? '' }) }],
  toDOM: n => ['span', { 'data-wikilink': n.attrs.target, 'data-display': n.attrs.display, class: 'editor-wikilink' }, n.attrs.display || n.attrs.target],
  parseMarkdown: { match: n => n.type === 'flowWikilink', runner: (s, n, t) => s.addNode(t, { target: n.target, display: n.display }) },
  // 'html' statt 'text': remark-stringify escaped Text ("\\[\\[x]]"), rohe
  // Knoten nicht. Die Quelle bleibt damit exakt das, was der Server parst.
  toMarkdown:    { match: n => n.type.name === 'flow_wikilink', runner: (s, n) =>
    s.addNode('html', undefined, n.attrs.display ? `[[${n.attrs.target}|${n.attrs.display}]]` : `[[${n.attrs.target}]]`) },
}));
export const embedNode = $node('flow_embed', () => ({
  group: 'inline', inline: true, atom: true, attrs: { slug: { default: '' } },
  parseDOM: [{ tag: 'span[data-embed]', getAttrs: el => ({ slug: el.dataset.embed }) }],
  toDOM: n => ['span', { 'data-embed': n.attrs.slug, class: 'editor-embed' }, `![[${n.attrs.slug}]]`],
  parseMarkdown: { match: n => n.type === 'flowEmbed', runner: (s, n, t) => s.addNode(t, { slug: n.target }) },
  toMarkdown:    { match: n => n.type.name === 'flow_embed', runner: (s, n) => s.addNode('html', undefined, `![[${n.attrs.slug}]]`) },
}));

// ── > [!NOTE] Callout: Blockquote, dessen erste Zeile der Typ ist ─────────
// Das Escaping von "[!NOTE]" ist ein Serializer-Artefakt (remark-stringify
// schützt "[" vor Link-Verwechslung). Wir heben die Markierung in ein Attribut
// und schreiben sie beim Serialisieren roh zurück.
export const calloutRemark = $remark('flowCallout', () => () => (tree) => {
  const walk = node => {
    if (!node.children) return;
    node.children.forEach(walk);
    if (node.type !== 'blockquote') return;
    const first = node.children[0];
    const t = first?.children?.[0];
    if (first?.type !== 'paragraph' || t?.type !== 'text') return;
    const m = /^\[!([A-Za-z]+)\]\n?/.exec(t.value);
    if (!m) return;
    node.type = 'flowCallout'; node.kind = m[1];
    t.value = t.value.slice(m[0].length);
    if (!t.value) first.children.shift();
    if (!first.children.length) node.children.shift();
  };
  walk(tree);
});
export const calloutNode = $node('flow_callout', () => ({
  group: 'block', content: 'block+', defining: true, attrs: { kind: { default: 'NOTE' } },
  parseDOM: [{ tag: 'blockquote[data-callout]', getAttrs: el => ({ kind: el.dataset.callout }) }],
  toDOM: n => ['blockquote', { 'data-callout': n.attrs.kind, class: 'callout callout-' + n.attrs.kind.toLowerCase() }, 0],
  parseMarkdown: { match: n => n.type === 'flowCallout', runner: (s, n, t) => { s.openNode(t, { kind: n.kind }); s.next(n.children); s.closeNode(); } },
  toMarkdown:    { match: n => n.type.name === 'flow_callout', runner: (s, n) => {
    // Der Marker muss IN den ersten Absatz, sonst setzt remark zwischen
    // Marker-Block und Text einen Absatzabstand ("> [!NOTE]\n>\n> Text").
    s.openNode('blockquote');
    let first = true;
    n.content.forEach(child => {
      if (first && child.type.name === 'paragraph') {
        s.openNode('paragraph');
        s.addNode('html', undefined, `[!${n.attrs.kind}]`); // roh, kein Escaping
        s.next(child.content);
        s.closeNode();
        first = false;
      } else {
        if (first) { s.openNode('paragraph'); s.addNode('html', undefined, `[!${n.attrs.kind}]`); s.closeNode(); first = false; }
        s.next(child);
      }
    });
    s.closeNode(); } },
}));

// Frontmatter vorerst draußen — erst Wikilink/Embed/Callout, das ist die Frage, die zählt.
// ── Input-Rules: getippt, nicht nur geladen ───────────────────────────────
// Die remark-Plugins wirken beim PARSEN. Was man tippt, geht nie durch den
// Parser — es landet als Text im Dokument, und den escaped der Serializer
// pflichtgemäß ("\\[\\[x]]"). Die Regel verwandelt "[[ziel|anzeige]]" in dem
// Moment, in dem die schließende Klammer fällt, in den Knoten — wie "**x**"
// zu Fett wird.
export const wikilinkInputRule = $inputRule(ctx => new InputRule(
  /(?<!!)\[\[([^\[\]\n|]+)(?:\|([^\[\]\n]+))?\]\]$/,
  (state, match, start, end) => state.tr.replaceWith(start, end,
    wikilinkNode.type(ctx).create({ target: match[1].trim(), display: (match[2] ?? '').trim() }))));
export const embedInputRule = $inputRule(ctx => new InputRule(
  /!\[\[([^\[\]\n|]+)\]\]$/,
  (state, match, start, end) => state.tr.replaceWith(start, end,
    embedNode.type(ctx).create({ slug: match[1].trim() }))));

// ── Paste-Rule: eingefügt, nicht nur getippt ──────────────────────────────
// Die Input-Rules feuern pro Tastendruck auf die schließende Klammer. Wer
// "[[ziel]]" aus einer anderen Karte KOPIERT, fügt den ganzen Block auf einmal
// ein — kein Tastendruck, keine Regel, und der Serializer escaped den Text zu
// "\\[\\[ziel]]". Eingefügter Klartext läuft deshalb durch den Markdown-
// Parser, wo die $remark-Plugins greifen; nur wenn flow-Syntax drinsteckt,
// sonst bleibt der Slice, wie er ist (kein Umdeuten normaler Absätze).
const FLOW_SYNTAX = /!?\[\[[^\[\]\n]+\]\]|^> \[![A-Za-z]+\]/m;
export const flowPasteRule = $pasteRule((ctx) => ({
  run: (slice, _view, isPlainText) => {
    if (!isPlainText) return slice;
    const text = slice.content.textBetween(0, slice.content.size, '\n');
    if (!FLOW_SYNTAX.test(text)) return slice;
    try { return markdownToSlice(text)(ctx); } catch { return slice; }
  },
}));

export const flowSyntax = [frontmatterRemark, frontmatterNode, wikiRemark, wikilinkNode, embedNode, calloutRemark, calloutNode, wikilinkInputRule, embedInputRule, flowPasteRule].flat();
