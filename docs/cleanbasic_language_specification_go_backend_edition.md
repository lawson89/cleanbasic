# CleanBasic Language Blueprint & Transpiler Architecture

CleanBasic is a statically typed, procedural systems programming language. It combines the readable, low-friction syntax of Visual Basic with the fast compilation, cross-platform builds, and memory safety of Go (and TinyGo for bare-metal targets). By enforcing flat data structures, value semantics, eliminating object-oriented inheritance, and using local Unified Function Call Syntax (UFCS), it provides scripting-language ergonomics with clean, predictable transpilation.

### Transpiler Internals & Architecture Suite (Go Bootstrap)

The CleanBasic bootstrap transpiler is structured as a clean, multi-package Go pipeline (`cmd/cb` and `internal/...`):

* `internal/token` & `internal/lexer` — Tokenization, two-word keyword coalescing (`else if`, `end fn`), and forbidden C-style logical operators (`&&`, `||`, `!`).

* `internal/parser` — Recursive-descent parser emitting a flat AST with strict bottom-up type rules.

* `internal/sema` — Module graph topological sorter, `GlobalSymbolTable`, local/prelude UFCS resolution, mutability enforcement (`const`/`dim`/`byref`), and static `can error` / `try / catch` verification.

* `internal/codegen` — Unity Go emitter generating a single, idiomatic `build.go` file with minimal runtime helpers and zero unused-import/unused-variable errors.

## 1. Core Architecture

* **Unity Go Backend:** The transpiler converts CleanBasic into a single procedural `build.go` file, leveraging Go's instant compilation, native garbage collection, multiple return values, and built-in cross-compilation (`GOOS`/`GOARCH` and TinyGo).

* **Procedural Purity & Value Semantics:** CleanBasic strictly bans classes, inheritance, exceptions, and raw user pointers. Structs and primitives use value semantics by default, explicit `byref` pointers (`*T`) for mutation, and local/prelude Unified Function Call Syntax (UFCS).

* **Streamlined Static Rules (No Bidirectional Inference):** Expressions compute their types strictly bottom-up. Untyped empty collection literals (`[]`, `{}`) are disallowed; empty collections must declare their type (`dim nums as list[int] = []`) or use typed constructors (`list[int]()`).

* **English-Over-Symbols:** C-style logical operators (`&&`, `||`, `!`) are forbidden at the lexer level and replaced with `and`, `or`, and `not`.

## 2. The CleanBasic EBNF Grammar

```
Program         ::= TopLevelDecl*

TopLevelDecl    ::= ImportDecl | GoImportDecl | GoBlockDecl | ConstDecl | TypeDecl | EnumDecl | FnDecl

ImportDecl      ::= "import" StringLiteral ["as" Identifier] NEWLINE
GoImportDecl    ::= "go import" StringLiteral ["as" Identifier] NEWLINE
GoBlockDecl     ::= "go" NEWLINE <verbatim Go code> "end go" NEWLINE

ConstDecl       ::= ["public"] "const" Identifier ["as" Type] "=" Expr NEWLINE

EnumDecl        ::= ["public"] "enum" Identifier NEWLINE Identifier* "end enum" NEWLINE


TypeDecl        ::= ["public"] "type" Identifier "=" "go" StringLiteral NEWLINE
                  | ["public"] "type" Identifier NEWLINE FieldDecl* "end type" NEWLINE
FieldDecl       ::= Identifier "as" Type NEWLINE

FnDecl          ::= ["public"] "fn" Identifier "(" [ParamList] ")" ["as" Type] ["can error"] "=" "go" StringLiteral NEWLINE
                  | ["public"] "fn" Identifier "(" [ParamList] ")" ["as" Type] ["can error"] NEWLINE
                    Statement*
                    "end fn" NEWLINE
ParamList       ::= Param ("," Param)* [","]
Param           ::= ["byref"] Identifier "as" Type

Statement       ::= VarDecl | Assignment | ExprStmt
                  | IfStmt | ForStmt | WhileStmt | SelectStmt
                  | TryCatchStmt | DeferStmt | ReturnStmt | BreakStmt | ContinueStmt

VarDecl         ::= "dim" Identifier ["as" Type] "=" Expr NEWLINE
Assignment      ::= Expr "=" Expr NEWLINE
ExprStmt        ::= Expr NEWLINE

TryCatchStmt    ::= "try" NEWLINE Statement* "catch" Identifier NEWLINE Statement* "end try" NEWLINE
DeferStmt       ::= "defer" NEWLINE Statement* "end defer" NEWLINE

IfStmt          ::= "if" Expr NEWLINE Statement*
                    ("else if" Expr NEWLINE Statement*)*
                    ["else" NEWLINE Statement*]
                    "end if" NEWLINE

ForStmt         ::= "for" Identifier "=" Expr "to" Expr NEWLINE
                    Statement*
                    "end for" NEWLINE
                  | "for" IdentList "in" Expr NEWLINE
                    Statement*
                    "end for" NEWLINE

WhileStmt       ::= "while" Expr NEWLINE
                    Statement*
                    "end while" NEWLINE

SelectStmt      ::= "select case" Expr NEWLINE
                    ("case" ExprList NEWLINE Statement*)*
                    ["case else" NEWLINE Statement*]
                    "end select" NEWLINE
ExprList        ::= Expr ("," Expr)*

StartJobStmt    ::= "start job" CallExpr NEWLINE
ReturnStmt      ::= "return" [Expr] NEWLINE
BreakStmt       ::= "break" NEWLINE
ContinueStmt    ::= "continue" NEWLINE

Expr            ::= OrExpr
OrExpr          ::= AndExpr ("or" AndExpr)*
AndExpr         ::= NotExpr ("and" NotExpr)*
NotExpr         ::= "not" NotExpr | CmpExpr
CmpExpr         ::= AddExpr (("==" | "!=" | "<" | ">" | "<=" | ">=") AddExpr)*
AddExpr         ::= MulExpr (("+" | "-") MulExpr)*
MulExpr         ::= UnaryExpr (("*" | "/" | "%") UnaryExpr)*
UnaryExpr       ::= ("+" | "-") UnaryExpr | PostfixExpr

PostfixExpr     ::= PrimaryExpr
                    ( "." Identifier
                    | "[" Expr "]"
                    | "[" [Expr] ":" [Expr] "]"
                    | "(" [CallArgList] ")" )*

PrimaryExpr     ::= Literal | QualifiedIdent | TypedEmptyCtor | "(" Expr ")"

TypedEmptyCtor  ::= ("list" "[" Type "]" | "dict" "[" Type "," Type "]") "(" ")"
CallArgList     ::= Expr ("," Expr)* [","]

Literal         ::= NumberLiteral | StringLiteral
                  | "true" | "false"
                  | NonEmptyListLiteral | NonEmptyDictLiteral
NonEmptyListLiteral ::= "[" Expr ("," Expr)* [","] "]"
NonEmptyDictLiteral ::= "{" DictEntry ("," DictEntry)* [","] "}"
DictEntry       ::= Expr ":" Expr

Type            ::= "int" | "float" | "str" | "bool" | "byte" | "err"
                  | "list" "[" Type "]"
                  | "dict" "[" Type "," Type "]"
                  | QualifiedIdent

IdentList       ::= Identifier ("," Identifier)*
QualifiedIdent  ::= Identifier ("." Identifier)*

```

## 3. Streamlined Language Rules & Code Examples

### 1. Multi-File Modules, Explicit Qualification & Name Mangling

Every `.cb` file is implicitly a module. Everything is private by default; the `public` keyword exposes types, enums, constants, and functions to the importer.

* **Relative Nested Paths:** Module imports (`import "entities/hero.cb"`) are resolved relative to the importing file and normalized to canonical module IDs (`entities/hero`).

* **Default & Explicit Aliases (`as`):** `import "entities/hero.cb"` binds the alias `hero`. Use `import "input/button.cb" as inputBtn` to disambiguate colliding filenames.

* **Go Identifier Mangling & Native Go Interop:** Declarations emit with collision-free mangled Go identifiers (`cb_<sanitizedModuleId>__<name>`, e.g., `cb_entities_hero__Player`). Native Go interop uses `go import "pkg" [as alias]`, verbatim top-level `go ... end go` blocks, `[public] type Name = go "GoType"`, and `[public] fn name(...) ... = go "target"` (supporting native `error` returns for `can error` bindings).

* **Defining-Module & Local UFCS:** Unified Function Call Syntax (`obj.method(arg)`) resolves against the **built-in prelude** (`str`, `int`, `byte`, `list`, `dict`), functions declared in the **current module**, and `public fn`s declared in the **defining module** of `obj`'s `type` or `enum`. Unrelated external module functions must still be called with their explicit module alias (`mod.helper(arg)`).

**player.cb (The Library Module)**

```
public type Player
    name as str
    health as int
end type

public fn heal(byref p as Player, amount as int)
    p.health = p.health + amount
end fn

fn calculateMaxHealth() as int
    return 100
end fn

```

### 2. One-Way Type Inference, Mutability & Typed Containers

CleanBasic enforces strict bottom-up type inference:

* **Inferred Declarations:** `dim x = 10` and `dim hero = player.Player("Alice", 10)` infer their type directly from the right-hand expression.

* **Explicit Empty Collections:** Untyped `[]` and `{}` literals are forbidden. Empty containers require either an explicit variable type (`dim scores as dict[str, int] = {}`) or a typed constructor (`dim scores = dict[str, int]()`).

**main.cb (The Executable Module)**

```
import "player.cb"

' Local helper in the same module CAN be called via UFCS
fn isAlive(p as player.Player) as bool
    return p.health > 0
end fn

fn main() as int
    dim maxPotion = 50
    dim hero = player.Player("Alice", 10)

    ' Cross-module function uses explicit module prefix:
    player.heal(hero, maxPotion)

    ' Same-module function supports UFCS:
    if hero.isAlive()
        print("Hero " + hero.name + " has " + hero.health.toStr() + " HP")
    end if

    ' Empty collections require typed constructors:
    dim emptyItems = list[str]()
    dim lookup = dict[str, int]()
    return 0
end fn

```

### 3. Error Handling & Cleanup (Go Native Returns & Defer)

Functions declare fallibility by adding `can error` to their signature (`fn connect(ip as str) as Connection can error`), lowering directly to Go's `(Connection, error)` multiple return values.

1. **Implicit Bubbling:** Calling a `can error` function inside another `can error` function automatically emits `if _err != nil { return zeroVal, _err }`.

2. **Deterministic Cleanup (`defer`):** `defer ... end defer` blocks lower directly to Go's native `defer func() { ... }()`.

3. **Exception-less `try / catch` Blocks:** Lowers to a stack-local `var _tryErr error` block using `goto _catch_N` so `return`, `break`, and `continue` statements preserve their enclosing function and loop scopes.

**network.cb (Error Bubbling & Cleanup)**

```
public type Connection
    id as int
end type

public fn connect(ip as str) as Connection can error
    if ip == ""
        error("IP cannot be empty")
    end if
    return Connection(1)
end fn

public fn closeConnection(conn as Connection)
    ' Cleanup logic
end fn

public fn processNetwork() as int can error
    dim conn = connect("192.168.1.1")
    defer
        closeConnection(conn)
    end defer
    return 100
end fn

fn startServer()
    try
        dim records = processNetwork()
        print("Connected and processed " + records.toStr() + " records.")
    catch e
        print("Failed to start server: " + e.msg)
    end try
end fn

```

### 4. `byte`, Slicing, 2-Var Loops & `+` String Concatenation

* **Unified `+` Concatenation & `.toStr()`:** The `+` operator supports both numeric addition (`int`, `float`, `byte`) and string concatenation (`str + str`). Primitives (`int`, `float`, `bool`, `byte`, `str`) provide `.toStr()` for explicit string conversion.

* **`byte` Primitive Type & String Byte Indexing:** `byte` lowers to Go `byte` (`uint8`), `list[byte]` lowers to `[]byte`, and `int.toByte()` and `byte.toInt()` convert explicitly. Read-only string indexing `s[i]` returns `byte` directly via Go indexing.

* **Bounds-Safe Slicing:** `x[start:end]`, `x[start:]`, `x[:end]`, and `x[:]` on `str` and `list[T]` clamp out-of-bounds or negative indices without panicking and evaluate as r-values.

* **2-Variable `for-in` Loops & `byref` Addressability/Hygiene:** `for i, v in myList` binds `i as int` and `v` as the element type; `for ch in myStr` and `for byteIdx, ch in myStr` iterate UTF-8 runes as 1-rune `str` values. Mutable variables, struct fields (`s.field`), and list elements (`items[i]`, `node.children[i]`) may be passed to `byref` parameters and `byref` UFCS receivers; `dict` and `str` index expressions are not addressable and are rejected. Local `dim`, `for` loop variables, and `catch` variables cannot shadow an enclosing function's `byref` parameter.

```
fn demoContainers() as int
    dim nums = [10, 20, 30]
    dim scores = {"alice": 100, "bob": 85}

    dim total = 0
    for i = 0 to nums.len() - 1
        total = total + nums[i]
    end for

    for x in nums
        print("Item: ", x)
    end for

    dim aliceScore = scores["alice"]
    select case aliceScore
        case 100
            print("Perfect score! Total: " + total.toStr())
        case else
            print("Keep trying")
    end select
    return 0
end fn

```

### 5. Declarative CLI Parsing (`std/cli`) and `''` Doc-Comments

CleanBasic includes a pure-CleanBasic command-line parser (`import "std/cli"`) and `''` doc-comments for documentation extraction (`tools/cbdoc.cb`) and formatting (`tools/cbfmt.cb`).

```
import "std/cli"

'' Searches files for a matching pattern.
public fn runSearch() as int
    dim p = cli.newParser("cbgrep", "1.0", "Search for pattern in files")
    p.addFlag("ignore-case", "i", "Case-insensitive search")
    p.addOption("output", "o", "Output file path", "out.txt")
    p.addArg("pattern", "Substring pattern to search for")

    try
        dim args = p.parse()
        if args.helpRequested
            p.printUsage()
            return 0
        end if
        return 0
    catch e
        print(e.msg)
        p.printUsage()
        return 2
    end try
end fn

```

### 6. Structured Concurrency (`mailbox[T]`, `start job`)

CleanBasic provides structured, data-race-safe concurrency built around typed message queues and goroutine-backed `start job` statements:

* **Typed Mailboxes (`mailbox[T]`):** Lowered 1:1 to Go `chan T`. Created with `mailbox[T]()` (`make(chan T)`, unbuffered) or `mailbox[T](cap)` (`make(chan T, cap)`).
  * `box.post(msg as T)` — sends `msg` (`box <- msg`, infallible).
  * `box.get() as T` — blocking receive (`(<-box)`, infallible).
  * `box.close()` (`close(box)`), `box.count() as int` (`int64(len(box))`), and `for msg in box ... end for` (`for msg := range box`).
* **Background Jobs (`start job` Statement):**
  * `start job fnCall(...)` is a standalone statement that lowers 1:1 to `go fnCall(...)`.
  * Target must be a user or module function/UFCS call; `can error` functions and `byref` parameters are rejected at compile time. Results and completion signals are communicated via `mailbox[T]`.

### 7. Pure Value Structs & Recursive Trees (`list[T]`)

User-defined `type` structs map 1:1 to native Go value structs with zero hidden pointer indirection:

* Structs are always passed and assigned by value, guaranteeing value-copy isolation and zero-nil safety.
* Direct or mutually recursive value cycles (e.g. `Node -> Node` or `Expr <-> Block`) are rejected at compile time with a clear diagnostic.
* Recursive trees, ASTs, and linked structures model child nodes using `list[T]` (or `dict[K, T]`), which provide fixed-size reference headers in Go.
* Structs containing `list` or `dict` fields cannot be compared directly with raw `==` or `!=`.
### 8. `go.mod` Passthrough & Persistent Build Workspace

When running `cb build` or `cb run`, the CLI searches upward from the entry `.cb` file (and imported local modules, ignoring `bootstrap/go.mod` outside `bootstrap/`) for a project `go.mod` and `go.sum`. It syncs them into a persistent per-project workspace under the user cache directory (rewriting relative `replace` paths to absolute project paths) so CleanBasic projects can depend on external or local Go modules seamlessly.
