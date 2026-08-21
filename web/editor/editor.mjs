// editor.mjs — Milkdown als Stift für den Wissens-Editor.
//
// Arbeitsteilung (Soenne, 21.08.): Milkdown ist der STIFT, RenderDocument auf
// dem Server ist die WAHRHEIT. Der Editor kennt flows Syntax (Wikilink,
// Artefakt-Einbettung, Callout, Frontmatter), damit sie beim Schreiben nicht
// zerbricht — gezeigt wird sie von der Server-Vorschau daneben, mit Chroma,
// Mermaid und aufgelösten Links. Deshalb rendert der Editor selbst KEINE
// Code-Farben und KEINE Diagramme; das wäre eine zweite Wahrheit.
//
// Die <textarea name="body"> bleibt die Quelle des Formulars und der
// htmx-Vorschau: Milkdown schreibt bei jeder Änderung Markdown hinein und
// löst deren Vorschau-Request aus. Fällt das Skript aus (CSP, Netz, alter
// Browser), steht die Textarea weiterhin da — bedienbar, nur ohne Stift.
import { Editor, rootCtx, defaultValueCtx, remarkStringifyOptionsCtx } from '@milkdown/kit/core';
import { commonmark } from '@milkdown/kit/preset/commonmark';
import { gfm, remarkGFMPlugin } from '@milkdown/kit/preset/gfm';
import { history } from '@milkdown/kit/plugin/history';
import { listener, listenerCtx } from '@milkdown/kit/plugin/listener';
import { flowSyntax } from './flow-syntax.mjs';

// Schreibweise an den Bestand angleichen — das sind Serializer-Optionen,
// keine Modell-Eigenschaften. Agenten schreiben dieselben Karten; ein Editor,
// der '*' statt '-' setzt, verrauscht jeden Diff.
const STRINGIFY = {
  bullet: '-', listItemIndent: 'one', emphasis: '*', strong: '*', rule: '-',
};

async function mount(source) {
  const host = document.createElement('div');
  host.className = 'prose milkdown-host';
  source.insertAdjacentElement('beforebegin', host);

  // Vorschau über die bestehende htmx-Verdrahtung der Textarea auslösen:
  // wir setzen ihren Wert und feuern das Ereignis, auf das sie hört.
  let timer = 0;
  const sync = (markdown) => {
    if (source.value === markdown) return;
    source.value = markdown;
    clearTimeout(timer);
    timer = setTimeout(() => source.dispatchEvent(new Event('keyup', { bubbles: true })), 400);
  };

  await Editor.make()
    .config((ctx) => {
      ctx.set(rootCtx, host);
      ctx.set(defaultValueCtx, source.value);
      ctx.set(remarkGFMPlugin.options.key, { tablePipeAlign: false, tableCellPadding: true });
      ctx.update(remarkStringifyOptionsCtx, (o) => ({
        ...o, ...STRINGIFY,
        // Harter Umbruch als zwei Leerzeichen, wie der Bestand ihn schreibt.
        handlers: { ...(o.handlers ?? {}), break: () => '  \n' },
      }));
      ctx.get(listenerCtx).markdownUpdated((_, markdown) => sync(markdown));
    })
    .use(commonmark).use(gfm).use(history).use(listener).use(flowSyntax)
    .create();

  source.classList.add('sr-only');
  source.setAttribute('aria-hidden', 'true');
  source.tabIndex = -1;
  host.dataset.ready = '';
}

function boot() {
  const source = document.getElementById('editor-body');
  if (!source || source.dataset.milkdown === 'mounted') return;
  source.dataset.milkdown = 'mounted';
  mount(source).catch((err) => {
    // Stift kaputt → Textarea bleibt. Laut, damit es auffällt.
    console.error('flow editor: Milkdown konnte nicht starten, Textarea bleibt aktiv', err);
    delete source.dataset.milkdown;
  });
}

if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot);
else boot();
