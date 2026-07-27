// Command chromacss writes the committed static/chroma.css.
package main

import (
	"os"

	"github.com/serverkraken/flow/internal/adapter/webui"
)

func main() {
	if err := os.WriteFile("internal/adapter/webui/static/chroma.css", []byte(webui.GenerateChromaCSS()), 0o644); err != nil {
		panic(err)
	}
}
