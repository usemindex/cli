package output

import (
	"fmt"
	"io"
	"strings"
)

// Table escreve uma tabela formatada com larguras de colunas automáticas.
func Table(w io.Writer, headers []string, rows [][]string) {
	if len(headers) == 0 {
		return
	}

	// calcula a largura máxima de cada coluna
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// linha separadora
	separator := buildSeparator(widths)

	// escreve o header
	fmt.Fprintln(w, separator)
	fmt.Fprint(w, "|")
	for i, h := range headers {
		fmt.Fprintf(w, " %-*s |", widths[i], h)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, separator)

	// escreve as linhas
	for _, row := range rows {
		fmt.Fprint(w, "|")
		for i := range headers {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			fmt.Fprintf(w, " %-*s |", widths[i], cell)
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, separator)
}

// buildSeparator cria a linha de separação com as larguras fornecidas.
func buildSeparator(widths []int) string {
	parts := make([]string, len(widths))
	for i, w := range widths {
		parts[i] = strings.Repeat("-", w+2)
	}
	return "+" + strings.Join(parts, "+") + "+"
}
