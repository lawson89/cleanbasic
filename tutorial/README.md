# CleanBasic Tutorial: 11 Progressive Exercises

This tutorial is a hands-on introduction to CleanBasic. Each exercise has:
- a compilable starter file under `exercises/` with `// TODO:` comments, and
- a complete reference solution under `solutions/`.

## Setup

Build the bootstrap compiler once from the repository root:

```bash
go build -C bootstrap -o cb ./cmd/cb
```

Then type-check or run any starter file or solution:

```bash
# Type-check and run an exercise starter
./bootstrap/cb check tutorial/exercises/ex01_variables_and_arithmetic.cb
./bootstrap/cb run tutorial/exercises/ex01_variables_and_arithmetic.cb

# Or run the reference solution directly
./bootstrap/cb run tutorial/solutions/ex01_variables_and_arithmetic.cb
```

## Exercise Index

| # | Topic | Starter | Solution |
| --- | --- | --- | --- |
| 1 | Variables, Constants, and Arithmetic | [ex01_variables_and_arithmetic.cb](exercises/ex01_variables_and_arithmetic.cb) | [ex01_variables_and_arithmetic.cb](solutions/ex01_variables_and_arithmetic.cb) |
| 2 | Conditionals and `for` Loops (FizzBuzz) | [ex02_fizzbuzz.cb](exercises/ex02_fizzbuzz.cb) | [ex02_fizzbuzz.cb](solutions/ex02_fizzbuzz.cb) |
| 3 | Functions, Recursion, and `while` Loops | [ex03_functions_and_loops.cb](exercises/ex03_functions_and_loops.cb) | [ex03_functions_and_loops.cb](solutions/ex03_functions_and_loops.cb) |
| 4 | Strings and Lists | [ex04_strings_and_lists.cb](exercises/ex04_strings_and_lists.cb) | [ex04_strings_and_lists.cb](solutions/ex04_strings_and_lists.cb) |
| 5 | Dictionaries (`dict[K, V]`) | [ex05_dictionaries.cb](exercises/ex05_dictionaries.cb) | [ex05_dictionaries.cb](solutions/ex05_dictionaries.cb) |
| 6 | Structs, `byref`, and UFCS | [ex06_structs_and_ufcs.cb](exercises/ex06_structs_and_ufcs.cb) | [ex06_structs_and_ufcs.cb](solutions/ex06_structs_and_ufcs.cb) |
| 7 | Enums and `select case` | [ex07_enums_and_select.cb](exercises/ex07_enums_and_select.cb) | [ex07_enums_and_select.cb](solutions/ex07_enums_and_select.cb) |
| 8 | Explicit Error Handling (`can error`, `try / catch`) | [ex08_error_handling.cb](exercises/ex08_error_handling.cb) | [ex08_error_handling.cb](solutions/ex08_error_handling.cb) |
| 9 | Resource Cleanup with `defer` | [ex09_defer_cleanup.cb](exercises/ex09_defer_cleanup.cb) | [ex09_defer_cleanup.cb](solutions/ex09_defer_cleanup.cb) |
| 10 | Race-Free Concurrency (`start`, `job[T]`, `.wait()`) | [ex10_concurrency_jobs.cb](exercises/ex10_concurrency_jobs.cb) | [ex10_concurrency_jobs.cb](solutions/ex10_concurrency_jobs.cb) |
| 11 | Cooperative Cancellation & Timeouts (`cancelToken`, `waitTimeout`, `isDone`) | [ex11_cancellation_and_timeouts.cb](exercises/ex11_cancellation_and_timeouts.cb) | [ex11_cancellation_and_timeouts.cb](solutions/ex11_cancellation_and_timeouts.cb) |

---

## Exercise 1: Variables, Constants, and Arithmetic

**Concepts:** `const`, `dim`, `float`, arithmetic operators, `.toStr()`, `print()`

**Files:** [Starter](exercises/ex01_variables_and_arithmetic.cb) | [Solution](solutions/ex01_variables_and_arithmetic.cb)

### Task
Write a program that defines a constant `BOILING_C = 100.0`, converts `0.0`, `25.0`, and `BOILING_C` from Celsius to Fahrenheit using the formula `F = C * 9.0 / 5.0 + 32.0`, and prints each conversion.

> **Tip:** CleanBasic is strictly typed—use float literals (`9.0`, `5.0`, `32.0`) when computing with `float` values.

### Expected Output
```text
0 C = 32 F
25 C = 77 F
100 C = 212 F
```

<details>
<summary>Show Solution</summary>

```cleanbasic
const BOILING_C = 100.0

fn toFahrenheit(c as float) as float
    return c * 9.0 / 5.0 + 32.0
end fn

fn main()
    dim freezing = 0.0
    dim room = 25.0

    print(freezing.toStr() + " C = " + toFahrenheit(freezing).toStr() + " F")
    print(room.toStr() + " C = " + toFahrenheit(room).toStr() + " F")
    print(BOILING_C.toStr() + " C = " + toFahrenheit(BOILING_C).toStr() + " F")
end fn
```

</details>

---

## Exercise 2: Conditionals and `for` Loops (FizzBuzz)

**Concepts:** `for i = start to end`, `if / else if / else / end if`, `%`, `and`

**Files:** [Starter](exercises/ex02_fizzbuzz.cb) | [Solution](solutions/ex02_fizzbuzz.cb)

### Task
Loop from `1` to `15` (inclusive). Print:
- `"FizzBuzz"` if the number is divisible by both 3 and 5,
- `"Fizz"` if divisible by 3,
- `"Buzz"` if divisible by 5,
- otherwise, the number itself.

### Expected Output
```text
1
2
Fizz
4
Buzz
Fizz
7
8
Fizz
Buzz
11
Fizz
13
14
FizzBuzz
```

<details>
<summary>Show Solution</summary>

```cleanbasic
fn main()
    for i = 1 to 15
        if i % 3 == 0 and i % 5 == 0
            print("FizzBuzz")
        else if i % 3 == 0
            print("Fizz")
        else if i % 5 == 0
            print("Buzz")
        else
            print(i.toStr())
        end if
    end for
end fn
```

</details>

---

## Exercise 3: Functions, Recursion, and `while` Loops

**Concepts:** `fn ... end fn`, recursion, `while ... end while`, integer division `/` and modulo `%`

**Files:** [Starter](exercises/ex03_functions_and_loops.cb) | [Solution](solutions/ex03_functions_and_loops.cb)

### Task
Write two functions:
1. `factorial(n as int) as int` — computes `n!` recursively (returning `1` for `n <= 1`).
2. `digitSum(n as int) as int` — uses a `while` loop to sum the base-10 digits of a non-negative integer.

Print `factorial(6)` and `digitSum(4829)`.

### Expected Output
```text
6! = 720
digitSum(4829) = 23
```

<details>
<summary>Show Solution</summary>

```cleanbasic
fn factorial(n as int) as int
    if n <= 1
        return 1
    end if
    return n * factorial(n - 1)
end fn

fn digitSum(n as int) as int
    dim remaining = n
    dim total = 0
    while remaining > 0
        total = total + (remaining % 10)
        remaining = remaining / 10
    end while
    return total
end fn

fn main()
    print("6! = " + factorial(6).toStr())
    print("digitSum(4829) = " + digitSum(4829).toStr())
end fn
```

</details>

---

## Exercise 4: Strings and Lists

**Concepts:** `list[str]()`, `str.split()`, `str.trim()`, `str.toLower()`, `list.push()`, `list.sort()`, `list.join()`

**Files:** [Starter](exercises/ex04_strings_and_lists.cb) | [Solution](solutions/ex04_strings_and_lists.cb)

### Task
Given the messy CSV string `"  Banana , apple,  Cherry , , date "`, split it on `","`, trim whitespace from each entry, lowercase it, skip any empty entries, sort the cleaned items alphabetically, and print them joined with `" | "` along with the item count.

### Expected Output
```text
items (4): apple | banana | cherry | date
```

<details>
<summary>Show Solution</summary>

```cleanbasic
fn main()
    dim raw = "  Banana , apple,  Cherry , , date "
    dim parts = raw.split(",")
    dim cleaned = list[str]()

    for item in parts
        dim entry = item.trim().toLower()
        if entry != ""
            cleaned.push(entry)
        end if
    end for

    cleaned.sort()
    print("items (" + cleaned.len().toStr() + "): " + cleaned.join(" | "))
end fn
```

</details>

---

## Exercise 5: Dictionaries (`dict[K, V]`)

**Concepts:** `dict[str, int]()`, `.get(key, default)`, `.contains(key)`, index assignment `d[k] = v`

**Files:** [Starter](exercises/ex05_dictionaries.cb) | [Solution](solutions/ex05_dictionaries.cb)

### Task
Count how many times each word appears in `"go basic go clean basic go"`. Use a `dict[str, int]` and print the counts for `"go"`, `"basic"`, `"clean"`, and a missing word `"rust"`.

### Expected Output
```text
go: 3
basic: 2
clean: 1
rust: 0
```

<details>
<summary>Show Solution</summary>

```cleanbasic
fn main()
    dim text = "go basic go clean basic go"
    dim words = text.split(" ")
    dim counts = dict[str, int]()

    for w in words
        dim prev = counts.get(w, 0)
        counts[w] = prev + 1
    end for

    dim queries = ["go", "basic", "clean", "rust"]
    for q in queries
        print(q + ": " + counts.get(q, 0).toStr())
    end for
end fn
```

</details>

---

## Exercise 6: Structs, `byref`, and UFCS

**Concepts:** `type ... end type`, positional & zero-value constructors, `byref` parameters, Uniform Function Call Syntax (UFCS)

**Files:** [Starter](exercises/ex06_structs_and_ufcs.cb) | [Solution](solutions/ex06_structs_and_ufcs.cb)

### Task
Define a `type Account` with fields `owner as str` and `balance as int`.
Write three functions:
1. `deposit(byref a as Account, amount as int)` — adds `amount` to `a.balance` if `amount > 0`.
2. `withdraw(byref a as Account, amount as int) as bool` — subtracts `amount` and returns `true` if `amount > 0 and amount <= a.balance`; otherwise returns `false`.
3. `summary(a as Account) as str` — returns `"<owner>: $<balance>"`.

Create an account for `"Ada"` with `$100`, deposit `$50`, attempt to withdraw `$200` (fails) and `$40` (succeeds), and call all functions using UFCS (`acct.deposit(...)`).

### Expected Output
```text
withdraw 200 ok: false
withdraw 40 ok: true
Ada: $110
```

<details>
<summary>Show Solution</summary>

```cleanbasic
type Account
    owner as str
    balance as int
end type

fn deposit(byref a as Account, amount as int)
    if amount > 0
        a.balance = a.balance + amount
    end if
end fn

fn withdraw(byref a as Account, amount as int) as bool
    if amount > 0 and amount <= a.balance
        a.balance = a.balance - amount
        return true
    end if
    return false
end fn

fn summary(a as Account) as str
    return a.owner + ": $" + a.balance.toStr()
end fn

fn main()
    dim acct = Account("Ada", 100)
    acct.deposit(50)

    dim ok1 = acct.withdraw(200)
    dim ok2 = acct.withdraw(40)

    print("withdraw 200 ok: " + ok1.toStr())
    print("withdraw 40 ok: " + ok2.toStr())
    print(acct.summary())
end fn
```

</details>

---

## Exercise 7: Enums and `select case`

**Concepts:** `enum ... end enum`, `select case ... case ... case else ... end select`, combining enums with structs

**Files:** [Starter](exercises/ex07_enums_and_select.cb) | [Solution](solutions/ex07_enums_and_select.cb)

### Task
Define an `enum Tier` with `Standard`, `Express`, and `Overnight`, and a `type Shipment` with `tier as Tier` and `weightKg as int`.
Write `shippingCost(s as Shipment) as int` using `select case`:
- `Tier.Standard`: `$5 + weightKg * 2`
- `Tier.Express`: `$12 + weightKg * 4`
- `Tier.Overnight`: `$25 + weightKg * 6`
- `case else`: `0`

Print the shipping cost for a 3 kg package at each tier.

### Expected Output
```text
Standard: $11
Express: $24
Overnight: $43
```

<details>
<summary>Show Solution</summary>

```cleanbasic
enum Tier
    Standard
    Express
    Overnight
end enum

type Shipment
    tier as Tier
    weightKg as int
end type

fn shippingCost(s as Shipment) as int
    select case s.tier
        case Tier.Standard
            return 5 + s.weightKg * 2
        case Tier.Express
            return 12 + s.weightKg * 4
        case Tier.Overnight
            return 25 + s.weightKg * 6
        case else
            return 0
    end select
end fn

fn main()
    dim s1 = Shipment(Tier.Standard, 3)
    dim s2 = Shipment(Tier.Express, 3)
    dim s3 = Shipment(Tier.Overnight, 3)

    print("Standard: $" + s1.shippingCost().toStr())
    print("Express: $" + s2.shippingCost().toStr())
    print("Overnight: $" + s3.shippingCost().toStr())
end fn
```

</details>

---

## Exercise 8: Explicit Error Handling (`can error`, `try / catch`)

**Concepts:** `can error`, `error(msg)`, `str.toInt()`, `try ... catch e ... end try`, `e.msg`

**Files:** [Starter](exercises/ex08_error_handling.cb) | [Solution](solutions/ex08_error_handling.cb)

### Task
Write a function `parsePositiveInt(raw as str) as int can error` that trims `raw`, parses it with `.toInt()`, and raises `error("value must be positive")` if the number is `<= 0`.
Then write `sumInputs(inputs as list[str]) as int can error` that calls `parsePositiveInt` on each element and returns the total.
In `main()`, call `sumInputs` twice inside `try / catch`: once with `["10", " 25 ", "5"]` and once with `["10", "-3", "5"]`.

> **Rule to remember:** Calls to `can error` functions/methods (like `.toInt()`) must be standalone statements or the direct right-hand side of a `dim` / assignment—never nested inside another expression.

### Expected Output
```text
valid sum: 40
error caught: value must be positive
```

<details>
<summary>Show Solution</summary>

```cleanbasic
fn parsePositiveInt(raw as str) as int can error
    dim trimmed = raw.trim()
    dim val = trimmed.toInt()
    if val <= 0
        error("value must be positive")
    end if
    return val
end fn

fn sumInputs(inputs as list[str]) as int can error
    dim total = 0
    for item in inputs
        dim parsed = parsePositiveInt(item)
        total = total + parsed
    end for
    return total
end fn

fn main()
    try
        dim good = sumInputs(["10", " 25 ", "5"])
        print("valid sum: " + good.toStr())
    catch e
        print("unexpected: " + e.msg)
    end try

    try
        dim bad = sumInputs(["10", "-3", "5"])
        print("unreachable: " + bad.toStr())
    catch e
        print("error caught: " + e.msg)
    end try
end fn
```

</details>

---

## Exercise 9: Resource Cleanup with `defer`

**Concepts:** `defer ... end defer`, LIFO cleanup on both normal return and error exit

**Files:** [Starter](exercises/ex09_defer_cleanup.cb) | [Solution](solutions/ex09_defer_cleanup.cb)

### Task
Write a function `runStep(name as str, shouldFail as bool) can error` that:
1. Prints `"open: " + name`
2. Registers a `defer` block that prints `"close: " + name`
3. If `shouldFail` is `true`, raises `error(name + " failed")`
4. Otherwise prints `"work: " + name`

In `main()`, call `runStep("job-A", false)` and `runStep("job-B", true)` inside `try / catch` blocks and observe that `defer` runs in both cases.

### Expected Output
```text
open: job-A
work: job-A
close: job-A
open: job-B
close: job-B
caught: job-B failed
```

<details>
<summary>Show Solution</summary>

```cleanbasic
fn runStep(name as str, shouldFail as bool) can error
    print("open: " + name)
    defer
        print("close: " + name)
    end defer

    if shouldFail
        error(name + " failed")
    end if

    print("work: " + name)
end fn

fn main()
    try
        runStep("job-A", false)
    catch e
        print("caught: " + e.msg)
    end try

    try
        runStep("job-B", true)
    catch e
        print("caught: " + e.msg)
    end try
end fn
```

</details>

---

## Exercise 10: Race-Free Concurrency (`start`, `job[T]`, `.wait()`)

**Concepts:** `start`, `job[T]`, `list[job[T]]()`, `.wait()`, value-isolated fan-out / fan-in

**Files:** [Starter](exercises/ex10_concurrency_jobs.cb) | [Solution](solutions/ex10_concurrency_jobs.cb)

### Task
Write `isPrime(n as int) as bool` and `countPrimes(low as int, high as int) as int`.
In `main()`, split the range `1..400` into 4 concurrent chunks (`1..100`, `101..200`, `201..300`, `301..400`) using `start countPrimes(...)`, collect the handles in a `list[job[int]]`, wait for all of them with `.wait()`, and print the total number of primes up to 400.

> **Why it's race-free:** `start` eagerly copies value arguments and forbids `byref` parameters or mutable global state access in spawned functions, so concurrent jobs cannot share mutable memory.

### Expected Output
```text
primes in 1..400: 78
```

<details>
<summary>Show Solution</summary>

```cleanbasic
fn isPrime(n as int) as bool
    if n < 2
        return false
    end if
    dim d = 2
    while d * d <= n
        if n % d == 0
            return false
        end if
        d = d + 1
    end while
    return true
end fn

fn countPrimes(low as int, high as int) as int
    dim count = 0
    for n = low to high
        if isPrime(n)
            count = count + 1
        end if
    end for
    return count
end fn

fn main()
    dim jobs = list[job[int]]()
    jobs.push(start countPrimes(1, 100))
    jobs.push(start countPrimes(101, 200))
    jobs.push(start countPrimes(201, 300))
    jobs.push(start countPrimes(301, 400))

    dim total = 0
    for j in jobs
        total = total + j.wait()
    end for

    print("primes in 1..400: " + total.toStr())
end fn
```

</details>

---

## Exercise 11: Cooperative Cancellation & Timeouts (`cancelToken`, `waitTimeout`, `isDone`)

**Concepts:** `cancelToken()`, `tok.isCanceled()`, `tok.cancel()`, `job.isDone()`, `job.waitTimeout(ms)`, `try / catch`

**Files:** [Starter](exercises/ex11_cancellation_and_timeouts.cb) | [Solution](solutions/ex11_cancellation_and_timeouts.cb)

### Task
Define `WorkerTask` with `id as int` and `tok as cancelToken`, plus `sumChunk(low as int, high as int) as int` and `waitForStop(task as WorkerTask) as str`.
In `main()`:
1. Create a token with `cancelToken()`.
2. Start `sumChunk(1, 10)` and wait for it with `waitTimeout(5000)` inside `try / catch`, printing `fast sum: 55`.
3. Start two `waitForStop` workers sharing `WorkerTask(1, tok)` and `WorkerTask(2, tok)`.
4. Print `w1.isDone()` before cancellation.
5. Wait on `w1` with `waitTimeout(15)` inside `try / catch`; in `catch`, record the timeout and call `tok.cancel()` to fan out cancellation to both workers.
6. Print `timed out: true`, both `.wait()` results, and `w1.isDone()` after `.wait()`.

### Expected Output
```text
fast sum: 55
w1 done before cancel: false
timed out: true
worker 1 canceled
worker 2 canceled
w1 done after wait: true
```

<details>
<summary>Show Solution</summary>

```cleanbasic
type WorkerTask
    id as int
    tok as cancelToken
end type

fn sumChunk(low as int, high as int) as int
    dim total = 0
    for n = low to high
        total = total + n
    end for
    return total
end fn

fn waitForStop(task as WorkerTask) as str
    dim ticks = 0
    while not task.tok.isCanceled()
        ticks = ticks + 1
    end while
    return "worker " + task.id.toStr() + " canceled"
end fn

fn main()
    dim tok = cancelToken()

    dim fastJob = start sumChunk(1, 10)
    try
        dim fastSum = fastJob.waitTimeout(5000)
        print("fast sum: " + fastSum.toStr())
    catch e
        print("fast job failed: " + e.msg)
    end try

    dim w1 = start waitForStop(WorkerTask(1, tok))
    dim w2 = start waitForStop(WorkerTask(2, tok))

    print("w1 done before cancel: " + w1.isDone().toStr())

    dim timedOut = false
    try
        dim unused = w1.waitTimeout(15)
    catch e
        timedOut = true
        tok.cancel()
    end try

    print("timed out: " + timedOut.toStr())
    print(w1.wait())
    print(w2.wait())
    print("w1 done after wait: " + w1.isDone().toStr())
end fn
```

</details>

---

### Where to Go Next
- Explore multi-file modules in [`tests/cases/07_multi_file_modules/`](../tests/cases/07_multi_file_modules/)
- See standard library usage (`std/cli`, `std/fs`, `std/os`, `std/regex`) in [`examples/cbgrep/main.cb`](../examples/cbgrep/main.cb)
- Read the full language rules in [`docs/cleanbasic_language_specification_go_backend_edition.md`](../docs/cleanbasic_language_specification_go_backend_edition.md)
