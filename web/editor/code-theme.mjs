// code-theme.mjs — CodeMirror in den Code-Farben der Leseansicht.
//
// Spiegelt chromacss.go (Stil „flow-karteikasten"): drei Farben — Schlüssel-
// wort, Zahl, String —, Kommentar in Meta-Tinte, alles andere Lesetinte auf
// --code-bg. Die Hex-Werte hier und dort hält TestCodeTheme_MatchesChroma
// gleich; ein Code-Block sieht beim Schreiben aus wie nach dem Speichern.
import { EditorView } from '@codemirror/view';
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language';
import { tags as t } from '@lezer/highlight';

export const CODE = {
  text: '#33312A',
  keyword: '#D9480F',
  number: '#1864AB',
  string: '#0F8A46',
  comment: '#8A8578',
  deleted: '#B4452F',
};

export const flowCodeTheme = [
  EditorView.theme({
    '&': { backgroundColor: 'rgb(var(--code-bg))', color: CODE.text, fontSize: '12.8px' },
    '.cm-scroller': { fontFamily: '"JetBrains Mono", ui-monospace, monospace', lineHeight: '1.65' },
    '.cm-content': { padding: '0', caretColor: 'rgb(var(--ink))' },
    '.cm-line': { padding: '0' },
    '.cm-gutters': { display: 'none' },
    '&.cm-focused': { outline: 'none' },
    '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'rgb(var(--ink))' },
    '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': { backgroundColor: 'rgb(var(--accent-wash))' },
    '.cm-activeLine': { backgroundColor: 'transparent' },
  }),
  syntaxHighlighting(HighlightStyle.define([
    { tag: [t.keyword, t.tagName, t.processingInstruction, t.controlKeyword, t.definitionKeyword, t.moduleKeyword], color: CODE.keyword },
    { tag: [t.number, t.integer, t.float, t.bool, t.null], color: CODE.number },
    { tag: [t.string, t.special(t.string), t.inserted], color: CODE.string },
    { tag: [t.comment, t.lineComment, t.blockComment, t.docComment], color: CODE.comment },
    { tag: t.deleted, color: CODE.deleted },
    { tag: t.emphasis, fontStyle: 'italic' },
    { tag: t.strong, fontWeight: 'bold' },
  ])),
];
