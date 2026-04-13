package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONOutput(t *testing.T) {
	var buf bytes.Buffer

	data := map[string]any{
		"nome":  "Mindex",
		"plano": "team",
		"ativo": true,
	}

	if err := JSON(&buf, data); err != nil {
		t.Fatalf("JSON() erro inesperado: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Fatal("JSON() produziu output vazio")
	}

	// verifica que é JSON válido
	var parsed map[string]any
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("output de JSON() não é JSON válido: %v\noutput: %s", err, output)
	}

	if parsed["nome"] != "Mindex" {
		t.Errorf("nome: esperado %q, obtido %q", "Mindex", parsed["nome"])
	}
}

func TestJSONOutputIndented(t *testing.T) {
	var buf bytes.Buffer

	data := map[string]any{"key": "value"}
	JSON(&buf, data)

	output := buf.String()
	// output indentado deve ter quebras de linha
	if !strings.Contains(output, "\n") {
		t.Error("JSON() deveria produzir output indentado com quebras de linha")
	}
}

func TestTableBasic(t *testing.T) {
	var buf bytes.Buffer

	headers := []string{"Nome", "Plano", "Status"}
	rows := [][]string{
		{"minha-org", "team", "ativo"},
		{"outra-org", "free", "inativo"},
	}

	Table(&buf, headers, rows)

	output := buf.String()
	if output == "" {
		t.Fatal("Table() produziu output vazio")
	}

	// verifica que os headers aparecem no output
	for _, h := range headers {
		if !strings.Contains(output, h) {
			t.Errorf("header %q não encontrado no output da tabela", h)
		}
	}

	// verifica que os dados aparecem
	for _, row := range rows {
		for _, cell := range row {
			if !strings.Contains(output, cell) {
				t.Errorf("célula %q não encontrada no output da tabela", cell)
			}
		}
	}
}

func TestTableAutoColumnWidth(t *testing.T) {
	var buf bytes.Buffer

	headers := []string{"Coluna"}
	rows := [][]string{
		{"valor curto"},
		{"este é um valor muito mais longo que o header"},
	}

	Table(&buf, headers, rows)
	output := buf.String()

	// a linha mais longa deve caber na coluna
	if !strings.Contains(output, "este é um valor muito mais longo que o header") {
		t.Error("conteúdo longo deveria aparecer no output da tabela")
	}
}

func TestTableEmpty(t *testing.T) {
	var buf bytes.Buffer

	headers := []string{"Col1", "Col2"}
	rows := [][]string{}

	Table(&buf, headers, rows)
	output := buf.String()

	// headers devem aparecer mesmo sem linhas
	if !strings.Contains(output, "Col1") {
		t.Error("header deveria aparecer mesmo sem linhas")
	}
}

func TestMarkdownPlainText(t *testing.T) {
	var buf bytes.Buffer

	content := "# Título\n\nAlgum **texto em negrito**."

	// com noColor=true, deve funcionar sem renderização ANSI
	if err := Markdown(&buf, content, true); err != nil {
		t.Fatalf("Markdown() erro inesperado: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Fatal("Markdown() produziu output vazio")
	}

	// o texto deve aparecer no output
	if !strings.Contains(output, "Título") {
		t.Errorf("conteúdo %q não encontrado no output: %s", "Título", output)
	}
}
