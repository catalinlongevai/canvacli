# OAuth 2.0 PKCE in Go for `canvacli`

Research notes for implementing the Canva Connect API auth flow as a Go CLI.
Target audience: the implementer of `canvacli login` and the auth middleware.

## TL;DR

- **Stay stdlib + `golang.org/x/oauth2` v0.13.0+.** PKCE is built in
  (`oauth2.GenerateVerifier`, `oauth2.S256ChallengeOption`,
  `oauth2.VerifierOption`). No third-party OAuth library needed.
- **Local callback** = `net.Listen("tcp", "127.0.0.1:0")` + an `http.Server`
  whose handler pushes `(code, state)` onto a channel and triggers
  `srv.Shutdown(ctx)`.
- **Storage** = JSON-serialised `*oauth2.Token` at
  `os.UserConfigDir()/canvacli/token.json` with `chmod 0600` on Unix.
  `os.UserConfigDir()` already honours `XDG_CONFIG_HOME` on Linux.
- **Auto-refresh** = wrap a persisted token with a custom `TokenSource`
  that re-saves the token after every refresh, then use
  `oauth2.NewClient(ctx, src)`. `oauth2`'s `Transport` already retries
  expired tokens; you only need an extra 401 retry layer for *server-side*
  revocation.
- **Browser** = `github.com/pkg/browser` (BSD-2). 30 lines of stdlib
  `exec.Command` works too if you'd rather not take the dep.

## 1. PKCE with `golang.org/x/oauth2`

PKCE landed in `golang.org/x/oauth2` v0.13.0 (Oct 2023). The relevant
exports:

```go
func GenerateVerifier() string                            // 32 random octets, base64url
func S256ChallengeFromVerifier(v string) string
func S256ChallengeOption(v string) AuthCodeOption         // pass to AuthCodeURL
func VerifierOption(v string) AuthCodeOption              // pass to Exchange
```

Reference: <https://pkg.go.dev/golang.org/x/oauth2#GenerateVerifier>.

Canva's authorization endpoint is
`https://www.canva.com/api/oauth/authorize` and the token endpoint is
`https://www.canva.com/api/oauth/token`. Canva requires
`code_challenge_method=s256` (lowercase per their docs, but the spec
treats it case-insensitively and the stdlib emits `S256` — confirm against
a live request before shipping). Source:
<https://www.canva.dev/docs/connect/authentication/>.

Token exchange uses HTTP Basic auth with `client_id:client_secret`. For a
public CLI you generally do **not** ship a client secret; verify with
Canva that they accept "public client" PKCE without a secret (per RFC 7636
this is the canonical use case). If they require a secret, embed it but
treat it as a non-secret — PKCE is what actually protects the flow.

```go
conf := &oauth2.Config{
    ClientID:     clientID,
    ClientSecret: clientSecret, // optional; see note above
    Endpoint: oauth2.Endpoint{
        AuthURL:   "https://www.canva.com/api/oauth/authorize",
        TokenURL:  "https://www.canva.com/api/oauth/token",
        AuthStyle: oauth2.AuthStyleInHeader, // Basic auth on token endpoint
    },
    Scopes:      []string{"asset:read", "asset:write", "design:meta:read"},
    RedirectURL: redirectURL, // built after we know the listener port
}

verifier := oauth2.GenerateVerifier()
state := randString(32) // 32 bytes hex/base64

authURL := conf.AuthCodeURL(state,
    oauth2.AccessTypeOffline,            // ask for a refresh token
    oauth2.S256ChallengeOption(verifier),
)

// ... user clicks through ...

tok, err := conf.Exchange(ctx, code, oauth2.VerifierOption(verifier))
```

## 2. Local callback HTTP server

Pattern (RFC 8252 §7.3):

1. `ln, _ := net.Listen("tcp", "127.0.0.1:0")` — kernel picks a port.
2. Read the assigned port from `ln.Addr().(*net.TCPAddr).Port` and build
   `redirectURL = fmt.Sprintf("http://127.0.0.1:%d/callback", port)`.
3. Start an `http.Server` whose handler validates `state`, captures
   `code`, writes a small HTML success page, then signals shutdown.
4. `select { case res := <-resultCh: ... case <-time.After(2*time.Min): ... }`.

Use `127.0.0.1` literally — never `localhost`. Some browsers and
resolvers send `localhost` over IPv6 first, and your `tcp4` listener will
miss it. Canva requires the registered redirect URI match exactly, so
register `http://127.0.0.1:0/callback` is not allowed; you'll need to
either (a) register a fixed port like `http://127.0.0.1:8765/callback`
and tolerate the rare collision, or (b) register a wildcard if the
provider supports it. Canva's docs don't currently confirm wildcard
support — assume fixed-port-with-fallback.

Skeleton:

```go
type result struct {
    code  string
    err   error
}

func runCallbackServer(ctx context.Context, expectedState string) (string, string, error) {
    ln, err := net.Listen("tcp4", "127.0.0.1:0")
    if err != nil {
        return "", "", err
    }
    port := ln.Addr().(*net.TCPAddr).Port
    redirectURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

    resCh := make(chan result, 1)
    mux := http.NewServeMux()
    mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
        q := r.URL.Query()
        if errMsg := q.Get("error"); errMsg != "" {
            http.Error(w, errMsg, http.StatusBadRequest)
            resCh <- result{err: fmt.Errorf("authz error: %s", errMsg)}
            return
        }
        if q.Get("state") != expectedState {
            http.Error(w, "state mismatch", http.StatusBadRequest)
            resCh <- result{err: errors.New("state mismatch")}
            return
        }
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        _, _ = w.Write([]byte(`<!doctype html><meta charset=utf-8>
            <title>canvacli</title>
            <h1>Authorisation complete</h1>
            <p>You can close this window and return to the terminal.</p>
            <script>window.close()</script>`))
        resCh <- result{code: q.Get("code")}
    })

    srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
    go srv.Serve(ln)

    defer func() {
        shCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
        defer cancel()
        _ = srv.Shutdown(shCtx)
    }()

    select {
    case res := <-resCh:
        return res.code, redirectURL, res.err
    case <-ctx.Done():
        return "", redirectURL, ctx.Err()
    case <-time.After(2 * time.Minute):
        return "", redirectURL, errors.New("timed out waiting for browser approval")
    }
}
```

Notes on the HTML response: a `<script>window.close()</script>` call only
works for windows the script itself opened, so most Canva users will see
the success page until they close the tab. That's fine; just don't rely
on auto-close.

## 3. Token storage

`os.UserConfigDir()` does the right thing on every target:

| OS      | Path                                          |
| ------- | --------------------------------------------- |
| Linux   | `$XDG_CONFIG_HOME` else `$HOME/.config`       |
| macOS   | `$HOME/Library/Application Support`           |
| Windows | `%AppData%`                                   |

Reference: <https://pkg.go.dev/os#UserConfigDir>. The user spec mentions
`~/.config/canvacli/token.json` — note that `os.UserConfigDir()` will
hand back `~/Library/Application Support` on macOS, which differs from
the spec. Decision needed: either follow `os.UserConfigDir()` (idiomatic,
respects XDG and macOS conventions) or hard-code `~/.config/canvacli`
(matches the spec exactly, surprises macOS users). Recommend the former.

```go
func tokenPath() (string, error) {
    base, err := os.UserConfigDir()
    if err != nil {
        return "", err
    }
    return filepath.Join(base, "canvacli", "token.json"), nil
}

func saveToken(t *oauth2.Token) error {
    p, err := tokenPath()
    if err != nil {
        return err
    }
    if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
        return err
    }
    // Write to a temp file then rename — atomic, leaks no partial token.
    tmp, err := os.CreateTemp(filepath.Dir(p), "token-*.json")
    if err != nil {
        return err
    }
    if err := os.Chmod(tmp.Name(), 0o600); err != nil {
        tmp.Close()
        return err
    }
    if err := json.NewEncoder(tmp).Encode(t); err != nil {
        tmp.Close()
        return err
    }
    if err := tmp.Close(); err != nil {
        return err
    }
    return os.Rename(tmp.Name(), p)
}
```

`os.Chmod` is a no-op on Windows (the call succeeds, the bit pattern is
ignored). For real Windows hardening you'd need
`golang.org/x/sys/windows` ACLs — out of scope for v1; document it as a
known limitation.

`*oauth2.Token` already serialises cleanly to JSON: it has tagged
`access_token`, `token_type`, `refresh_token`, and `expiry` fields plus
an `Extra` map for provider-specific extras.

## 4. Auto-refresh middleware

Two layers needed:

**Layer A — clock-driven refresh (free):** wrap the cached token in a
`TokenSource` and let `oauth2.NewClient` install a `Transport` that
refreshes whenever `tok.Valid()` returns false. The transport never
inspects 401s — it goes off `Expiry`. To persist the refreshed token,
wrap the source so we get a callback:

```go
type persistingSource struct {
    inner oauth2.TokenSource
    save  func(*oauth2.Token) error
    last  *oauth2.Token
}

func (p *persistingSource) Token() (*oauth2.Token, error) {
    t, err := p.inner.Token()
    if err != nil {
        return nil, err
    }
    if p.last == nil || t.AccessToken != p.last.AccessToken {
        if err := p.save(t); err != nil {
            return nil, err
        }
        p.last = t
    }
    return t, nil
}

func newClient(ctx context.Context, conf *oauth2.Config, cached *oauth2.Token) *http.Client {
    base := conf.TokenSource(ctx, cached)
    src := &persistingSource{inner: base, save: saveToken, last: cached}
    return oauth2.NewClient(ctx, oauth2.ReuseTokenSource(cached, src))
}
```

`oauth2.ReuseTokenSource` caches in memory so refreshes only happen when
needed. Reference: <https://pkg.go.dev/golang.org/x/oauth2#ReuseTokenSource>.

**Layer B — server-revocation 401 retry:** Layer A handles expiry but
not "user revoked their integration in Canva's dashboard". For that we
need a custom `http.RoundTripper` that, on 401, calls `src.Token()` once
and replays the request. If the second response is also 401, surface a
typed `ErrAuthRevoked` so callers can prompt re-login.

```go
type refreshOn401 struct {
    base http.RoundTripper
    src  oauth2.TokenSource
}

func (r *refreshOn401) RoundTrip(req *http.Request) (*http.Response, error) {
    resp, err := r.base.RoundTrip(req)
    if err != nil || resp.StatusCode != http.StatusUnauthorized {
        return resp, err
    }
    resp.Body.Close()
    // Force-refresh: invalidate the token and ask src for a new one.
    // Easiest path: call /oauth/token directly with the refresh token —
    // oauth2's TokenSource won't refresh a token it considers Valid().
    // Implementation detail left for the implementer.
    // ...
    return r.base.RoundTrip(req.Clone(req.Context()))
}
```

Edge case: request body has been consumed. Buffer the body before the
first send (`req.GetBody`) so retry is safe. Use `http.NewRequest` (not
`NewRequestWithContext` from a `bytes.Buffer`) and set `req.GetBody` so
the stdlib retry helpers work.

Give-up rule: surface `ErrAuthRevoked` after one failed refresh. Don't
spin.

## 5. Browser opening

`github.com/pkg/browser` is 100 lines, BSD-2, zero deps, supports
darwin/linux/windows/wasm, and is used by ~1.8k packages. Reference:
<https://pkg.go.dev/github.com/pkg/browser>.

```go
import "github.com/pkg/browser"
_ = browser.OpenURL(authURL)
```

If you'd rather avoid the dep, the entire implementation is:

```go
func openBrowser(url string) error {
    var cmd *exec.Cmd
    switch runtime.GOOS {
    case "darwin":
        cmd = exec.Command("open", url)
    case "windows":
        cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
    default: // linux, freebsd, etc.
        cmd = exec.Command("xdg-open", url)
    }
    return cmd.Start()
}
```

Either way, **always** print the URL to stdout as a fallback for headless
environments (SSH, WSL without `wslu`, containers). Pattern:

```
Opening browser to authorise canvacli...
If nothing happens, visit this URL manually:
  https://www.canva.com/api/oauth/authorize?...
```

## Open questions for the implementer

1. Does Canva accept PKCE *without* a client secret for public clients?
   If not, we ship the secret in the binary and rely on PKCE for safety.
2. Does Canva allow wildcard ports on registered redirect URIs, or do we
   need a fixed port (and a fallback list)?
3. Refresh token rotation: Canva docs say refresh tokens are single-use.
   Confirm the response body always returns a fresh `refresh_token` —
   our `persistingSource` assumes so.

## Sources

- [golang.org/x/oauth2 package docs (PKCE)](https://pkg.go.dev/golang.org/x/oauth2#GenerateVerifier)
- [oauth2.Config.Client / TokenSource / ReuseTokenSource](https://pkg.go.dev/golang.org/x/oauth2#Config.Client)
- [os.UserConfigDir](https://pkg.go.dev/os#UserConfigDir)
- [Canva Connect API authentication](https://www.canva.dev/docs/connect/authentication/)
- [github.com/pkg/browser](https://pkg.go.dev/github.com/pkg/browser)
- [RFC 8252 — OAuth 2.0 for Native Apps](https://datatracker.ietf.org/doc/html/rfc8252)
- [RFC 7636 — PKCE](https://datatracker.ietf.org/doc/html/rfc7636)
- [Auth0 PKCE Go CLI gist](https://gist.github.com/ogazitt/f749dad9cca8d0ac6607f93a42adf322)
- [int128/oauth2cli — reference implementation](https://pkg.go.dev/github.com/int128/oauth2cli)
- [cli/oauth — GitHub CLI's OAuth lib](https://pkg.go.dev/github.com/cli/oauth)
