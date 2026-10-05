# goxd Build Plan

Oct 6, 2026 · @PHAN KIEU PHU

## Goal and rules

goxd is Go plus five small additions for API code. Each addition hides one piece of Go syntax, and each one can be skipped and written as plain Go in the same file.

| Rule | What it means in practice |
| --- | --- |
| Superset | Every valid Go file is valid goxd. A file with no additions translates to itself. |
| Hide, never forbid | Each sugar has a plain Go form. Both forms can be mixed in one function. |
| No decorator, no rewrite | The translator only changes types marked with a decorator. All other code passes through untouched. |
| Rewrite only what Go rejects | Inside a managed method, sugar is code that plain Go would not compile. Code that is already correct Go keeps its meaning. |
| Readable output | Generated Go is formatted and looks hand-written, so people can learn Go from it. |
| No runtime reflection | Routes, wiring and binding are generated code. Speed equals hand-written Go. |
| Eject at any time | `goxd eject` leaves a normal Go project that no longer needs goxd. |

**Words used in this plan.** A *managed type* is a struct marked `@Controller` or `@Injectable`. A *managed method* is any method of a managed type that is not marked `@Plain`.

**Not in version 1:** gRPC and message consumers, an ORM, request-scoped providers, parameter decorators such as `@Body`, and any change to Go's type system.

## Language: five additions

The translator acts only inside managed methods. For each addition, one local rule decides whether it rewrites the code or leaves your Go alone.

| Addition | Sugar you write | Plain Go that still works | The translator acts when |
| --- | --- | --- | --- |
| Decorators | `@Get("/:id")` above a method | A hand-written `Routes(r *goxd.Router)` method | The type has decorators. Decorators plus a manual `Routes()` on one type is an error. |
| Implicit context | Leave `ctx` out of signatures and calls | Write `ctx context.Context` first and pass it yourself | The signature has no `ctx`, or a callee wants a context first and the call does not pass one. |
| Errors as throw | `throw NotFound("...")` and `user := s.repo.Find(id)` | `error` in the result, `user, err := ...`, `return nil, err` | The result list has no `error`, or a call's last `error` value is not received. |
| `parallel` | `a, b := parallel(f(x), g(y))` | `errgroup`, `sync.WaitGroup`, `go`, channels | The code calls `parallel(...)`. |
| `@Async` | `@Async` on a method, callers call it normally | `go func() { ... }()` | The method has `@Async`. |

Sugar and plain Go in one type:

```go
@Injectable
type UserService struct {
    repo *UserRepo
    db   *sql.DB
}

// Sugar: no ctx, no error.
func (s *UserService) Find(id string) *User {
    user := s.repo.FindByID(id)
    if user == nil {
        throw NotFound("user not found")
    }
    return user
}

// Plain Go in the same type. goxd changes nothing here.
func (s *UserService) Count(ctx context.Context) (int, error) {
    var n int
    err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&n)
    if err != nil {
        return 0, fmt.Errorf("count users: %w", err)
    }
    return n, nil
}
```

### Context rules

1. Every managed method ends with `ctx context.Context` as its first parameter. If you wrote it, nothing changes.
2. Inside a managed method the name `ctx` always exists, so you can pass it to any Go library.
3. Fill-in rule: when a callee's first parameter is `context.Context` and the call does not pass one, the translator inserts `ctx`. This covers managed methods and normal Go libraries.
4. `@Timeout("2s")` wraps the method's context with a timeout and a deferred cancel.

### Error rules

1. Every managed method ends with `error` as its last result. If you wrote it, nothing changes.
2. `throw X` becomes `return <zero values>, X`.
3. Propagation rule: when a call returns `error` last and your code does not receive it, the translator adds `if err != nil { return ..., err }`.
4. A bare call statement such as `s.repo.Save(u)` propagates in a method with implicit `error`. In a method where you wrote `error` yourself, it keeps Go's meaning. Write `_ = s.repo.Save(u)` to drop an error on purpose.
5. Propagation works in assignments, call statements, `return` arguments and `parallel` arguments. A call nested deeper in an expression must move to its own line in version 1.

### Concurrency rules

- `parallel`: each argument is one call, and results come back in order. Each call runs in its own goroutine with a shared context. The first error cancels the others and is returned.
- Lists: `goxd.Map(items, fn)` runs `fn` for each item with a concurrency limit. It is a normal generic library function.
- `@Async`: the call returns at once. The app starts the goroutine and gives it panic recovery, error logging, and a context that keeps request values but is not cancelled when the request ends. Shutdown waits for it. An `@Async` method has no results.
- Channels, `select`, mutexes and raw `go` statements are plain Go and are never rewritten.

### Version 1 decorators

`@Controller(path)`, `@Get`, `@Post`, `@Put`, `@Patch`, `@Delete`, `@HttpCode(n)`, `@Injectable`, `@Module`, `@UseGuards(...)`, `@UseInterceptors(...)`, `@Use(fn)`, `@Timeout(d)`, `@Async`, `@Plain`.

- Binding: a parameter whose name matches `:name` in the path is a path parameter. One struct parameter is filled from the JSON body and from `query:"..."` tags.
- `@Plain` turns every rewrite off for one method. Use it for interface methods that need an exact signature.
- An unknown decorator name stops the build.
- Function literals are plain Go in version 1: `ctx` is visible inside them, but `throw` and propagation are not.

## Translator design

The translator is eight passes, and Go's own packages do the real parsing and type checking. You write a scanner and a rewriter, not a compiler.

1. **Scan.** Read tokens with `go/scanner`. Turn each `@X(...)` line into a `//goxd:X(...)` comment on the same line. Turn `throw X` into the call form `throw(X)`. Line numbers do not change.
2. **Parse.** Run `go/parser` with comments on. Attach each decorator to the type or function below it. An unknown decorator is an error with file and line.
3. **Model.** Build the app model: controllers, routes, injectables and their constructors, modules, managed methods. Check for duplicate routes, missing providers and dependency cycles.
4. **Rewrite signatures.** Add `ctx` first and `error` last where they are missing. Record for each method which of the two were implicit.
5. **Type check.** Load the package with `golang.org/x/tools/go/packages`, passing the rewritten files as an overlay. Run `go/types` with an error handler so checking continues after errors.
6. **Rewrite call sites.** With type information, insert `ctx`, expand error propagation, expand `throw`, turn `return v` into `return v, nil`, and expand `parallel`, `@Timeout` and `@Async`.
7. **Generate.** Write one `<name>_goxd.go` per source file with `//line` comments that point back to the `.goxd` file. Write one `goxd_wire_gen.go` per package with routes, binders and constructor wiring.
8. **Verify.** Type check the output again, which must be clean. Format with `go/format`. Write a file only when its content changed, so Go's build cache keeps its hits.

| Decision | Choice | Reason |
| --- | --- | --- |
| Where generated code lives | Next to the source, with a `Code generated by goxd. DO NOT EDIT.` header | The Go compiler ignores `.goxd` files and picks up the `.go` files with no extra setup. |
| Mixed packages | `.go` and `.goxd` files can share a package | A plain `.go` file sees the generated signatures, so it passes `ctx` and handles `error` itself. |
| User mistakes | Report type errors from pass 5 with `.goxd` positions, after removing the errors expected at sugar sites | Fast feedback without waiting for `go build`. |
| Dependency wiring | Generated constructor calls in dependency order, no runtime container | Same idea as Google Wire: errors at build time and nothing to resolve at startup. |
| Caching | Hash of each source file plus the translator version | Unchanged files skip passes 1 to 4. |

**Hardest part: pass 5.** The code still contains calls with a missing `ctx` and assignments with a missing `err`, so the type checker reports errors there. The rewrite needs the callee's signature at exactly those places. If one round gives too little information, the fallback is to fix the sites that are resolved and check again until no sugar is left. Phase 0 tests this before anything else is built.

## Runtime library

The runtime is a normal Go package named `goxd` that works without the translator. Generated code only calls this package, so a plain Go developer can use it directly.

| Part | What it does | Main types |
| --- | --- | --- |
| App | Builds modules, starts the server, shuts down cleanly and waits for async tasks | `goxd.App`, `OnInit`, `OnShutdown` hooks |
| Router | Method and path matching on top of `net/http.ServeMux` (Go 1.22 patterns) | `goxd.Router`, `goxd.Get(r, path, handler, opts...)` |
| Handlers | One typed shape for every route | `func(ctx context.Context, req T) (R, error)` |
| Guards | Allow or deny a request before the handler | `Guard.CanActivate(ctx, *http.Request) error` |
| Interceptors | Wrap a handler for logging, metrics, caching | `type Interceptor func(next Handler) Handler` |
| Errors | HTTP errors and one place that turns any error into a JSON response | `goxd.HTTPError`, `NotFound`, `BadRequest`, `Unauthorized`, `Forbidden`, `Conflict`, `ExceptionFilter` |
| Modules | Group providers and controllers, like `@Module` | `goxd.Module{Imports, Providers, Controllers}` |
| Concurrency | The code behind `parallel` and `@Async` | `goxd.Group`, `goxd.Map`, `app.Go(ctx, name, fn)` |
| Safe state | Shared data that is safe across requests | `goxd.Cache[K, V]`, `goxd.Counter` |
| Request access | Reach the raw request when the sugar is not enough | `goxd.Request(ctx)`, `goxd.Logger(ctx)` |
| Testing | Call routes in tests without a network port | `goxdtest.New(module).Get("/users/1")` |

Design points:

- The guard and interceptor chain for each route is built once, when the route is registered.
- Binding and validation are generated functions. Version 1 supports `required`, `min`, `max`, `len`, `email` and `oneof`.
- An error that is not a `goxd.HTTPError` becomes a 500 response. The detail goes to the log, not to the client.
- Providers are singletons in version 1.
- The package depends only on the Go standard library. `goxd.Group` is a small copy of the errgroup idea, so there is no extra module to install.
- Minimum Go version is 1.22, for `ServeMux` path patterns and `context.WithoutCancel`.

## CLI and editor support

One `goxd` command wraps the translator and the Go toolchain, so nobody has to remember two steps.

| Command | What it does |
| --- | --- |
| `goxd new <name>` | Creates a project with one module, one controller, one service and a test |
| `goxd gen` | Translates `.goxd` files to Go and stops |
| `goxd build` | Runs `gen`, then `go build` |
| `goxd run` | Runs the app and rebuilds when a file changes |
| `goxd test` | Runs `gen`, then `go test -race` |
| `goxd fmt` | Formats `.goxd` files with gofmt rules |
| `goxd vet` | Reports goxd-specific problems (list below) |
| `goxd routes` | Prints every route with its guards and handler |
| `goxd explain <file>` | Shows the source and the generated Go side by side |
| `goxd eject` | Removes `.goxd` files and keeps the generated Go as the source |

`goxd vet` rules in version 1:

- A managed method writes to a field of its own service outside the constructor (data race risk).
- A raw `go` statement inside a managed method (no panic recovery, not tracked at shutdown).
- An `@Async` method that uses the request body or response writer.
- `_ =` used to drop an error, reported as a note.

Project layout:

```
myapp/
  go.mod
  main.goxd
  users/
    users_module.goxd
    user_controller.goxd
    user_service.goxd
    user_repo.go                # plain Go, hand-written
    user_controller_goxd.go     # generated
    user_service_goxd.go        # generated
    goxd_wire_gen.go            # generated
```

Editor support grows in three levels, one per phase:

| Level | What the developer gets | How it is built |
| --- | --- | --- |
| 1. Highlighting | Go colours plus decorators and `throw` | A small TextMate grammar on top of Go's, and `*.goxd linguist-language=Go` in `.gitattributes` |
| 2. Errors and format on save | Red underlines in the `.goxd` file, auto format | A VS Code extension that runs `goxd gen --json` and `goxd fmt` |
| 3. Autocomplete and jump to code | The normal Go editing experience | A language server that sends a shadow Go file to gopls and maps positions back |

Level 3 is the largest single piece of work in the project. It comes last, and the language is usable without it.

## Phases and gates

Six phases take the project to version 0.1 in about 24 weeks of part-time work, and every phase ends with something usable.

&#91;embedded content: roadmap · 6 phases, 6 gates\]

Phases 1 and 2 already give a working framework with decorators. Phase 3 is the one that hides `ctx` and errors, and its design depends on the result of the Phase 0 spike.

The week numbers assume about 10 hours a week from one person. I do not know your real hours, so treat them as relative sizes.

You can stop after any gate and still have a useful tool:

- After Phase 1: a small Go framework with guards and interceptors.
- After Phase 2: NestJS-style decorators, with `ctx` and errors written by hand.
- After Phase 3: the language you described.

First two weeks (Phase 0):

- [ ] Write 20 pairs of `.goxd` source and the Go you expect from it, covering every rule in the language section.
- [ ] Spike: run `go/types` on five of the sources after the signature rewrite, and print the callee signature at each sugar site.
- [ ] Answer the open questions at the end of this plan.
- [ ] Create the repository with CI that runs `go test -race` and the golden tests.

## Testing and benchmarks

The translator is tested by comparing its output to Go you would write by hand, and the performance promise is tested against a hand-written app.

| Test | What it proves | How |
| --- | --- | --- |
| Golden files | Each rule produces the expected Go | `testdata/*.goxd` next to `*.go.golden`, with an update flag |
| Passthrough | goxd is a true superset | Translate plain Go packages renamed to `.goxd`. Output must equal the input. |
| Compile | Generated code is valid | Every golden output passes `go build` and `go vet` |
| Same behaviour | Sugar does not change results | One HTTP test suite runs against a hand-written app and its goxd twin |
| Error messages | Mistakes are explained well | Bad inputs with the expected message, file and line |
| Race | Runtime is safe under load | All runtime tests with `-race`, plus a stress test with parallel requests |
| Fuzz | The scanner never crashes or damages code | Go fuzzing on pass 1. Lines without sugar must come out unchanged. |

The twin app is the centre of the test plan. Write a small CRUD API by hand in Phase 1, then write the same API in goxd. Their generated and hand-written code should look almost the same.

Benchmarks, measured from Phase 1 onward. The targets are proposals to confirm after the first baseline. Nothing here is measured yet.

| Metric | How to measure | Proposed target |
| --- | --- | --- |
| Time per request through guards, binding and handler | `go test -bench . -benchmem` on the twin apps | Within 5% of the hand-written app |
| Allocations per request | Same benchmark, `allocs/op` | Equal to the hand-written app |
| Startup with 200 routes and 50 providers | Time from process start to first accepted request | Under 10 ms |
| Translate 100 files | `goxd gen` cold, then with one file changed | Under 1 s cold, under 200 ms warm |
| Binary size | Compare the twin apps | Within 5% |

Run the benchmarks in CI on every change to the runtime or the generator, and fail the build when a number gets clearly worse.

## Risks and open questions

The biggest risk is technical and can be tested in the first two weeks: whether `go/types` gives enough information about code that still contains sugar.

| Risk | Why it matters | What to do |
| --- | --- | --- |
| Type checking code with sugar gives too little information | Implicit `ctx` and error propagation depend on it | Phase 0 spike. Fallback: fix and re-check in rounds. Last resort: a visible marker such as `call()?` |
| Hidden control flow confuses people | A line can return early, and an `@Async` call looks like a normal call | `goxd explain`, readable output, and `goxd vet` notes |
| Data races in singleton services | Go runs requests in parallel, unlike Node | The vet rule on field writes, `-race` by default in `goxd test`, safe state types |
| Editor support takes longer than the language | Without autocomplete, people give up | Ship levels 1 and 2 early. Keep comment-style decorators in `.go` files as a supported option |
| The project grows into a full language | A solo project cannot maintain one | Keep to five additions. A sixth needs a written reason and a plain Go form |
| A new Go release breaks the scanner | The tool stops working after an upgrade | `go/parser` does the real parsing. Run CI against the next Go release |
| People do not trust a new tool | Lock-in fear | `goxd eject` and the passthrough test, both shown in the README |

Decide before Phase 2 starts:

- [ ] Are all methods of a managed type managed (the default in this plan), or only exported ones?
- [ ] Keep the two meanings of a bare call statement (error rule 4), or always propagate inside managed methods?
- [ ] `throw X` as a keyword, or only the call form `throw(X)`?
- [ ] Commit generated files, or ignore them and always run `goxd build`?
- [ ] Stay on `net/http.ServeMux`, or write a router?
- [ ] Confirm the name: GitHub organisation, pkg.go.dev, VS Code Marketplace, domain.
- [ ] Module path and licence.
