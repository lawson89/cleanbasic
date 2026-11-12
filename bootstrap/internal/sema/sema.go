package sema

import (
	"os"

	"cleanbasic/bootstrap/internal/ast"
	"cleanbasic/bootstrap/internal/parser"
)

// AnalyzedProgram holds the parsed (and in Phase 2+, semantically checked) program.
type AnalyzedProgram struct {
	EntryFile string
	Main      *ast.Program
}

// LoadAndAnalyze loads and parses the entry CleanBasic file.
// Full module graph resolution and semantic analysis are implemented in Phase 2.
func LoadAndAnalyze(entryPath string) (*AnalyzedProgram, error) {
	raw, err := os.ReadFile(entryPath)
	if err != nil {
		return nil, err
	}
	prog, err := parser.Parse(string(raw), entryPath)
	if err != nil {
		return nil, err
	}
	return &AnalyzedProgram{
		EntryFile: entryPath,
		Main:      prog,
	}, nil
}
