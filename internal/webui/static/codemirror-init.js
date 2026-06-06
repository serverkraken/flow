// codemirror-init.js — Plan E · Task 12 (M7 note editing).
//
// Boots a CodeMirror 6 editor into a div on the page, hydrating its
// initial content from a hidden <textarea>. On form submit the editor's
// current document is written back into the textarea so the standard
// form post carries the edited content to the server.
//
// The CM bundle (window.CM) is built from tools/codemirror/entry.mjs.
// We pick only the extensions we need: basicSetup, markdown language,
// default keymap with Tab-indent, and the Tokyonight theme that matches
// the rest of the WebUI.
//
// Vim mode is intentionally NOT enabled — users who want vim run flow in
// their terminal via the kompendium TUI. The WebUI editor stays minimal.
//
// Public surface: window.initNoteEditor(mountId, textareaId). The mount
// div is replaced with the CM view; the textarea is kept in the form but
// hidden so the submit still includes a `content` field even on browsers
// where the JS init failed (graceful degradation — without JS the
// textarea is the editor).
(function () {
    "use strict";

    function init(mountId, textareaId) {
        var mount = document.getElementById(mountId);
        var ta = document.getElementById(textareaId);
        if (!mount || !ta) return;
        // Fallback: no CM bundle (e.g. JS disabled or load failure) →
        // leave the textarea visible so the user can still edit. The
        // form submit works either way.
        if (!window.CM) {
            ta.style.display = "block";
            return;
        }

        var CM = window.CM;
        var extensions = [
            CM.basicSetup,
            CM.markdown(),
            CM.keymap.of([].concat(CM.defaultKeymap, [CM.indentWithTab])),
        ];
        // Optional theme — present in the bundle but guarded so a future
        // bundle rebuild that drops it doesn't crash init.
        if (CM.tokyonightTheme) extensions.push(CM.tokyonightTheme);
        if (CM.syntaxHighlighting && CM.tokyonightHighlight) {
            extensions.push(CM.syntaxHighlighting(CM.tokyonightHighlight));
        }

        var view = new CM.EditorView({
            doc: ta.value,
            extensions: extensions,
            parent: mount,
        });

        // Hide the textarea but keep it in the form. The submit handler
        // copies the current editor doc into it just before the request
        // goes out so the server receives the latest content.
        ta.style.display = "none";

        var form = ta.form;
        if (form) {
            form.addEventListener("submit", function () {
                ta.value = view.state.doc.toString();
            });
        }
    }

    window.initNoteEditor = init;
})();
