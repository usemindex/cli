package cmd

import (
	"testing"
)

func TestChunkFiles(t *testing.T) {
	cases := []struct {
		name     string
		input    []string
		size     int
		expected [][]int // sizes of resulting chunks
	}{
		{"exact multiple", make([]string, 100), 50, [][]int{{50}, {50}}},
		{"with remainder", make([]string, 115), 50, [][]int{{50}, {50}, {15}}},
		{"smaller than chunk", make([]string, 5), 50, [][]int{{5}}},
		{"empty", []string{}, 50, [][]int{}},
		{"size 1", []string{"a", "b", "c"}, 1, [][]int{{1}, {1}, {1}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chunks := chunkFiles(tc.input, tc.size)
			if len(chunks) != len(tc.expected) {
				t.Fatalf("número de chunks: esperado %d, obtido %d", len(tc.expected), len(chunks))
			}
			for i, c := range chunks {
				if len(c) != tc.expected[i][0] {
					t.Errorf("chunk %d: esperado tamanho %d, obtido %d", i, tc.expected[i][0], len(c))
				}
			}
		})
	}
}
