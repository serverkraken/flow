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
// Nur die Teile des Themes, die wir nutzen — die Sammeldatei zöge KaTeX samt
// Schriften mit, und Formeln gibt es in flow nicht.
import '@milkdown/crepe/theme/common/prosemirror.css';
import '@milkdown/crepe/theme/common/reset.css';
import '@milkdown/crepe/theme/common/block-edit.css';
import '@milkdown/crepe/theme/common/code-mirror.css';
import '@milkdown/crepe/theme/common/cursor.css';
import '@milkdown/crepe/theme/common/link-tooltip.css';
import '@milkdown/crepe/theme/common/list-item.css';
import '@milkdown/crepe/theme/common/placeholder.css';
import '@milkdown/crepe/theme/common/toolbar.css';
import '@milkdown/crepe/theme/common/table.css';
import '@milkdown/crepe/theme/frame.css';
import { Crepe } from '@milkdown/crepe';
import { remarkStringifyOptionsCtx, editorViewCtx } from '@milkdown/kit/core';
import { remarkGFMPlugin } from '@milkdown/kit/preset/gfm';
import { replaceAll } from '@milkdown/kit/utils';
import { LanguageDescription, StreamLanguage } from '@codemirror/language';
import { shell } from '@codemirror/legacy-modes/mode/shell';
import { flowSyntax } from './flow-syntax.mjs';

// Schreibweise an den Bestand angleichen — Serializer-Optionen, keine
// Modell-Eigenschaften. Agenten schreiben dieselben Karten; ein Editor, der
// '*' statt '-' setzt, verrauscht jeden Diff.
// Code-Sprachen im Stift: die, die in flow-Karten vorkommen — nicht das
// ganze Sprachverzeichnis (das wäre 2 MB Bundle für Sprachen, die hier nie
// jemand schreibt). Alles andere bleibt Klartext, Mermaid ebenso: das Diagramm
// zeigt die Karte nach dem Speichern.
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
};

async function mount(source) {
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
      [Crepe.Feature.Latex]: false,
      [Crepe.Feature.TopBar]: false,
      [Crepe.Feature.AI]: false,
    },
    featureConfigs: {
      [Crepe.Feature.Placeholder]: { text: DE.placeholder, mode: 'doc' },
      [Crepe.Feature.BlockEdit]: { textGroup: DE.textGroup, listGroup: DE.listGroup, advancedGroup: DE.advancedGroup },
      [Crepe.Feature.Toolbar]: DE.toolbar,
      [Crepe.Feature.LinkTooltip]: DE.link,
      [Crepe.Feature.CodeMirror]: { languages },
    },
  });

  crepe.editor
    .config((ctx) => {
      ctx.set(remarkGFMPlugin.options.key, { tablePipeAlign: false, tableCellPadding: true });
      ctx.update(remarkStringifyOptionsCtx, (o) => ({
        ...o, ...STRINGIFY,
        // Harter Umbruch als zwei Leerzeichen, wie der Bestand ihn schreibt.
        handlers: { ...(o.handlers ?? {}), break: () => '  \n' },
      }));
    })
    .use(flowSyntax);
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
