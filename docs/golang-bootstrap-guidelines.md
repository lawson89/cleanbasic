# CleanBasic Bootstrap Transpiler — Agent Guidelines

## Project Goal
Build a clean, fast bootstrap transpiler (`cb`) written in Go (1.22+) that compiles CleanBasic (`.cb`) source files into a single procedural Go file (`main.go`), which is then compiled/executed by the standard Go toolchain (`go build` / `go run`).

## Authoritative Specification
- Always consult `docs/cleanbasic_language_specification_go_backend_edition.md` as the single source of truth for syntax, EBNF grammar, semantics, and type rules.
- Do NOT implement legacy C++ features (no C++ headers, no `EXPECT_CPP` assertions, no LSP server in the bootstrap compiler).

## Architectural & Coding Rules
1. **Zero External Dependencies:** Use ONLY the Go standard library (`fmt`, `os`, `strings`, `strconv`, `path/filepath`, `os/exec`, `slices`, `maps`, etc.) in both the transpiler codebase and the emitted `main.go` output.
2. **Idiomatic Go for Clarity, Bounded by Self-Hosting Portability:** Write idiomatic, readable Go (1.22+) so the bootstrap compiler is as clear and maintainable as possible (using standard library helpers like `slices`, `maps`, `strings`, `fmt`, receiver methods that map cleanly to CleanBasic UFCS, and clean data structures). Avoid Go features that CleanBasic does not support and has no straightforward self-hosting mapping—specifically: runtime `reflect`, complex interface hierarchies / visitor-pattern double-dispatch, ubiquitous closures capturing mutable outer scope, channel/goroutine concurrency inside the compiler pipeline, or `unsafe`. (Existing shallow AST marker interfaces `ast.Expr` / `ast.Statement` / `ast.TopLevelDecl` are fine in Go because they map cleanly to tagged/arena structs in CleanBasic.) Also avoid ternary-like simulated one-liners where explicit `if/else` blocks are clearer.
3. **Permanent Language Simplifications:**
   - **One-Way Bottom-Up Inference:** Expressions compute their own types bottom-up. Empty collection literals (`[]`, `{}`) are rejected; use typed constructors `list[T]()` / `dict[K, V]()`.
   - **Defining-Module & Local UFCS:** `obj.method(args)` resolves to methods in the built-in prelude (`str`, `int`, `float`, `bool`, `byte`, `list`, `dict`, `mailbox`, `err`), functions declared in the *same* `.cb` module, or `public fn`s declared in the defining module of `obj`'s `type`/`enum`.
   - **Addressable `byref` Arguments & Shadowing Hygiene:** Mutable variables (including `public dim` module variables), struct fields (`s.field`), and list elements (`list[i]`) may be passed to `byref` parameters; `dict` and `str` index expressions are rejected. Inner-scope `dim`, `for` loop variables, and `catch` variables may shadow an enclosing function's `byref` parameter (`IdentExpr.ByRef` is resolved per reference in `sema`).
4. **Go Target Emitter Hygiene & Stdlib Modules:**
   - CleanBasic `int` maps to `int64`; `float` maps to `float64`; `str` maps to `string`; `bool` maps to `bool`; `byte` maps to `byte` (`uint8`); `list[byte]` maps to `[]byte`.
   - Standard library modules (`std/fs`, `std/os`, `std/cli`, `std/regex`) live as real `.cb` files in `std/` using CleanBasic Go interop (`go import`, `go ... end go`, `type ... = go "..."`, `fn ... = go "..."`) or pure CleanBasic (`std/cli.cb`).
   - Emit `_ = varName` after local variable declarations so Go's unused-variable rule never breaks valid CleanBasic programs.
   - Emit column-1 `//line <filepath>:<line>` directives before statements by default so Go stack traces and debuggers point directly to `.cb` source lines.
   - Track used standard library packages (`fmt`, `strings`, `os`, etc.) and `go import` declarations during code generation and emit only the imports actually referenced in `main.go`.
   - Lower `can error` functions to `(T, error)` (or `error` for void), `defer ... end defer` to `defer func() { ... }()`, and `try / catch` to scoped `{ ... }` blocks with `goto _catch_N`.

## Package Layout
- `cmd/cb/main.go` — CLI entrypoint (`cb run <file.cb>`, `cb build <file.cb> [-o out]`, `cb emit <file.cb> [-o out.go]`, `cb check <file.cb>`)
- `internal/token/` — Token definitions, keyword lookup, and two-word keyword coalescing (`else if`, `end fn`, `go import`, `end go`, `select case`, etc.)
- `internal/ast/` — AST node definitions for declarations, statements, expressions, and types
- `internal/lexer/` — Scanner with line/col tracking, `//` comments, single-quoted `byte` literals, and `go ... end go` raw blocks
- `internal/parser/` — Recursive-descent parser matching the EBNF grammar
- `internal/sema/` — Symbol tables, module import graph (including `std/*.cb` discovery), type checking, mutability, `IdentExpr.ByRef` resolution, and fallibility verification
- `internal/codegen/` — Single-file procedural Go code emitter with `//line` directives
- `../std/` — Standard library modules (`fs.cb`, `os.cb`, `cli.cb`, `regex.cb`)

## Verification Workflow
- Run `go test ./...` after every change.
- Never consider a phase complete until all existing and phase-specific `.cb` tests in `tests/cases/` pass.
