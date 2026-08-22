// Leerer Ersatz für katex: die Latex-Funktion von Crepe ist abgeschaltet
// (flow hat keine Formeln), ihre statische Abhängigkeit wäre sonst trotzdem
// im Bundle. Wird nie aufgerufen.
const katex = { render() {}, renderToString() { return ''; } };
export default katex;
