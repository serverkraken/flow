// Single-file IIFE exposed as `window.CM` — consumed by
// internal/webui/static/codemirror-init.js (Plan E / Task 12).
//
// We pick a deliberately minimal extension surface so the bundle stays small:
// basicSetup + markdown language + default keymap + indentWithTab + a custom
// Tokyonight theme. No vim mode (users who want it can run flow inside their
// terminal); no autocomplete (note editing is prose, not code).
import { EditorView, basicSetup }      from "codemirror";
import { EditorState, Compartment }    from "@codemirror/state";
import { keymap, lineNumbers }         from "@codemirror/view";
import { defaultKeymap, indentWithTab, history, historyKeymap } from "@codemirror/commands";
import { markdown }                    from "@codemirror/lang-markdown";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { tags as t }                   from "@lezer/highlight";

// Tokyonight syntax tokens (matches the TUI's markdown render).
const tokyonightHighlight = HighlightStyle.define([
  { tag: t.heading,                 color: "#7aa2f7", fontWeight: "300" },
  { tag: [t.heading1, t.heading2],  color: "#7aa2f7", fontWeight: "200" },
  { tag: t.strong,                  color: "#c0caf5", fontWeight: "700" },
  { tag: t.emphasis,                color: "#bb9af7", fontStyle: "italic" },
  { tag: t.link,                    color: "#7dcfff", textDecoration: "underline" },
  { tag: t.url,                     color: "#7dcfff" },
  { tag: t.monospace,               color: "#e0af68" },
  { tag: t.list,                    color: "#9ece6a" },
  { tag: t.quote,                   color: "#9aa5ce", fontStyle: "italic" },
  { tag: t.comment,                 color: "#414868", fontStyle: "italic" },
  { tag: t.atom,                    color: "#f7768e" },
  { tag: t.processingInstruction,   color: "#414868" },
]);

const tokyonightTheme = EditorView.theme({
  "&":                { color: "#c0caf5", backgroundColor: "#16161e" },
  ".cm-content":      { caretColor: "#7aa2f7", fontFamily: "'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, monospace" },
  ".cm-cursor, .cm-dropCursor": { borderLeftColor: "#7aa2f7" },
  "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection":
                      { backgroundColor: "#33467c" },
  ".cm-activeLine":   { backgroundColor: "#1f2335" },
  ".cm-gutters":      { backgroundColor: "#1a1b26", color: "#414868", border: "none", borderRight: "1px solid #2a2e44" },
  ".cm-activeLineGutter": { backgroundColor: "transparent", color: "#9aa5ce" },
  ".cm-lineNumbers .cm-gutterElement": { padding: "0 0.5rem 0 0.75rem", fontVariantNumeric: "tabular-nums" },
}, { dark: true });

export {
  EditorView, EditorState, Compartment, basicSetup,
  keymap, lineNumbers, history, historyKeymap,
  defaultKeymap, indentWithTab,
  markdown,
  HighlightStyle, syntaxHighlighting,
  tokyonightHighlight, tokyonightTheme,
};
