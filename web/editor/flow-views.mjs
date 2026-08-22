// flow-views.mjs — was der Stift zeigt, wie es die Leseansicht zeigt.
//
// Verweise ([[…]]), Einbettungen (![[…]]) und Callouts bekommen im Stift
// dasselbe Markup wie aus RenderDocument: a.wikilink / span.wikilink-broken,
// figure.figure mit Rahmen, Bild und „Abb. n · Name", div.callout mit
// Titelzeile. Was sich auflöst, entscheidet der SERVER (/ui/editor/aufloesen,
// dieselben Regeln wie die Leseansicht) — hier wird nur gezeichnet.
import { $prose, $view } from '@milkdown/kit/utils';
import { Plugin, PluginKey } from '@milkdown/kit/prose/state';
import { Decoration, DecorationSet } from '@milkdown/kit/prose/view';
import { wikilinkNode, embedNode, CALLOUT_GLYPH } from './flow-syntax.mjs';

export const resolveKey = new PluginKey('flowResolve');

// Beschriftungen kommen aus dem i18n-Katalog des Servers (editor.templ legt
// sie als data-Attribute an die Textarea) — dieselben Wörter wie die Figuren
// der Leseansicht.
const LABELS = { fig: 'Abb.', mermaid: 'gerendert aus ```mermaid', unresolved: 'Unaufgelöste Artefakt-Referenz' };
export function setLabels(l) { Object.assign(LABELS, l); }
export function labels() { return LABELS; }

// Der Knoten des Editors: das versteckte Feld (Bearbeiten) oder die
// Projektauswahl (Neu) — dieselben Felder, die die Vorschau mitschickt.
function nodeOf() {
  const f = document.querySelector('[name=node]') || document.querySelector('[name=projectId]');
  return f ? f.value : '';
}

function targetsOf(doc) {
  const wl = new Set(), em = new Set();
  doc.descendants((n) => {
    if (n.type.name === 'flow_wikilink') wl.add(n.attrs.target);
    else if (n.type.name === 'flow_embed') em.add(n.attrs.slug);
  });
  return { wl, em };
}

// Figuren durchnummerieren — in DOM-Reihenfolge, wie figureTransformer es
// auf dem Server tut (Einbettungen und Mermaid-Diagramme in einer Zählung).
export function renumber(root) {
  let i = 0;
  root.querySelectorAll('figure[data-fig] figcaption b').forEach((b) => { b.textContent = LABELS.fig + ' ' + (++i); });
}

async function fetchMissing(view) {
  const st = resolveKey.getState(view.state);
  const { wl, em } = targetsOf(view.state.doc);
  const qs = new URLSearchParams();
  qs.set('node', st.node);
  let n = 0;
  for (const t of wl) if (!st.wl.has(t)) { qs.append('wl', t); n++; }
  for (const s of em) if (!st.em.has(s)) { qs.append('embed', s); n++; }
  if (!n) return;
  try {
    const r = await fetch('/ui/editor/aufloesen?' + qs.toString(), { credentials: 'same-origin', headers: { Accept: 'application/json' } });
    if (!r.ok) return; // unbekannt bleibt unbekannt — der nächste Anschlag fragt erneut
    const data = await r.json();
    if (resolveKey.getState(view.state).node !== st.node) return; // Register gewechselt — Antwort verfallen
    const meta = { wl: {}, em: {} };
    for (const t of wl) if (!st.wl.has(t)) meta.wl[t] = (data.wikilinks && data.wikilinks[t]) || null;
    for (const s of em) if (!st.em.has(s)) meta.em[s] = (data.embeds && data.embeds[s]) || null;
    view.dispatch(view.state.tr.setMeta(resolveKey, meta).setMeta('addToHistory', false));
  } catch { /* Netz weg → Anzeige bleibt „ausstehend", kein Fehlerrauschen */ }
}

function calloutTitle(kind, title) {
  const k = kind.toLowerCase();
  const p = document.createElement('p');
  p.className = 'callout-title';
  p.contentEditable = 'false';
  const g = document.createElement('span');
  g.className = 'callout-glyph';
  g.setAttribute('aria-hidden', 'true');
  g.textContent = CALLOUT_GLYPH[k] || '';
  p.appendChild(g);
  p.appendChild(document.createTextNode(' ' + (title || k.charAt(0).toUpperCase() + k.slice(1))));
  return p;
}

export const flowResolvePlugin = $prose(() => new Plugin({
  key: resolveKey,
  state: {
    init: () => ({ wl: new Map(), em: new Map(), node: nodeOf() }),
    apply(tr, st) {
      const meta = tr.getMeta(resolveKey);
      if (!meta) return st;
      if (meta.reset) return { wl: new Map(), em: new Map(), node: meta.node };
      const wl = new Map(st.wl), em = new Map(st.em);
      for (const [k, v] of Object.entries(meta.wl || {})) wl.set(k, v);
      for (const [k, v] of Object.entries(meta.em || {})) em.set(k, v);
      return { ...st, wl, em };
    },
  },
  props: {
    // Die Auflösung reist als Dekoration zu den Node-Views (spec.ref/known);
    // Callouts bekommen ihre Titelzeile als Widget vor dem ersten Block.
    decorations(state) {
      const st = resolveKey.getState(state);
      const decos = [];
      state.doc.descendants((n, pos) => {
        if (n.type.name === 'flow_wikilink') {
          decos.push(Decoration.node(pos, pos + n.nodeSize, {}, { ref: st.wl.get(n.attrs.target) || null, known: st.wl.has(n.attrs.target) }));
        } else if (n.type.name === 'flow_embed') {
          decos.push(Decoration.node(pos, pos + n.nodeSize, {}, { ref: st.em.get(n.attrs.slug) || null, known: st.em.has(n.attrs.slug) }));
        } else if (n.type.name === 'flow_callout') {
          const kind = n.attrs.kind, title = n.attrs.title;
          decos.push(Decoration.widget(pos + 1, () => calloutTitle(kind, title), { side: -1, key: 'ct:' + kind + ':' + title, ignoreSelection: true }));
        }
      });
      return DecorationSet.create(state.doc, decos);
    },
  },
  view(view) {
    let timer = 0;
    const schedule = () => { clearTimeout(timer); timer = setTimeout(() => fetchMissing(view), 300); };
    const onChange = (e) => {
      const name = e.target && e.target.name;
      if (name === 'node' || name === 'projectId') view.dispatch(view.state.tr.setMeta(resolveKey, { reset: true, node: nodeOf() }).setMeta('addToHistory', false));
    };
    document.addEventListener('change', onChange);
    schedule();
    renumber(view.dom);
    return {
      update(v, prev) {
        if (v.state.doc !== prev.doc || resolveKey.getState(v.state) !== resolveKey.getState(prev)) schedule();
        renumber(v.dom);
      },
      destroy() { clearTimeout(timer); document.removeEventListener('change', onChange); },
    };
  },
}));

function specOf(decorations) {
  for (const d of decorations) if (d.spec && 'known' in d.spec) return d.spec;
  return { ref: null, known: false };
}

// [[ziel|anzeige]] — aufgelöst wie der Server: Anzeige, sonst Titel der
// Karte, sonst das Ziel. Kaputt = rot durchgestrichen (wikilink-broken).
export const wikilinkView = $view(wikilinkNode, () => (node, _view, _getPos, decorations) => {
  const dom = document.createElement('span');
  const render = (n, decos) => {
    const { ref, known } = specOf(decos);
    dom.className = !known ? 'wl' : ref ? 'wikilink' : 'wikilink-broken';
    dom.dataset.wikilink = n.attrs.target;
    dom.dataset.display = n.attrs.display;
    dom.textContent = n.attrs.display || (ref && ref.title) || n.attrs.target;
    dom.title = ref ? ref.href : '';
  };
  render(node, decorations);
  return { dom, update(n, decos) { if (n.type !== node.type) return false; render(n, decos); return true; }, ignoreMutation: () => true };
});

// ![[slug]] — die Figur der Leseansicht: Rahmen, Bild, „Abb. n · Name";
// Dateien als Chip; unaufgelöst wie der Server (wikilink-broken mit Titel);
// noch ohne Antwort: die Adresse in Mono.
export const embedView = $view(embedNode, () => (node, _view, _getPos, decorations) => {
  const dom = document.createElement('span');
  dom.className = 'embed-host';
  dom.contentEditable = 'false';
  const build = (n, decos) => {
    const { ref, known } = specOf(decos);
    dom.dataset.embed = n.attrs.slug;
    dom.replaceChildren();
    let el;
    if (!known) {
      el = document.createElement('span'); el.className = 'editor-embed'; el.textContent = `![[${n.attrs.slug}]]`;
    } else if (!ref) {
      el = document.createElement('span'); el.className = 'wikilink-broken'; el.title = LABELS.unresolved; el.textContent = n.attrs.slug;
    } else {
      el = document.createElement('figure'); el.className = 'figure'; el.dataset.fig = '';
      if (ref.isImage) {
        const fr = document.createElement('div'); fr.className = 'frame';
        const img = document.createElement('img'); img.src = ref.src; img.alt = ref.name; img.loading = 'lazy';
        if (ref.width) img.width = ref.width;
        if (ref.height) img.height = ref.height;
        fr.appendChild(img); el.appendChild(fr);
      } else {
        const a = document.createElement('span'); a.className = 'filechip'; a.textContent = '■ ' + ref.name + ' · ' + ref.size; el.appendChild(a);
      }
      const cap = document.createElement('figcaption');
      const b = document.createElement('b'); b.textContent = LABELS.fig;
      cap.appendChild(b); cap.appendChild(document.createTextNode(' · ' + ref.name));
      el.appendChild(cap);
    }
    dom.style.display = el.tagName === 'FIGURE' ? 'block' : 'inline';
    dom.appendChild(el);
  };
  build(node, decorations);
  return { dom, update(n, decos) { if (n.type !== node.type) return false; build(n, decos); return true; }, ignoreMutation: () => true };
});

export const flowViews = [flowResolvePlugin, wikilinkView, embedView].flat();
