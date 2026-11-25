# CleanBasic

A statically typed, readable BASIC dialect that transpiles to clean, single-file Go with zero external dependencies.

**Project Status:** The Go bootstrap compiler (`cb`) is complete and passing all tests. Self-hosting the compiler in CleanBasic is in progress (see the [Self-Hosting Plan](docs/selfhost/self-hosting-plan.md)).

## Why CleanBasic?

- **Readable BASIC syntax, Go performance:** Familiar block-delimited syntax (`fn ... end fn`, `if ... end if`, `type ... end type`) lowered 1:1 to idiomatic Go and compiled to native binaries, with explicit numeric conversions (`.toFloat()`, `.toInt()`, `.toStr()`) and `print(...)` across all primitive types.
- **Zero-dependency toolchain:** Both the bootstrap compiler and emitted Go code rely strictly on the Go standard library.
- **Explicit error handling & race-free concurrency:** `can error` / `try ... catch` error propagation, `defer`, and value-isolated `start` / `job[T]` tasks (`.wait()`, `.waitTimeout(ms)`, `.isDone()`) with explicit cooperative `cancelToken` cancellation (zero buffer sizing, race-free by construction).
- **Direct Go interop:** Import any Go package with `go import "pkg" [as alias]` or embed Go bindings cleanly when needed.

## Example

```cleanbasic
type Task
    name as str
    done as bool
end type

fn label(t as Task) as str
    if t.done
        return "[x] " + t.name
    end if
    return "[ ] " + t.name
end fn

fn worker(id as int) as str
    dim t = Task("compile module " + id.toStr(), true)
    return t.label()
end fn

fn main()
    dim tok = cancelToken()
    dim j1 = start worker(1)
    dim j2 = start worker(2)

    try
        print(j1.waitTimeout(5000))
    catch e
        tok.cancel()
        print("timed out: " + e.msg)
    end try
    print(j2.wait())
end fn
```

## Quickstart

**Prerequisites:** Go 1.23+

Build the `cb` bootstrap compiler from the repository root:

```bash
go build -C bootstrap -o cb ./cmd/cb
```

### Commands

- `cb check <file.cb>` — Run lexing, parsing, module resolution, and semantic type-checking.
- `cb emit <file.cb> [-o out.go]` — Transpile a CleanBasic program to Go source code.
- `cb build <file.cb> [-o bin]` — Compile a CleanBasic program into a native executable binary.
- `cb run <file.cb> [--] [args...]` — Compile and run a CleanBasic program immediately, forwarding trailing arguments and `stdin`.

New to CleanBasic? Start with the [11-Exercise CleanBasic Tutorial](tutorial/README.md).

## Example: `cbgrep`

[`examples/cbgrep/main.cb`](examples/cbgrep/main.cb) is a CleanBasic `grep` utility using `std/cli`, `std/fs`, `std/os`, `std/regex`, and bounded parallel workers (`start` / `job[T]` / `cancelToken` / `waitTimeout`).

### Supported Flags

- `-i`, `--ignore-case` — Case-insensitive matching
- `-n`, `--line-number` — Prefix matches with 1-based line numbers
- `-c`, `--count` — Print only the count of matching lines per target
- `-v`, `--invert-match` — Select non-matching lines
- `-F`, `--fixed-strings` — Treat pattern as a literal substring instead of a regex
- `-r`, `--recursive` — Recursively search directories (skipping `.git`)
- `-q`, `--quiet` — Suppress normal output and cancel remaining workers on first match
- `-t`, `--timeout <ms>` — Maximum wait time in milliseconds per parallel batch (`0` = no timeout)

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

## Documentation

- [11-Exercise CleanBasic Tutorial](tutorial/README.md)
- [Language Specification (Go Backend Edition)](docs/cleanbasic_language_specification_go_backend_edition.md)
- [Self-Hosting Handbook](docs/selfhost/cleanbasic-handbook.md)
- [Self-Hosting Implementation Plan](docs/selfhost/self-hosting-plan.md)
- [Go Bootstrap Guidelines](docs/golang-bootstrap-guidelines.md)

## License

Licensed under the [Apache License 2.0](LICENSE).
