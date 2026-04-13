package output

import (
	"io"
	"os"

	"github.com/charmbracelet/glamour"
)

// Markdown renderiza conteúdo markdown em w.
// Se noColor=true ou a variável NO_COLOR estiver definida, usa texto simples sem ANSI.
func Markdown(w io.Writer, content string, noColor bool) error {
	// usa texto simples quando cores estão desativadas
	if noColor || os.Getenv("NO_COLOR") != "" {
		renderer, err := glamour.NewTermRenderer(
			glamour.WithStylePath("notty"),
		)
		if err != nil {
			// fallback: escreve o conteúdo bruto
			_, err = io.WriteString(w, content)
			return err
		}
		rendered, err := renderer.Render(content)
		if err != nil {
			_, err = io.WriteString(w, content)
			return err
		}
		_, err = io.WriteString(w, rendered)
		return err
	}

	// renderização com cores (ambiente de terminal)
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)
	if err != nil {
		// fallback: escreve o conteúdo bruto
		_, err = io.WriteString(w, content)
		return err
	}

	rendered, err := renderer.Render(content)
	if err != nil {
		_, err = io.WriteString(w, content)
		return err
	}

	_, err = io.WriteString(w, rendered)
	return err
}
