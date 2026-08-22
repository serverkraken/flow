// editor.mjs — Crepe (Milkdown) als Stift für den Wissens-Editor.
//
// Arbeitsteilung (Soenne, 21.08.): der Stift ist der STIFT, RenderDocument
// auf dem Server ist die WAHRHEIT. Crepe bringt mit, was ein Editor braucht —
// Blockmenü per „/", Auswahl-Werkzeuge, Link-Tooltip, Tabellen-Werkzeuge,
// Code-Block mit Sprachwahl, Block-Griff — auf derselben Engine wie zuvor;
// die flow-Syntax ([[…]], ![[…]], Callouts, Frontmatter) bleibt als Plugin.
//
// Die <textarea name="body"> bleibt die Quelle des Formulars und der
// Markdown-Vorschau: der Stift schreibt bei jeder Änderung Markdown hinein.
// Fällt das Skript aus, steht die Textarea weiterhin da — bedienbar.
// Nur die BEDIEN-Teile des Themes — Blockmenü, Code-Block-Chrome, Cursor,
// Link-Tooltip, Platzhalter, Werkzeuge, Tabellen-Griffe. Crepes reset.css
// (seine Typografie) bleibt draußen: die Schrift des Stifts ist .prose, die-
// selbe Regel wie in der Leseansicht (Soenne, 22.08.: „genauso gerendert wie
// später im Dokument"). Die Sammeldatei zöge zudem KaTeX samt Schriften mit.
import '@milkdown/crepe/theme/common/prosemirror.css';
import '@milkdown/crepe/theme/common/block-edit.css';
import '@milkdown/crepe/theme/common/code-mirror.css';
import '@milkdown/crepe/theme/common/cursor.css';
import '@milkdown/crepe/theme/common/link-tooltip.css';
import '@milkdown/crepe/theme/common/placeholder.css';
import '@milkdown/crepe/theme/common/toolbar.css';
import '@milkdown/crepe/theme/common/table.css';
import '@milkdown/crepe/theme/frame.css';
import { Crepe } from '@milkdown/crepe';
import { remarkStringifyOptionsCtx, editorViewCtx, editorViewOptionsCtx } from '@milkdown/kit/core';
import { remarkGFMPlugin } from '@milkdown/kit/preset/gfm';
import { replaceAll } from '@milkdown/kit/utils';
import { LanguageDescription, StreamLanguage } from '@codemirror/language';
import { shell } from '@codemirror/legacy-modes/mode/shell';
import { flowSyntax } from './flow-syntax.mjs';
import { flowCodeTheme } from './code-theme.mjs';
import { flowViews, setLabels, labels, renumber } from './flow-views.mjs';

// Schreibweise an den Bestand angleichen — Serializer-Optionen, keine
// Modell-Eigenschaften. Agenten schreiben dieselben Karten; ein Editor, der
// '*' statt '-' setzt, verrauscht jeden Diff.
// Code-Sprachen im Stift: die, die in flow-Karten vorkommen — nicht das
// ganze Sprachverzeichnis (das wäre 2 MB Bundle für Sprachen, die hier nie
// jemand schreibt). Alles andere bleibt Klartext; Mermaid zeichnet der
// Code-Block unter sich als Figur (mermaidPreview).
const languages = [
  LanguageDescription.of({ name: 'Go', alias: ['go', 'golang'], extensions: ['go'], load: () => import('@codemirror/lang-go').then((m) => m.go()) }),
  LanguageDescription.of({ name: 'JavaScript', alias: ['js', 'javascript', 'mjs'], extensions: ['js', 'mjs'], load: () => import('@codemirror/lang-javascript').then((m) => m.javascript()) }),
  LanguageDescription.of({ name: 'TypeScript', alias: ['ts', 'typescript'], extensions: ['ts'], load: () => import('@codemirror/lang-javascript').then((m) => m.javascript({ typescript: true })) }),
  LanguageDescription.of({ name: 'CSS', alias: ['css'], extensions: ['css'], load: () => import('@codemirror/lang-css').then((m) => m.css()) }),
  LanguageDescription.of({ name: 'HTML', alias: ['html', 'templ'], extensions: ['html'], load: () => import('@codemirror/lang-html').then((m) => m.html()) }),
  LanguageDescription.of({ name: 'JSON', alias: ['json'], extensions: ['json'], load: () => import('@codemirror/lang-json').then((m) => m.json()) }),
  LanguageDescription.of({ name: 'YAML', alias: ['yaml', 'yml'], extensions: ['yaml', 'yml'], load: () => import('@codemirror/lang-yaml').then((m) => m.yaml()) }),
  LanguageDescription.of({ name: 'Markdown', alias: ['md', 'markdown'], extensions: ['md'], load: () => import('@codemirror/lang-markdown').then((m) => m.markdown()) }),
  LanguageDescription.of({ name: 'SQL', alias: ['sql'], extensions: ['sql'], load: () => import('@codemirror/lang-sql').then((m) => m.sql()) }),
  LanguageDescription.of({ name: 'Python', alias: ['py', 'python'], extensions: ['py'], load: () => import('@codemirror/lang-python').then((m) => m.python()) }),
  LanguageDescription.of({ name: 'Shell', alias: ['sh', 'bash', 'zsh', 'shell'], extensions: ['sh'], load: async () => StreamLanguage.define(shell) }),
];

// ```mermaid: die Figur der Leseansicht unter dem Code — Rahmen, SVG,
// „Abb. n · gerendert aus mermaid". Gezeichnet von derselben Lib über die
// Brücke in mermaid-init.js (window.flowMermaid), entprellt je Anschlag;
// ein Syntaxfehler zeigt den Fehlerrahmen mit der Quelle, wie der Server.
function mermaidFigure(src, svg) {
  const fig = document.createElement('figure');
  fig.className = 'mermaid-figure' + (svg ? '' : ' mermaid-error');
  fig.dataset.fig = '';
  const fr = document.createElement('div');
  fr.className = 'frame';
  if (svg) fr.innerHTML = svg;
  else { const pre = document.createElement('pre'); pre.className = 'mermaid'; pre.textContent = src; fr.appendChild(pre); }
  fig.appendChild(fr);
  const cap = document.createElement('figcaption');
  const b = document.createElement('b'); b.textContent = labels().fig;
  cap.appendChild(b);
  const span = document.createElement('span'); span.className = 'mermaid-cap'; span.textContent = labels().mermaid;
  cap.appendChild(document.createTextNode(' · ')); cap.appendChild(span);
  fig.appendChild(cap);
  return fig;
}
function mermaidPreview(language, content, apply) {
  if ((language || '').toLowerCase() !== 'mermaid') return null;
  const src = content.trim();
  if (!src) return null;
  setTimeout(() => {
    const mm = window.flowMermaid;
    const done = (fig) => { apply(fig); queueMicrotask(() => { const root = document.querySelector('.milkdown .ProseMirror'); if (root) renumber(root); }); };
    if (!mm) return done(mermaidFigure(src, null));
    mm.render(src).then((svg) => done(mermaidFigure(src, svg))).catch(() => done(mermaidFigure(src, null)));
  }, 400);
  return undefined; // asynchron — apply() liefert nach
}

const STRINGIFY = {
  bullet: '-', listItemIndent: 'one', emphasis: '*', strong: '*', rule: '-',
};

// Die Beschriftungen des Stifts auf Deutsch — Crepe ist sonst englisch.
const DE = {
  placeholder: 'Schreib los — „/" öffnet die Blöcke, „[[" die Verweise.',
  textGroup: { label: 'Text', text: { label: 'Absatz' }, h1: { label: 'Titel (H1)' }, h2: { label: 'Abschnitt (H2)' }, h3: { label: 'Unterabschnitt (H3)' }, h4: { label: 'H4' }, h5: { label: 'H5' }, h6: { label: 'H6' }, quote: { label: 'Zitat' }, divider: { label: 'Trennlinie' } },
  listGroup: { label: 'Listen', bulletList: { label: 'Liste' }, orderedList: { label: 'Nummerierte Liste' }, taskList: { label: 'Aufgaben' } },
  advancedGroup: { label: 'Blöcke', image: null, codeBlock: { label: 'Code' }, table: { label: 'Tabelle' }, math: null },
  toolbar: { boldLabel: 'Fett', italicLabel: 'Kursiv', strikethroughLabel: 'Durchgestrichen', codeLabel: 'Code', linkLabel: 'Link' },
  link: { editButton: 'Ändern', removeButton: 'Entfernen', confirmButton: 'OK', inputPlaceholder: 'Adresse eintragen …' },
  code: { previewLabel: 'Diagramm', previewLoading: 'Zeichne …', previewToggleText: (only) => (only ? 'Code zeigen' : 'Code ausblenden'), copyText: 'Kopieren', searchPlaceholder: 'Sprache suchen', noResultText: 'Keine Sprache' },
};

async function mount(source) {
  // Figuren-Beschriftungen aus dem Katalog des Servers (editor.templ).
  setLabels({ fig: source.dataset.figLabel, mermaid: source.dataset.mermaidLabel, unresolved: source.dataset.unresolvedLabel });
  const host = document.createElement('div');
  host.className = 'milkdown-host';
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

  const crepe = new Crepe({
    root: host,
    defaultValue: source.value,
    features: {
      [Crepe.Feature.ImageBlock]: false, // Bilder kommen als ![[Artefakt]] über den Picker
      // Crepes Listenpunkte sind eigene Bausteine (Label-Spalte, SVG-Punkt);
      // die Leseansicht setzt ul/li mit list-disc — der Stift auch.
      [Crepe.Feature.ListItem]: false,
      [Crepe.Feature.Latex]: false,
      [Crepe.Feature.TopBar]: false,
      [Crepe.Feature.AI]: false,
    },
    featureConfigs: {
      [Crepe.Feature.Placeholder]: { text: DE.placeholder, mode: 'doc' },
      [Crepe.Feature.BlockEdit]: { textGroup: DE.textGroup, listGroup: DE.listGroup, advancedGroup: DE.advancedGroup },
      [Crepe.Feature.Toolbar]: DE.toolbar,
      [Crepe.Feature.LinkTooltip]: DE.link,
      [Crepe.Feature.CodeMirror]: { languages, theme: flowCodeTheme, renderPreview: mermaidPreview, ...DE.code },
    },
  });

  crepe.editor
    .config((ctx) => {
      // Das Wurzelelement des Stifts ist .prose: dieselbe Typografie wie die
      // Leseansicht, aus derselben Regel — keine zweite Schriftleiter.
      ctx.update(editorViewOptionsCtx, (o) => {
        const prev = o.attributes;
        return { ...o, attributes: (state) => {
          const a = typeof prev === 'function' ? prev(state) : (prev ?? {});
          return { ...a, class: [a.class, 'prose'].filter(Boolean).join(' ') };
        } };
      });
      ctx.set(remarkGFMPlugin.options.key, { tablePipeAlign: false, tableCellPadding: true });
      ctx.update(remarkStringifyOptionsCtx, (o) => ({
        ...o, ...STRINGIFY,
        // Harter Umbruch als zwei Leerzeichen, wie der Bestand ihn schreibt.
        handlers: { ...(o.handlers ?? {}), break: () => '  \n' },
      }));
    })
    .use(flowSyntax)
    .use(flowViews);
  crepe.on((l) => l.markdownUpdated((_, markdown) => sync(markdown)));
  await crepe.create();

  host.dataset.ready = '';
  // Der Modus-Umschalter (editor-mode.js) spricht den Stift hierüber an.
  window.flowEditor = {
    setMarkdown(md) { crepe.editor.action(replaceAll(md)); },
    getMarkdown() { return crepe.getMarkdown(); },
    focus() { crepe.editor.ctx.get(editorViewCtx).focus(); },
  };
  document.dispatchEvent(new CustomEvent('flow:editor-ready'));
}

function boot() {
  const source = document.getElementById('editor-body');
  if (!source || source.dataset.milkdown === 'mounted') return;
  source.dataset.milkdown = 'mounted';
  mount(source).catch((err) => {
    // Stift kaputt → Textarea bleibt. Laut, damit es auffällt.
    console.error('flow editor: Crepe konnte nicht starten, Textarea bleibt aktiv', err);
    delete source.dataset.milkdown;
  });
}

if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot);
else boot();
