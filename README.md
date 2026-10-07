# gox

**Go plus five small additions for API code.** Decorators, implicit `ctx`, `throw`-style errors, `parallel` and `@Async`, translated ahead of time into plain, readable Go. No runtime reflection. You can eject at any time.

> **Status: pre-alpha (Phase 0).** Nothing here is usable yet. This README is the plan.
> The full design lives in [`goxd Build Plan.md`](./goxd%20Build%20Plan.md). It was written under the working name *goxd*, and the project now ships as **gox** under [github.com/goxnest](https://github.com/goxnest).

```go
@Controller("/users")
type UserController struct {
    svc *UserService
}

@Get("/:id")
func (c *UserController) GetUser(id string) *User {
    return c.svc.Find(id)
}

@Injectable
type UserService struct {
    repo *UserRepo
}

func (s *UserService) Find(id string) *User {
    user := s.repo.FindByID(id)   // ctx is passed in and the error is returned for you
    if user == nil {
        throw NotFound("user not found")
    }
    return user
}
```

`gox build` turns that into the Go you would write by hand: `ctx context.Context` first, `error` last, `if err != nil { return nil, err }`, plus generated routes and constructor wiring.

---

## Why

NestJS-style structure (controllers, injectables, modules, guards, interceptors) is productive, but in Go it usually costs one of two things: runtime reflection, or a lot of boilerplate. gox gives you the structure at build time, and the output is ordinary Go.

## Principles

| Rule | In practice |
| --- | --- |
| **Superset** | Every valid Go file is valid gox. A file with no additions translates to itself. |
| **Hide, never forbid** | Each addition has a plain Go form. You can mix both forms in one function. |
| **No decorator, no rewrite** | Only types marked `@Controller` or `@Injectable` are touched. |
| **Rewrite only what Go rejects** | Code that is already valid Go keeps its meaning. |
| **Readable output** | Generated Go is formatted and looks hand-written. |
| **No runtime reflection** | Routes, binding and wiring are generated. It runs as fast as hand-written Go. |
| **Eject at any time** | `gox eject` leaves a normal Go project with no gox dependency. |

## The five additions

| Addition | Sugar | Plain Go that still works |
| --- | --- | --- |
| Decorators | `@Get("/:id")` above a method | A hand-written `Routes(r *gox.Router)` method |
| Implicit context | Leave `ctx` out of signatures and calls | `ctx context.Context` written and passed by hand |
| Errors as throw | `throw NotFound("...")`, `user := s.repo.Find(id)` | `error` result, `user, err := ...`, `return nil, err` |
| `parallel` | `a, b := parallel(f(x), g(y))` | `errgroup`, `sync.WaitGroup`, channels |
| `@Async` | `@Async` on a method; callers call it normally | `go func() { ... }()` |

Version 1 decorators: `@Controller`, `@Get`, `@Post`, `@Put`, `@Patch`, `@Delete`, `@HttpCode`, `@Injectable`, `@Module`, `@UseGuards`, `@UseInterceptors`, `@Use`, `@Timeout`, `@Async`, `@Plain`.

**Not in version 1:** gRPC and message consumers, an ORM, request-scoped providers, parameter decorators such as `@Body`, and any change to Go's type system.

## How it works

The translator is a scanner and a rewriter. Go's own packages (`go/scanner`, `go/parser`, `go/types`, `golang.org/x/tools/go/packages`) do the real parsing and type checking.

```
.gox ──► 1 scan ──► 2 parse ──► 3 model ──► 4 rewrite signatures
                                                    │
 *_gox.go ◄── 8 verify ◄── 7 generate ◄── 6 rewrite call sites ◄── 5 type check (overlay)
```

1. **Scan.** `@X(...)` becomes a `//gox:X(...)` comment and `throw X` becomes `throw(X)`. Line numbers stay the same.
2. **Parse.** Decorators are attached to the type or function below them.
3. **Model.** Controllers, routes, injectables and modules. Checks for duplicate routes, missing providers and cycles.
4. **Rewrite signatures.** Adds `ctx` first and `error` last where they are missing.
5. **Type check.** Uses `go/types` on the rewritten files through an overlay. *This is the hardest part and the Phase 0 spike.*
6. **Rewrite call sites.** Inserts `ctx`, expands error propagation, `throw`, `parallel`, `@Timeout` and `@Async`.
7. **Generate.** One `<name>_gox.go` per source file, with `//line` directives that point back to the `.gox` file, plus a wiring file per package.
8. **Verify.** Re-checks and formats the output, and writes a file only when its content changed.

## Runtime

Package `gox` uses the standard library only and works without the translator. Generated code only calls this package.

`App` · `Router` (on `net/http.ServeMux`, Go 1.22 patterns) · typed handlers `func(ctx, req T) (R, error)` · `Guard` · `Interceptor` · `HTTPError` + `ExceptionFilter` · `Module` · `Group` / `Map` / `app.Go` · `Cache[K,V]` / `Counter` · `goxtest` for in-process route tests.

## CLI (planned)

| Command | What it does |
| --- | --- |
| `gox new <name>` | Scaffold a project |
| `gox gen` | Translate `.gox` to Go |
| `gox build` / `gox run` / `gox test` | `gen`, then the Go toolchain (`test` runs with `-race`) |
| `gox fmt` / `gox vet` | Format, and report gox-specific problems |
| `gox routes` | Print every route with its guards and handler |
| `gox explain <file>` | Show the source and the generated Go side by side |
| `gox eject` | Keep the generated Go and remove the `.gox` files |

## Roadmap

About 24 weeks of part-time work to v0.1. Every phase ends with something usable.

- [ ] **Phase 0: Spike (2 weeks).** 20 golden `.gox` → `.go` pairs. Prove that `go/types` gives enough information on code that still contains sugar. Set up CI with `go test -race` and the golden tests.
- [ ] **Phase 1: Runtime.** Package `gox` with router, guards, interceptors and errors, plus the hand-written twin CRUD app and its baseline benchmarks.
- [ ] **Phase 2: Decorators.** Routes, binding and DI wiring are generated. `ctx` and errors are still written by hand.
- [ ] **Phase 3: Implicit `ctx` and errors.** `throw`, `parallel` and `@Async`.
- [ ] **Later phases:** editor support (TextMate grammar, then a VS Code extension, then a gopls-backed language server), `gox vet`, `gox eject`, and the v0.1 release.

### Open questions

- [ ] Are all methods of a managed type managed, or only exported ones?
- [ ] Should a bare call statement always propagate errors inside managed methods?
- [ ] Should `throw X` be a keyword, or only the call form `throw(X)`?
- [ ] Should generated files be committed, or ignored and always regenerated?
- [ ] Stay on `net/http.ServeMux`, or write a router?

## Repository layout (target)

```
cmd/gox/            CLI
internal/           translator: scan, parse, model, check, rewrite, gen, verify, translate, vet, eject
runtime/            package gox (stdlib only) + goxtest
testdata/           golden/, passthrough/, errors/
examples/           twin-handwritten/, twin-gox/
spike/              Phase 0 throwaway code
editor/vscode/      grammar + extension
```

Right now the repository holds the build plan, a prototype scanner (`main.go`, which rewrites `@X` lines in `main.gox`) and model sketches in `gox/`.

## Try the prototype

```sh
go run . main.gox
```

## Testing

- **Golden files:** each rule produces the expected Go.
- **Passthrough:** plain Go renamed to `.gox` must come out unchanged.
- **Twin app:** one HTTP test suite runs against a hand-written app and its gox twin.
- **Fuzz:** the scanner never changes lines that contain no sugar.
- **Benchmarks:** target is within 5% of the hand-written app per request, with equal allocations.

## Contributing

The project is in early design. Issues and discussion about the open questions are welcome.

## License

[MIT](./LICENSE)
