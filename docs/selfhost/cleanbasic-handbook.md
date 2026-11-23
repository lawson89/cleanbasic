# CleanBasic Developer Handbook (Self-Hosting Edition)

This handbook is a practical, codebase-grounded reference for writing the self-hosted CleanBasic transpiler in CleanBasic (`selfhost/`). Every rule, signature, and Go mapping below is verified against `docs/cleanbasic_language_specification_go_backend_edition.md`, `bootstrap/internal/`, and `std/*.cb`.

---

## 1. Type System & Built-in Methods

### 1.1 Primitive, Container, and Composite Types

| CleanBasic Type | Go Mapping | Default Zero Value (`dim x as T`) | Comparable (`==`, `!=`, `select case`, `dict` key) |
| :--- | :--- | :--- | :--- |
| `int` | `int64` | `int64(0)` | Yes |
| `float` | `float64` | `float64(0)` | Yes |
| `str` | `string` | `""` | Yes |
| `bool` | `bool` | `false` | Yes |
| `byte` | `byte` | `byte(0)` | Yes |
| `err` | `error` | `nil` | No |
| `list[T]` | `[]Go(T)` | `nil` (safe for `.len()`, `.push()`, `for-in`) | No |
| `dict[K, V]` | `map[Go(K)]Go(V)` | `make` / `map[K]V{}` (non-nil, ready for writes) | No |
| `mailbox[T]` | `chan Go(T)` | `make(chan Go(T))` (unbuffered channel) | No |
| `type S` (struct) | `S` (or `<mod>__S`) | `S{}` with recursive `dict`/`mailbox` auto-init | Yes, if all fields are comparable and not `= go` |
| `enum E` | `int64`-backed `type E int64` | `E(0)` (first declared variant) | Yes |

> **Recursive Struct Field Auto-Initialization:**
> In `bootstrap/internal/codegen/codegen.go` (`structNeedsFieldInit` / `emitZeroStruct`) and `stmts.go` (`emitZeroOrInitVarDecl`), any zero-initialized CleanBasic struct—whether created via `dim s as S`, `S()` (0-arg constructor), or an error-return zero value—automatically initializes all `dict` fields to `map[K]V{}` and all `mailbox` fields to `make(chan T)`, **recursively** through nested struct fields. You do **not** need manual `s.table = dict[str, int]()` boilerplate for struct-owned dictionaries or mailboxes.

### 1.2 Indexing & Slicing

- `list[T]`: `xs[i]` (`int` index -> `T`), `xs[start:end]`, `xs[start:]`, `xs[:end]` (-> `list[T]`).
- `str`: `s[i]` (`int` index -> `byte`), `s[start:end]`, `s[start:]`, `s[:end]` (-> `str`).
- `dict[K, V]`: `d[k]` (`K` key -> `V`, returns zero value of `V` if absent; `d[k] = v` inserts/updates).

### 1.3 Built-in Functions & Methods

Verified against `bootstrap/internal/sema/builtins.go` and `bootstrap/internal/sema/sema.go`:

| Receiver / Global | Method / Call | Signature | Go Emission / Notes |
| :--- | :--- | :--- | :--- |
| Global | `print(msg)` | `(msg as str)` | Prints line to stdout (`fmt.Println`) |
| `list[T]` | `.len()` | `() as int` | `int64(len(xs))` |
| `list[T]` | `.push(val)` | `(val as T)` | `xs = append(xs, val)`; requires mutable addressable `list` |
| `list[str]` | `.join(sep)` | `(sep as str) as str` | `strings.Join(xs, sep)` |
| `list[T]` | `.sort()` | `()` | `slices.Sort(xs)` in-place; `T` must be `int`, `float`, `str`, or `byte`; requires mutable addressable `list` |
| `dict[K, V]` | `.len()` | `() as int` | `int64(len(d))` |
| `dict[K, V]` | `.get(k, defVal)` | `(k as K, defVal as V) as V` | Returns value or `defVal` if missing (`cbDictGet`) |
| `dict[K, V]` | `.contains(k)` | `(k as K) as bool` | `cbDictContains(d, k)` |
| `dict[K, V]` | `.remove(k)` | `(k as K)` | `delete(d, k)`; requires mutable addressable `dict` |
| `str` | `.len()` | `() as int` | `int64(len(s))` (byte length) |
| `str` | `.trim()` | `() as str` | `strings.TrimSpace(s)` |
| `str` | `.toUpper()` | `() as str` | `strings.ToUpper(s)` |
| `str` | `.toLower()` | `() as str` | `strings.ToLower(s)` |
| `str` | `.toStr()` | `() as str` | Identity (`s`) |
| `str` | `.quote()` | `() as str` | `strconv.Quote(s)` |
| `str` | `.toInt()` | `() as int can error` | `strconv.ParseInt(s, 10, 64)` (fallible) |
| `str` | `.toFloat()` | `() as float can error` | `strconv.ParseFloat(s, 64)` (fallible) |
| `str` | `.split(sep)` | `(sep as str) as list[str]` | `strings.Split(s, sep)` |
| `str` | `.contains(sub)` | `(sub as str) as bool` | `strings.Contains(s, sub)` |
| `str` | `.startsWith(pre)` | `(pre as str) as bool` | `strings.HasPrefix(s, pre)` |
| `str` | `.endsWith(suf)` | `(suf as str) as bool` | `strings.HasSuffix(s, suf)` |
| `str` | `.indexOf(sub)` | `(sub as str) as int` | `int64(strings.Index(s, sub))` (`-1` if absent) |
| `str` | `.replace(old, new)` | `(old as str, new as str) as str` | `strings.ReplaceAll(s, old, new)` |
| `int` | `.toByte()` | `() as byte` | `byte(n)` |
| `int` | `.toChar()` | `() as str` | `string(rune(n))` |
| `int` | `.toStr()` | `() as str` | `strconv.FormatInt(n, 10)` |
| `float` | `.toStr()` | `() as str` | `strconv.FormatFloat(f, 'g', -1, 64)` |
| `bool` | `.toStr()` | `() as str` | `strconv.FormatBool(b)` |
| `byte` | `.toInt()` | `() as int` | `int64(b)` |
| `byte` | `.toStr()` | `() as str` | `strconv.FormatInt(int64(b), 10)` |
| `mailbox[T]` | `.post(val)` | `(val as T)` | `ch <- val` |
| `mailbox[T]` | `.get()` | `() as T` | `(<-ch)` |
| `mailbox[T]` | `.close()` | `()` | `close(ch)` |
| `mailbox[T]` | `.len()` | `() as int` | `int64(len(ch))` |
| `err` | `.msg` | field `str` | `e.Error()` |

---

## 2. Declarations, Mutability, `byref`, UFCS & Modules

### 2.1 Top-Level & Local Declarations

- **`const` (top-level only):** `[public] const NAME = expr` or `[public] const NAME as T = expr`. Immutable everywhere.
- **`dim`:**
  - Top-level: `[public] dim name as T` or `[public] dim name = expr`.
  - Local: `dim name as T`, `dim name = expr`, or `dim name as T = expr`.
- **`type` (structs):**
  ```cleanbasic
  public type Point
      x as int
      y as int
  end type
  ```
  - **Recursive Struct Cycle Ban:** A struct cannot directly or transitively contain itself by value (`sema/decls.go` `checkRecursiveStructFields`), though it may contain `list[S]` or `dict[K, S]`.
  - **Positional-Only Constructors:** `Point()` (0-arg zero-initialized) or `Point(10, 20)` (all fields in declaration order). Partial/named constructors do not exist.
- **`enum`:** `[public] enum Color ... Red ... Green ... end enum`. Access variants via `Color.Red` (or `modAlias.Color.Red` cross-module).
- **`fn`:**
  ```cleanbasic
  public fn add(a as int, b as int) as int
      return a + b
  end fn
  ```
  Omit `as T` for `void`. Add `can error` before the newline for fallible functions. Parameters are passed by value unless prefixed with `byref`.
- **Go Interop:**
  - `go import "pkg"` or `go import "pkg" as alias`
  - `go ... end go` raw Go block at top level
  - `[public] type T = go "GoType"`
  - `[public] fn f(a as T) as R [can error] = go "goFunc"`

### 2.2 Mutability & `byref` Addressability Rules

Verified against `bootstrap/internal/sema/stmts.go` (`checkMutableTarget`):

- **Addressable (Valid for assignment, `byref`, `list.push`, `list.sort`):**
  1. Mutable local or top-level `dim` variables (and exported `public dim` module variables).
  2. Struct fields on an addressable struct target (`s.field`, `nested.a.b`).
  3. List elements on an addressable list target (`items[i]`, `arena.exprs[id].semaTypeId`).
- **Rejected (Compile-time `SemaError`):**
  - `const` declarations
  - `for` / `for-in` loop variables and `catch e` error variables (all marked `IsConst: true`)
  - `dict[k]` index expressions (Go maps are not addressable! Read into a temp, mutate, and assign back: `dim tmp = d[k]`; mutate `tmp`; `d[k] = tmp`)
  - `str[i]` byte indexing (strings are immutable)
  - Non-lvalue expressions (literals, function call results, slices)

### 2.3 UFCS & Method Resolution Order

When evaluating `target.name(args...)` (`bootstrap/internal/sema/exprs.go` `inferMethodOrUFCSCall`):
1. **Module-qualified call:** If `target` is an imported module alias, resolve `mod.StructName(...)` or `mod.fnName(...)` (requires `public`).
2. **Built-in method:** Check built-in methods on `target`'s type (`list`, `dict`, `str`, `int`, `float`, `bool`, `byte`, `mailbox`).
3. **Current module UFCS:** Look up `name` in the current module's functions where `params[0].Type.Equals(recvType)`.
4. **Defining module UFCS:** If `target`'s type was defined in another module `M`, look up `public fn name` in `M` where `params[0].Type.Equals(recvType)`.

If `params[0]` is `byref`, `target` must pass `checkMutableTarget`.

### 2.4 Module Imports & Visibility

- `import "relative/path"` (auto-aliases to last path segment, e.g. `"std/fs"` -> `fs`) or `import "path" as alias`.
- Only symbols marked `public` (`public fn`, `public type`, `public enum`, `public const`, `public dim`) are visible across module boundaries.
- **Emitted Go Naming:** Non-entry module symbols are sanitized as `<modID>__<SymbolName>` (e.g., `std_fs__readFile`, `token__Kind`), while entry module symbols keep their CleanBasic names.

---

## 3. Control Flow, Error Handling & `defer`

### 3.1 Control Flow

- **`if` / `else if` / `else` / `end if`:** Condition must be `bool`.
- **`while cond ... end while`:** Condition must be `bool`; supports `break` and `continue`.
- **`select case target ... case v1, v2 ... case else ... end select`:**
  - `target` must be comparable (`int`, `float`, `str`, `byte`, `bool`, `enum`, or comparable struct).
  - Case values must match `target`'s type. **No fallthrough.**
- **Numeric `for`:** `for i = start to end ... end for` (inclusive `start .. end`, `int` bounds, `i` is immutable).
- **`for-in`:**
  - `for item in listOrMailbox` / `for idx, item in listVal`
  - `for ch in strVal` / `for idx, ch in strVal` (yields 1-rune UTF-8 `str` values; `idx` is the UTF-8 byte offset)
  - `for k in dictVal` / `for k, v in dictVal`
  - All loop variables are immutable (`IsConst: true`).

### 3.2 Error Handling & The Top-Level-Only Fallible Call Rule

- Functions that may fail must declare `can error`: `fn parseFile(p as str) as Program can error`.
- Raise an error with `error("message")` (requires `try` block or `can error` function).
- Handle errors with:
  ```cleanbasic
  try
      dim content = fs.readFile(path)
  catch e
      os.eprintln("read failed: " + e.msg)
  end try
  ```
- **Top-Level-Only Fallible Call Rule (`sema/stmts.go`):** A `can error` call (or `.toInt()` / `.toFloat()`) may **only** appear as the **direct top-level expression** of:
  1. An expression statement: `fs.writeFile(p, text)`
  2. A `dim` initializer: `dim n = s.toInt()`
  3. An assignment RHS: `n = s.toInt()`
  4. A `return` value: `return fs.readFile(p)`
  5. A `start job` target (only if the spawned function is NOT `can error`—`start job` rejects `can error` functions).
  **Never** nest a `can error` call inside `if`, `while`, `print(...)`, binary operators, or another call's arguments!

### 3.3 `defer`

```cleanbasic
defer
    fs.removeAll(tmpDir)
end defer
```
- Executes LIFO when the enclosing function returns.
- `return` is forbidden inside `defer`, and `loopDepth` / `tryDepth` are reset to `0` inside `defer` (you cannot `break`/`continue` an outer loop or rely on an outer `try` from inside `defer`).

---

## 4. Standard Library API Reference

### 4.1 `std/fs` (17 public functions)

| Function | Signature | Notes |
| :--- | :--- | :--- |
| `readFile` | `(path as str) as str can error` | `os.ReadFile` |
| `writeFile` | `(path as str, content as str) can error` | `os.WriteFile(..., 0644)` |
| `exists` | `(path as str) as bool` | `os.Stat(path) == nil` |
| `mkdirAll` | `(path as str) can error` | `os.MkdirAll(path, 0755)` |
| `listDir` | `(path as str) as list[str] can error` | Entry names via `os.ReadDir` |
| `isDir` | `(path as str) as bool` | `info.IsDir()` |
| `isSymlink` | `(path as str) as bool` | `os.Lstat` symlink check |
| `join` | `(a as str, b as str) as str` | `filepath.Join(a, b)` |
| `base` | `(path as str) as str` | `filepath.Base` |
| `dir` | `(path as str) as str` | `filepath.Dir` |
| `ext` | `(path as str) as str` | `filepath.Ext` |
| `clean` | `(path as str) as str` | `filepath.Clean` |
| `abs` | `(path as str) as str can error` | `filepath.Abs` |
| `mkdirTemp` | `(pattern as str) as str can error` | `os.MkdirTemp("", pattern)` |
| `mkdirTempIn` | `(dir as str, pattern as str) as str can error` | `os.MkdirTemp(dir, pattern)` |
| `removeAll` | `(path as str) can error` | `os.RemoveAll` |
| `rel` | `(basePath as str, targPath as str) as str can error` | `filepath.Rel` |

### 4.2 `std/os` (12 public functions + `ExecResult`)

```cleanbasic
public type ExecResult
    exitCode as int
    stdout as str
    stderr as str
end type
```

| Function | Signature | Notes |
| :--- | :--- | :--- |
| `args` | `() as list[str]` | `os.Args[1:]` (excludes binary name) |
| `exit` | `(code as int)` | `os.Exit(int(code))` |
| `getEnv` | `(key as str) as str` | `os.Getenv` |
| `exec` | `(cmd as str, args as list[str]) as ExecResult can error` | Captures stdout/stderr; non-zero exit populates `exitCode` |
| `readStdin` | `() as str can error` | `io.ReadAll(os.Stdin)` |
| `cwd` | `() as str can error` | `os.Getwd` |
| `eprint` | `(msg as str)` | `fmt.Fprint(os.Stderr, msg)` |
| `eprintln` | `(msg as str)` | `fmt.Fprintln(os.Stderr, msg)` |
| `writeStdout` | `(msg as str)` | `fmt.Fprint(os.Stdout, msg)` (no trailing newline) |
| `execInDir` | `(dir as str, cmd as str, args as list[str]) as int can error` | Streams stdio, returns exit code |
| `cacheDir` | `() as str` | `os.UserCacheDir()` (fallback `os.TempDir()`) |
| `executable` | `() as str can error` | `os.Executable` |

### 4.3 `std/cli` (`Parser`, `ParsedArgs`, 10 public functions)

- Types: `public type Parser`, `public type ParsedArgs` (`helpRequested as bool`, `positionals as list[str]`, `flags as dict[str, bool]`, `options as dict[str, str]`, `args as dict[str, str]`).
- Functions:
  - `newParser(name as str, version as str, description as str) as Parser`
  - `addFlag(byref p as Parser, name as str, short as str, desc as str)`
  - `addOption(byref p as Parser, name as str, short as str, desc as str, defaultVal as str)`
  - `addArg(byref p as Parser, name as str, desc as str)`
  - `printUsage(p as Parser)`
  - `parse(p as Parser) as ParsedArgs can error`
  - `parseArgs(p as Parser, argv as list[str]) as ParsedArgs can error`
  - `hasFlag(a as ParsedArgs, name as str) as bool`
  - `getOption(a as ParsedArgs, name as str) as str`
  - `getArg(a as ParsedArgs, name as str) as str`

### 4.4 `std/regex` (`Regex`, 6 public functions)

- Opaque type: `public type Regex = go "CbStdRegex"`
- Functions:
  - `compile(pattern as str) as Regex can error`
  - `match(pattern as str, text as str) as bool can error`
  - `isMatch(r as Regex, text as str) as bool`
  - `find(r as Regex, text as str) as str`
  - `findAll(r as Regex, text as str) as list[str]`
  - `replaceAll(r as Regex, text as str, repl as str) as str`

---

## 5. Compiler-Writing Idioms & Gotchas

### 5.1 Flat Arena AST Pattern (`AstArena`)

CleanBasic has **no interfaces, no sum types, no pointers, and no recursive value structs**. Instead of Go's AST interface hierarchy (`ast.Expr`, `ast.Stmt`, `*ast.CallExpr`), store all AST nodes, types, symbols, scopes, and modules in flat `list[T]` pools inside a single `AstArena` struct:

- **Integer IDs (`-1` = none):** Child nodes, sibling lists (`list[int]`), and symbol table entries (`dict[str, int]`) store `int` indices into the arena.
- **Addressable In-Place Mutation:** `list[T]` elements are directly addressable, so sema can annotate `arena.exprs[id].semaTypeId = tyId` or pass `byref arena as AstArena` without value-copy bugs.
- **Tagged-Union Dispatch:** Codegen and sema switch on `select case arena.exprs[id].kind` with zero heap allocations per node.

See `docs/selfhost/self-hosting-plan.md` Section 3 for the complete `AstArena` type blueprint.

### 5.2 `O(N)` StringBuilder Pattern

Never concatenate Go output in a loop with `out = out + line` (which is `O(N^2)`). Instead, accumulate chunks in a `list[str]` and join once:

```cleanbasic
dim buf as list[str]
buf.push("package main\n\n")
for lineIdx = 0 to lines.len() - 1
    buf.push(lines[lineIdx])
    buf.push("\n")
end for
dim goSrc = buf.join("")
```

### 5.3 Deterministic `import` Emission with `list.sort()`

Go `map` iteration order is randomized. To ensure Stage 2 and Stage 3 emitted Go files are byte-for-byte identical (fixed-point bootstrap), **never** emit Go code by iterating directly over a `dict`. Instead:
1. Track used imports in `dict[str, str]` (path -> alias) or `dict[str, bool]`.
2. Collect keys into a `list[str]`.
3. Call `keys.sort()` (now built into CleanBasic for `list[str]`).
4. Iterate `keys` in sorted order to emit `import (...)` lines, struct/fn declarations, and runtime helpers.

### 5.4 Copy-Modify-Assign for `dict[k]` Structs vs. `dict[str, int]` Arena IDs

Because `dict[k]` is **not addressable**, you cannot pass `symTable[name]` to `byref` or mutate `symTable[name].field = val`:
- **Best pattern:** Store `int` arena IDs in `dict[str, int]` (e.g., `scope.vars[name] = varSymId`), then mutate `arena.varSyms[varSymId].goName = ...`.
- **Fallback if storing structs in a `dict`:** Read into a local `dim tmp = d[k]`, mutate `tmp`, and write back `d[k] = tmp`.

### 5.5 Forbidden Go Reserved Words (`bootstrap/internal/parser/parser.go`)

The bootstrap parser rejects Go keywords in **all** identifier positions (including struct fields/methods via `expectMemberIdent`), and rejects all 34 Go reserved/predeclared words in variable/function/type/param positions via `expectIdent`:

- **`goKeywords` (12 — forbidden everywhere, even member names):**
  `chan`, `default`, `fallthrough`, `func`, `goto`, `interface`, `map`, `package`, `range`, `struct`, `switch`, `var`
- **`goReservedWords` (34 — includes all 12 `goKeywords` plus 22 Go predeclared/stdlib identifiers, forbidden as top-level/local/param/type/fn identifiers):**
  `any`, `append`, `close`, `copy`, `delete`, `error`, `errors`, `float64`, `fmt`, `int64`, `len`, `make`, `new`, `nil`, `panic`, `println`, `recover`, `rune`, `slices`, `strconv`, `string`, `strings`

> **Tip:** Avoid `str`, `type`, `fn`, `err`, `len`, `map`, `range`, `default`, `var`, `func`, `error`, `string` when naming variables or struct fields in `selfhost/`. Prefer `ty`, `fnSym`, `errMsg`, `count`, `table`, `isRange`, `defVal`, `text`.
