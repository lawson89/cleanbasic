package main

import (
	"path/filepath"
	"testing"
)

func TestCLIParseCommand(t *testing.T) {
	file := filepath.Join("..", "..", "..", "tests", "cases", "01_hello_and_math.cb")
	if code := run([]string{"parse", file}); code != 0 {
		t.Fatalf("cb parse %s returned exit code %d, want 0", file, code)
	}
	if code := run([]string{"--dump-ast", file}); code != 0 {
		t.Fatalf("cb --dump-ast %s returned exit code %d, want 0", file, code)
	}
	if code := run([]string{}); code != 2 {
		t.Fatalf("cb with no args returned %d, want 2", code)
	}
}
