# CleanBasic

A modern BASIC dialect that transpiles to Go.

## Bootstrap Compiler CLI (`cb`)

Build the `cb` compiler from the `bootstrap/` module:

```bash
cd bootstrap
go build -o cb ./cmd/cb
```

### Commands

- `cb parse <file.cb>` — Lex and parse a CleanBasic source file (`--dump-ast` prints the AST).
- `cb check <file.cb>` — Run lexing, parsing, module resolution, and semantic type-checking.
- `cb emit <file.cb> [-o out.go]` — Transpile a CleanBasic program to Go source code.
- `cb build <file.cb> [-o bin]` — Compile a CleanBasic program into a native executable binary.
- `cb run <file.cb> [--] [args...]` — Compile and run a CleanBasic program immediately, forwarding trailing arguments and `stdin`.

## Example: `cbgrep`

[`examples/cbgrep/main.cb`](examples/cbgrep/main.cb) is a CleanBasic `grep` utility using `std/cli`, `std/fs`, `std/os`, and `std/regex`.

### Supported Flags

- `-i`, `--ignore-case` — Case-insensitive matching
- `-n`, `--line-number` — Prefix matches with 1-based line numbers
- `-c`, `--count` — Print only the count of matching lines per target
- `-v`, `--invert-match` — Select non-matching lines
- `-F`, `--fixed-strings` — Treat pattern as a literal substring instead of a regex
- `-r`, `--recursive` — Recursively search directories (skipping `.git`)

### Running `cbgrep`

```bash
# Run directly with cb run
./bootstrap/cb run examples/cbgrep/main.cb -i -n "fn main" examples/cbgrep/main.cb

# Search standard input
cat README.md | ./bootstrap/cb run examples/cbgrep/main.cb -i "cleanbasic"

# Build a standalone binary
./bootstrap/cb build examples/cbgrep/main.cb -o cbgrep
./cbgrep -r -n "import" examples/ tests/cases/
```

## Running Tests

```bash
cd bootstrap
go test ./...
```
