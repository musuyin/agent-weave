# Analysis: `server/cmd/server/main.go` — Server Startup Pattern

## The question

> I only had to use `r` (Gin router) to bind URLs with handlers. What does this explicit `srv` do?

## Short answer

`gin.Engine.Run()` is a thin convenience wrapper that creates exactly this `http.Server` internally.
Writing it explicitly gives us control the shortcut hides — most importantly, the ability to call `srv.Shutdown()` for graceful shutdown.

---

## Full walkthrough

### 1. Gin router as `http.Handler`

```go
router, _, _ := InitializeApp(ctx, log)   // returns *gin.Engine

srv := &http.Server{
    Addr:    ":" + port,
    Handler: router,           // gin.Engine implements http.Handler
}
```

`*gin.Engine` implements the `http.Handler` interface (`ServeHTTP(w, r)`), so it can be plugged into any standard `net/http` server as the request dispatcher.

### 2. Non-blocking start via goroutine

```go
go func() {
    log.Info("server starting", "addr", srv.Addr)
    if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
        log.Error("server error", "error", err)
        os.Exit(1)
    }
}()
```

`ListenAndServe` blocks forever, so it runs in a goroutine.
This frees the main goroutine to proceed to the signal-wait block below.

`http.ErrServerClosed` is the expected non-error return when `Shutdown()` is called — it must be filtered out, otherwise a clean shutdown would log a spurious error.

### 3. Graceful shutdown

```go
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
<-quit                          // blocks until Ctrl-C or kill

shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
srv.Shutdown(shutCtx)           // drains in-flight requests, then closes
```

`srv.Shutdown()` is only available because we hold a reference to the `*http.Server`.
With `r.Run(":8080")` the server is created inside Gin and there is no way to reach `Shutdown()`.

The 30-second timeout prevents the process from hanging indefinitely if a request is stuck.

---

## Why not just use `r.Run()`?

| | `r.Run(":8080")` | explicit `http.Server` |
|---|---|---|
| Graceful shutdown | No — blocks forever | Yes — `srv.Shutdown()` |
| Server-level timeouts | No | Yes (`ReadTimeout`, `WriteTimeout`, …) |
| TLS | No | Yes (`ListenAndServeTLS`) |
| Simplicity | High | Slightly more verbose |

For a production server that must drain in-flight requests on deploy/restart, the explicit pattern is required.

---

## Summary

`srv` is not doing anything Gin-specific — it is the standard Go HTTP server that hosts Gin as its request handler. The explicit reference to `srv` exists solely so `main()` can call `srv.Shutdown()` after receiving a SIGINT/SIGTERM, giving in-flight requests up to 30 seconds to complete before the process exits.
