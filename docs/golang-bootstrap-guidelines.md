# CleanBasic Bootstrap Transpiler — Agent Guidelines

## Project Goal
Build a clean, fast bootstrap transpiler (`cb`) written in Go (1.22+) that compiles CleanBasic (`.cb`) source files into a single procedural Go file (`main.go`), which is then compiled/executed by the standard Go toolchain (`go build` / `go run`).

## Authoritative Specification
- Always consult `docs/cleanbasic_language_specification_go_backend_edition.md` as the single source of truth for syntax, EBNF grammar, semantics, and type rules.
- Do NOT implement legacy C++ features (no C++ headers, no `EXPECT_CPP` assertions, no LSP server in the bootstrap compiler).

## Architectural & Coding Rules
1. **Zero External Dependencies:** Use ONLY the Go standard library (`fmt`, `os`, `strings`, `strconv`, `path/filepath`, `os/exec`) in both the transpiler codebase and the emitted `main.go` output.
2. **Self-Hosting Ready Style:** Write flat, procedural, data-oriented Go code. Use plain structs, tagged union interfaces/enums, and free functions. Avoid complex reflection, deep OOP hierarchies, or visitor-pattern boilerplate so the transpiler can be easily ported to CleanBasic itself later.
3. **Permanent Language Simplifications:**
   - **One-Way Bottom-Up Inference:** Expressions compute their own types bottom-up. Untyped empty collection literals (`dim x = []` or `foo([])`) are compile errors; require `dim x as list[int] = []`.
   - **Local & Prelude UFCS Only:** `obj.method(args)` resolves ONLY to methods in the built-in prelude (`str`, `list`, `dict`) or functions declared in the *same* `.cb` module. Cross-module calls must be module-qualified (`modAlias.fnName(obj, args)`).
   - **Lexer-Desugared String Interpolation:** `$"Hello {user.name}!"` restricts `{...}` to dotted identifier paths (`QualifiedIdent`) and simple `[index]` access (`{parts[0]}` / `{parts[idx]}`), and is desugared directly in the lexer into `"Hello " & user.name & "!"` tokens.
4. **Go Target Emitter Hygiene:**
   - CleanBasic `int` maps to `int64`; `float` maps to `float64`; `str` maps to `string`; `bool` maps to `bool`.
   - Emit `_ = varName` after local variable declarations so Go's unused-variable rule never breaks valid CleanBasic programs.
   - Track used standard library packages (`fmt`, `strings`, `os`, etc.) during code generation and emit only the imports actually referenced in `main.go`.
   - Lower `can error` functions to `(T, error)` (or `error` for void), `defer ... end defer` to `defer func() { ... }()`, and `try / catch` to scoped `{ ... }` blocks with `goto _catch_N`.

## Package Layout
- `cmd/cb/main.go` — CLI entrypoint (`cb run <file.cb>`, `cb build <file.cb> [-o out]`, `cb check <file.cb>`, `--dump-ast`)
- `internal/token/` — Token definitions, keyword lookup, and two-word keyword coalescing (`else if`, `end fn`, `select case`, etc.)
- `internal/ast/` — AST node definitions for declarations, statements, expressions, and types
- `internal/lexer/` — Scanner with line/col tracking, `'` and `#` comments, and `$"..."` desugaring
- `internal/parser/` — Recursive-descent parser matching the EBNF grammar
- `internal/sema/` — Symbol tables, module import graph, type checking, mutability, and fallibility verification
- `internal/codegen/` — Single-file procedural Go code emitter

## Verification Workflow
- Run `go test ./...` after every change.
- Never consider a phase complete until all existing and phase-specific `.cb` tests in `tests/cases/` pass.