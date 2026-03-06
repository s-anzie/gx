# GX Core — Design Document

> **Version** 0.1.0-concept  
> **Status** Design Phase  
> **Couche** Core (transport & routing)  
> **Dépend de** `net/http` · `golang.org/x/net/http2` · `quic-go/http3`

---

## Table of Contents

1. [Rôle du Core](#1-rôle-du-core)
2. [Handler](#2-handler)
3. [Context](#3-context)
4. [Response](#4-response)
5. [Router](#5-router)
6. [Middleware](#6-middleware)
7. [Group](#7-group)
8. [Serveur Dual-Stack](#8-serveur-dual-stack)
9. [SSE — Server-Sent Events](#9-sse--server-sent-events)
10. [TLS](#10-tls)
11. [Conventions internes](#11-conventions-internes)
12. [Decisions Log](#12-decisions-log)

---

## 1. Rôle du Core

Le Core est la fondation de tout. GX construit dessus, mais le Core reste **utilisable seul**, sans aucune dépendance vers la couche GX.

### Responsabilités

- Recevoir et router les requêtes HTTP/1.1, HTTP/2, HTTP/3
- Fournir un `Context` riche et poolé pour chaque requête
- Exposer un système de `Response` chainable
- Gérer la chaîne middleware avec accès à la réponse finale
- Démarrer et arrêter proprement les serveurs TCP et UDP

### Ce que le Core ne fait pas

Il ne valide pas, ne génère pas d'OpenAPI, ne trace pas, ne gère pas de plugins, ne connaît pas le concept d'erreur métier. Ce sont des responsabilités de la couche GX.

### Stack

```
┌──────────────────────────────────────┐
│            Application               │
├──────────────────────────────────────┤
│            GX Layer                  │
├──────────────────────────────────────┤
│  >>>       CORE LAYER       <<<      │  ← ce document
│                                      │
│  Handler · Context · Response        │
│  Router · Middleware · Group         │
│  SSE · TLS · Dual-Stack Server       │
├──────────────┬───────────────────────┤
│  net/http    │  x/net/http2          │
├──────────────┴───────────────────────┤
│         quic-go/http3                │
└──────────────────────────────────────┘
```

### Request Flow

```
Client
  │
  ├─ TCP  → HTTP/2  ─┐
  └─ UDP  → HTTP/3  ─┤
                     ↓
              [ ServeHTTP ]
                     ↓
             Context (poolé)
                     ↓
           Chaîne Middleware
                     ↓
                  Handler
                     ↓
               Response ↩
           (remonte la chaîne)
                     ↓
              Write(w) → Client
```

---

## 2. Handler

### Signature

```go
type Handler func(*Context) Response
```

C'est la décision centrale du framework. Tout en découle.

### Pourquoi cette signature

| Signature | Framework | Verdict | Raison |
|-----------|-----------|---------|--------|
| `func(*Context)` | Gin | ✗ | Pas de valeur de retour — effets de bord, oubli de `return` silencieux |
| `func(*Context) error` | Echo | ~ | Une erreur et une réponse normale sont deux chemins différents pour faire la même chose : écrire au client |
| `func(*Context) (Response, error)` | — | ~ | Redondant — une erreur est une Response. Double chemin inutile |
| `func(*Context) Response` | GX Core | ✓ | Unifié, testable unitairement, compilateur enforced |

### Règle fondamentale

Une erreur est une `Response`. `c.Fail(ErrNotFound)` retourne une `Response` d'erreur — pas une `error` Go. Les deux cas passent par le même chemin de code.

### Exemple

```go
func getUser(c *Context) Response {
    id := c.Param("id")

    user, err := db.FindUser(c.GoContext(), id)
    if err != nil {
        return c.Fail(ErrUserNotFound)   // erreur = Response
    }

    return c.JSON(user)                  // succès = Response
}
```

### Testabilité

L'avantage immédiat : un handler est testable sans `httptest.Server`, sans HTTP réel.

```go
func TestGetUser(t *testing.T) {
    res := getUser(MockContext("id", "123"))
    assert.Equal(t, 200, res.Status())
}

func TestGetUserNotFound(t *testing.T) {
    res := getUser(MockContext("id", "unknown"))
    assert.Equal(t, 404, res.Status())
}
```

### Comparaison avec Express

```
Express (JS)                          GX Core (Go)
─────────────────────────────────     ─────────────────────────────────
res.json(data)                        return c.JSON(data)
  → oubli de return = double send       → compilateur refuse
res.status(404).json({error})         return c.Fail(ErrNotFound)
  → side effect sur l'objet res         → valeur retournée
Impossible à tester sans supertest    Test unitaire pur
next(err) = chemin séparé             c.Fail() = même chemin
```

---

## 3. Context

### Concept

Le `Context` est le point d'entrée unique pour toute interaction avec la requête et la réponse dans un handler ou un middleware. Il est **poolé** via `sync.Pool` — zéro allocation par requête sur le hot path.

### Structure interne

```go
type Context struct {
    // Accès brut — disponibles mais rarement nécessaires
    Request  *http.Request
    Writer   http.ResponseWriter

    // Interne — non exportés, gérés par le framework
    params   Params
    handlers []Handler
    index    int
    store    map[string]any
    app      *App
    written  bool
}
```

### Méthodes — Lecture de la requête

```go
// Routing
c.Param("id")                    // paramètre de path :id → string
c.Query("page")                  // query string → string (vide si absent)
c.QueryDefault("limit", "20")    // query string avec fallback

// Corps
c.BindJSON(&dto)                 // désérialise le body JSON dans dto → error
c.BindXML(&dto)                  // désérialise le body XML dans dto → error
c.Body()                         // body brut → ([]byte, error)

// Formulaires
c.FormValue("field")             // application/x-www-form-urlencoded ou multipart

// Métadonnées
c.Header("Authorization")        // header de requête → string
c.ClientIP()                     // IP réelle, respecte X-Forwarded-For → string
c.Method()                       // méthode HTTP → string
c.Path()                         // path de la requête → string

// Protocole
c.Proto()                        // "HTTP/1.1" | "HTTP/2.0" | "HTTP/3" → string
c.IsHTTP2()                      // → bool
c.IsHTTP3()                      // → bool

// Context Go standard
c.GoContext()                    // → context.Context — pour propagation aux libs tierces
```

### Méthodes — Builders de Response

Les méthodes de réponse ne font **pas** d'écriture immédiate — elles construisent et retournent une `Response`. C'est le framework qui appelle `Write()` une fois la chaîne terminée.

```go
c.JSON(v)                        // 200 + application/json
c.Created(v)                     // 201 + application/json
c.NoContent()                    // 204
c.Text(format, args...)          // 200 + text/plain
c.HTML(html)                     // 200 + text/html
c.File(path)                     // stream fichier, Content-Type détecté
c.Stream(contentType, reader)    // stream générique depuis un io.Reader
c.Redirect(url)                  // 302 Found
c.Fail(appErr)                   // réponse d'erreur structurée
```

### Store par requête

Permet de transmettre des valeurs entre middleware et handlers dans le contexte d'une même requête.

```go
// Écriture — typiquement dans un middleware
c.Set("user", claims)

// Lecture — typiquement dans un handler
user, ok := c.Get("user")           // (any, bool)
user     := c.MustGet("user")       // any — panic si absent

// Avec GX layer — typé sans cast manuel
user := gx.Typed[AuthClaims](c)     // *AuthClaims — panic si absent ou mauvais type
user, ok := gx.TryTyped[AuthClaims](c)  // (*AuthClaims, bool)
```

### Contrôle de chaîne

```go
c.Next()                         // exécute le handler suivant dans la chaîne
c.Abort()                        // arrête la chaîne sans écrire de réponse
c.AbortWithStatus(code)          // arrête et écrit un status code nu
```

`Abort()` n'est utilisé qu'en dernier recours — le pattern recommandé est de retourner une `Response` de `c.Fail()` ce qui arrête naturellement la propagation.

### Pooling — Zéro allocation

```go
// Interne — géré par ServeHTTP
ctx := ctxPool.Get().(*Context)
ctx.reset(w, r)
defer ctxPool.Put(ctx)

// reset() remet à zéro tous les champs
// le store est cleared (pas réalloué)
// les slices handlers et params sont nil-ées
```

Le pool élimine l'allocation d'un `Context` à chaque requête. Pour 10 000 req/s, c'est 10 000 allocations de moins par seconde sur le GC.

**Conséquence importante** : ne jamais stocker un `*Context` au-delà du handler. Il sera réutilisé par une autre requête. Si une goroutine en a besoin plus longtemps, extraire les données nécessaires avant de lancer la goroutine.

```go
// Incorrect — c sera réutilisé
go func() { doSomething(c) }()

// Correct — extraire ce dont on a besoin
id := c.Param("id")
ctx := c.GoContext()
go func() { doSomething(ctx, id) }()
```

---

## 4. Response

### Interface

`Response` est une interface, pas une struct. Cela permet aux plugins GX de la wrapper sans casser le contrat du Core.

```go
type Response interface {
    Status()  int
    Headers() http.Header
    Write(w http.ResponseWriter) error
}
```

L'implémentation concrète (`jsonResponse`, `fileResponse`, etc.) est **non-exportée**. On ne construit jamais une Response directement, on passe toujours par les builders du Context.

### Builders — Référence complète

```go
// ── Succès ────────────────────────────────────────────────────────
c.JSON(v)                          // 200 + application/json
c.Created(v)                       // 201 + application/json
c.NoContent()                      // 204 — pas de corps
c.Text(format, args...)            // 200 + text/plain
c.HTML(html)                       // 200 + text/html
c.File(path)                       // stream fichier — Content-Type par extension
c.Stream(contentType, reader)      // stream depuis io.Reader

// ── Redirection ───────────────────────────────────────────────────
c.Redirect(url)                    // 302 Found
c.Redirect(url).Permanent()        // 301 Moved Permanently

// ── Erreur ────────────────────────────────────────────────────────
c.Fail(appErr)                     // status + code + message de AppError
c.Fail(appErr).With(key, value)    // contexte additionnel dans Details
c.Fail(appErr).Wrap(err)           // cause Go interne — loggée, non-exposée client
```

### Chaining — Immutable et Fluent

Chaque méthode de chaining retourne une **nouvelle** Response. L'original n'est pas muté. Cela rend le comportement prévisible et les tests triviaux.

```go
// Créer une ressource
return c.JSON(user).
    Status(201).
    Header("Location", "/users/"+user.ID).
    Header("X-Resource-ID", user.ID)

// Liste paginée avec metadata
return c.JSON(users).
    Header("X-Total-Count", strconv.Itoa(total)).
    Header("X-Page", page).
    Cache(30 * time.Second)

// Redirect permanent
return c.Redirect("/new-path").Permanent()

// Erreur avec contexte
return c.Fail(ErrEmailTaken).
    With("email", req.Email).
    With("suggestion", "Try signing in instead")
```

### Méthodes de chaining disponibles

```go
.Status(code int)              // override le status code
.Header(key, value string)     // ajoute un header de réponse
.Cache(d time.Duration)        // Cache-Control: max-age=...
.NoCache()                     // Cache-Control: no-store
.Permanent()                   // pour Redirect — passe 302 en 301
.With(key string, value any)   // pour Fail — ajoute dans Details
.Wrap(err error)               // pour Fail — attache une cause interne
```

### Écriture — Quand et comment

Le framework appelle `Write(w)` **une seule fois**, après que la chaîne middleware ait terminé. Le handler ne décide pas du moment d'écriture — il produit une valeur, le framework s'occupe du reste.

```go
// Dans ServeHTTP — simplifié
res := chain.execute(ctx)   // toute la chaîne s'exécute
res.Write(w)                // une seule écriture à la fin
```

---

## 5. Router

### Algorithme

Radix-tree par méthode HTTP. Un arbre distinct pour `GET`, `POST`, `PUT`, etc. Lookup en O(log n), zéro allocation pour les routes statiques en production.

### Syntaxe des paramètres

```
:id              Paramètre obligatoire       /users/42        → id = "42"
:id?             Paramètre optionnel         /users/          → id = ""
*path            Wildcard                    /files/a/b/c     → path = "a/b/c"
v:version        Paramètre partiel           /v2/users        → version = "2"
```

Le wildcard `*path` capture tout le reste du chemin, slash compris. Il ne peut apparaître qu'en fin de route.

### Priorité de résolution

Quand plusieurs routes peuvent matcher un même path :

```
1. Route statique exacte      /users/me
2. Route paramétrique         /users/:id
3. Route wildcard             /users/*path
```

Exemple : `/users/me` matche la route statique, pas `/:id`. C'est déterministe et déclaratif.

### Détection des conflits

Les conflits sont détectés **au boot**, jamais au runtime.

```go
app.GET("/users/:id",  handler)
app.GET("/users/:uid", handler)  // panic au démarrage — deux params sur le même segment
```

Un conflit irresolvable arrête le démarrage immédiatement avec un message clair indiquant les deux routes en conflit.

### Méthodes disponibles

```go
app.GET(path, handlers...)
app.POST(path, handlers...)
app.PUT(path, handlers...)
app.PATCH(path, handlers...)
app.DELETE(path, handlers...)
app.OPTIONS(path, handlers...)
app.HEAD(path, handlers...)
app.Any(path, handlers...)       // toutes les méthodes
```

### Exemple d'organisation

```go
app := core.New()

// Routes publiques
app.GET("/",       homeHandler)
app.GET("/health", healthHandler)

// API versionnée
v1 := app.Group("/api/v1")
v1.Use(requestID(), logger())

// Ressource users
users := v1.Group("/users", requireAuth)
users.GET("",        listUsers)      // GET    /api/v1/users
users.GET("/:id",    getUser)        // GET    /api/v1/users/:id
users.POST("",       createUser)     // POST   /api/v1/users
users.PATCH("/:id",  updateUser)     // PATCH  /api/v1/users/:id
users.DELETE("/:id", deleteUser)     // DELETE /api/v1/users/:id

// Sous-ressource
posts := users.Group("/:userId/posts")
posts.GET("",      listUserPosts)    // GET    /api/v1/users/:userId/posts
posts.GET("/:id",  getUserPost)      // GET    /api/v1/users/:userId/posts/:id

// Wildcard
app.GET("/static/*path", serveStatic)
```

Convention : pas de slash trailing. La racine d'un groupe s'enregistre avec `""`, pas `"/"`.

### Params — Type

```go
type Params map[string]string

// Accès
id     := c.Param("id")         // string vide si absent
userID := c.Param("userId")
path   := c.Param("path")       // wildcard — chemin complet
```

---

## 6. Middleware

### Signature — La différence fondamentale

```go
type Middleware func(c *Context, next Handler) Response
```

Le middleware **reçoit la Response du handler en retour** de `next(c)`. Il peut l'observer, la modifier, ou la remplacer avant de la retourner à son tour. C'est structurellement impossible dans Express où `next()` est un callback fire-and-forget.

### Patterns d'utilisation

**Avant et après le handler :**

```go
func timer(c *Context, next Handler) Response {
    start := time.Now()

    res := next(c)    // ← handler s'exécute ici

    // Après — accès complet à la Response produite
    return res.Header("X-Duration-Ms", strconv.FormatInt(time.Since(start).Milliseconds(), 10))
}
```

**Court-circuit — arrêt de la chaîne :**

```go
func requireAuth(c *Context, next Handler) Response {
    token := c.Header("Authorization")
    if token == "" {
        return c.Fail(ErrUnauthorized)   // next() jamais appelé
    }

    claims, err := verifyJWT(token)
    if err != nil {
        return c.Fail(ErrUnauthorized).Wrap(err)
    }

    c.Set("claims", claims)
    return next(c)    // continue la chaîne
}
```

**Modification de la réponse :**

```go
func addCORSHeaders(c *Context, next Handler) Response {
    res := next(c)
    return res.
        Header("Access-Control-Allow-Origin", "*").
        Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE")
}
```

**Récupération de panic :**

```go
func recovery(c *Context, next Handler) Response {
    defer func() {
        if r := recover(); r != nil {
            log.Printf("panic: %v\n%s", r, debug.Stack())
        }
    }()
    return next(c)
}
```

### Enregistrement

```go
// Global — s'applique à toutes les routes
app.Use(recovery(), logger(), requestID())

// Par groupe — s'applique aux routes du groupe
api := app.Group("/api", requireAuth)

// Par route — s'applique à cette route uniquement
app.GET("/admin", requireRole("admin"), adminHandler)

// Plusieurs handlers sur une même route — équivalent à middleware local
app.POST("/payments",
    validatePaymentBody,
    requireKYC,
    processPayment,
)
```

### Ordre d'exécution

```
Requête entrante
       ↓
   recovery()         ← Use global 1
       ↓
    logger()          ← Use global 2  (avant next)
       ↓
  requestID()         ← Use global 3
       ↓
  requireAuth()       ← middleware de groupe
       ↓
    handler()         ← handler final
       ↓
   Response ↩
       ↑
  requestID()         ← Use global 3  (après next)
       ↑
    logger()          ← Use global 2  (après next — log durée, status)
       ↑
   recovery()         ← Use global 1  (après next)
       ↑
   Write(w)           ← écriture unique
```

### Middleware standard fournis

```go
Logger()                        // log méthode + path + durée + proto
Recovery()                      // panic → Response 500 structurée
RequestID()                     // attache X-Request-ID (génère si absent)
SecureHeaders()                 // HSTS, X-Frame-Options, X-XSS-Protection...
NoCache()                       // Cache-Control: no-store, no-cache
BasicAuth(credentials)          // HTTP Basic Auth — map[user]pass
Timeout(d time.Duration)        // abort si handler dépasse la durée
```

---

## 7. Group

### Concept

Un `Group` est un sous-router qui partage un préfixe de chemin et un ensemble de middleware. Il n'introduit pas de coût runtime — les routes sont développées et enregistrées dans l'arbre racine au moment de la déclaration.

### Interface

```go
type Group struct { /* ... */ }

func (g *Group) Use(mw ...Middleware)
func (g *Group) GET(path string, handlers ...Handler)
func (g *Group) POST(path string, handlers ...Handler)
func (g *Group) PUT(path string, handlers ...Handler)
func (g *Group) PATCH(path string, handlers ...Handler)
func (g *Group) DELETE(path string, handlers ...Handler)
func (g *Group) OPTIONS(path string, handlers ...Handler)
func (g *Group) Group(prefix string, mw ...Middleware) *Group
```

### Imbrication

```go
app := core.New()

// Premier niveau
v1 := app.Group("/api/v1")
v1.Use(requestID(), logger())

// Deuxième niveau — hérite des middleware de v1
protected := v1.Group("/protected", requireAuth)

// Troisième niveau — hérite de v1 + protected
admin := protected.Group("/admin", requireRole("admin"))
admin.GET("/stats", getStats)
// → GET /api/v1/protected/admin/stats
// → middleware : requestID + logger + requireAuth + requireRole
```

### Résolution des middleware

Au moment de l'enregistrement d'une route, la chaîne complète est construite par concaténation :

```
global middleware + group middleware + route middleware + handler
```

C'est une construction statique au boot, pas dynamique par requête.

---

## 8. Serveur Dual-Stack

### Concept

Un seul appel `ListenDual` démarre deux serveurs simultanément sur le même port :

- **TCP + TLS** pour HTTP/1.1 et HTTP/2
- **UDP + QUIC** pour HTTP/3

Le header `Alt-Svc` est injecté automatiquement sur toutes les réponses HTTP/2 pour annoncer la disponibilité de HTTP/3. Les navigateurs compatibles basculeront sur HTTP/3 dès la prochaine requête.

```
Client
  ├─ TCP :8443 ─→ HTTP/2 (TLS 1.3)
  └─ UDP :8443 ─→ HTTP/3 (QUIC)
```

### API

```go
app := core.New()

// ── TLS ──────────────────────────────────────────────────────────

// Dev — certificat self-signed généré en mémoire
app.WithDevTLS("localhost", "127.0.0.1")

// Production — fichiers PEM
app.WithTLS("cert.pem", "key.pem")

// Custom — config TLS existante
app.WithTLSConfig(tlsCfg)

// ── Démarrage ─────────────────────────────────────────────────────

// HTTP/2 seul (TCP)
app.Listen(":8443")

// HTTP/2 + HTTP/3 simultané (TCP + UDP)
app.ListenDual(":8443", ":8443")   // même port, protocoles différents

// ── Timeouts ──────────────────────────────────────────────────────
app.ReadTimeout  = 15 * time.Second   // défaut
app.WriteTimeout = 15 * time.Second   // défaut
app.IdleTimeout  = 60 * time.Second   // défaut
```

### TLS Configuration interne

```go
// Config TLS minimale appliquée automatiquement
tls.Config{
    MinVersion: tls.VersionTLS13,
    NextProtos: []string{"h3", "h2", "http/1.1"},
}
```

TLS 1.3 minimum. HTTP/3 requiert TLS 1.3 par spec QUIC (RFC 9001).

### Alt-Svc Header

Injecté automatiquement sur chaque réponse HTTP/2 quand HTTP/3 est disponible :

```
Alt-Svc: h3=":8443"; ma=86400
```

`ma` (max-age) : durée en secondes pendant laquelle le client peut mémoriser l'annonce. 86400 = 24h.

### Graceful Shutdown

Déclenché automatiquement sur `SIGTERM` ou `SIGINT`.

```
SIGTERM reçu
    ↓
Stop d'accepter de nouvelles connexions (TCP + QUIC)
    ↓
Attente drain des requêtes en cours
    │  timeout : 30 secondes (configurable)
    ↓
Fermeture des serveurs TCP et QUIC
    ↓
Exit code 0
```

```go
// Configurer la fenêtre de drain
app.ShutdownTimeout = 30 * time.Second   // défaut
```

### Comparaison HTTP/2 vs HTTP/3

| Feature | HTTP/2 (TCP) | HTTP/3 (QUIC/UDP) |
|---|---|---|
| Multiplexing | ✓ | ✓ |
| Head-of-line blocking | ✗ au niveau TCP | ✓ éliminé |
| 0-RTT reconnect | ✗ | ✓ |
| Connection migration | ✗ | ✓ (wifi → 4G transparent) |
| Server Push | ✓ | ✗ (remplacé par Early Hints) |
| Déploiement | Universel | Requiert UDP ouvert |

---

## 9. SSE — Server-Sent Events

### Concept

Les Server-Sent Events permettent au serveur de pousser des données vers le client sur une connexion HTTP longue durée. Particulièrement efficaces sur HTTP/2 grâce au multiplexing — un seul flux parmi d'autres, sans bloquer.

### API

```go
app.GET("/stream", func(c *Context) Response {
    sse := c.SSE()     // prépare les headers, retourne un SSEWriter
    defer sse.Close()

    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()

    for {
        select {
        case t := <-ticker.C:
            sse.SendJSON("tick", map[string]any{
                "time": t.Format(time.RFC3339),
            })
        case <-c.GoContext().Done():
            return c.NoContent()
        }
    }
})
```

### Interface SSEWriter

```go
type SSEWriter interface {
    Send(event, data string)            // event brut
    SendJSON(event string, v any) error // sérialise en JSON
    SendComment(comment string)         // keepalive — ": ping"
    Close()
}
```

### Headers automatiques

`c.SSE()` positionne automatiquement :

```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no
```

Le header `X-Accel-Buffering: no` est nécessaire pour désactiver le buffering Nginx qui briserait le streaming.

### Format SSE

```
event: tick
data: {"time":"2024-01-15T10:30:00Z"}

: keepalive

event: update
data: {"id":42,"status":"done"}

```

---

## 10. TLS

### Génération Self-Signed (dev)

```go
// Génère cert + clé en mémoire, retourne une *tls.Config prête à l'emploi
app.WithDevTLS("localhost", "127.0.0.1", "::1")
```

Caractéristiques du certificat généré :

- Algorithme : ECDSA P-256
- Durée : 365 jours
- Usage : serverAuth
- SAN : les hosts passés en argument
- CA : self-signed (IsCA: true)

**Ne jamais utiliser en production.** Le framework refuse explicitement un certificat self-signed si `Environment == Production`.

### Chargement depuis fichiers

```go
app.WithTLS("cert.pem", "key.pem")
```

Accepte tout certificat PEM valide (Let's Encrypt, DigiCert, etc.). La clé doit correspondre au certificat.

### Config TLS Custom

```go
cfg := &tls.Config{
    GetCertificate: certManager.GetCertificate,   // Let's Encrypt auto-renew
    MinVersion:     tls.VersionTLS13,
    NextProtos:     []string{"h3", "h2", "http/1.1"},
}
app.WithTLSConfig(cfg)
```

**Important** : si une config custom est fournie, le framework s'assure que `NextProtos` contient bien `"h3"` et `"h2"` si les deux protocoles sont actifs. Il complète la config si nécessaire, il ne l'écrase pas.

---

## 11. Conventions Internes

### Nommage

```
Types exportés        PascalCase                Context, Handler, Response, Params
Méthodes              camelCase                 ServeHTTP, ListenDual, WithTLS
Variables internes    camelCase                 ctxPool, radixTree
Constantes            PascalCase                Production, Development
Fichiers              snake_case.go             context.go, radix_tree.go
```

### Organisation des fichiers

```
core/
├── app.go              App, Listen, ListenDual, ServeHTTP
├── context.go          Context, pool, méthodes de lecture et de réponse
├── response.go         Interface Response, tous les builders et implémentations
├── router.go           Router, radix-tree, conflict detection
├── middleware.go       type Middleware, chaîne d'exécution, middleware standard
├── group.go            Group, préfixes, héritage middleware
├── params.go           type Params, helpers
├── sse.go              SSEWriter, c.SSE()
└── tls.go              WithTLS, WithDevTLS, WithTLSConfig
```

### Gestion des panics

Le Core ne récupère pas les panics par défaut. C'est le middleware `Recovery()` qui s'en charge, et son utilisation est optionnelle (mais fortement recommandée). Cette séparation est intentionnelle : le Core ne fait pas de choix sur le format des erreurs non-catchées.

### Concurrence

- Le `Context` n'est pas thread-safe. Il est conçu pour être utilisé par une seule goroutine (la goroutine de la requête).
- Si une goroutine lancée depuis un handler a besoin de données de la requête, extraire les données avant de lancer la goroutine.
- Le `Router` est read-only après le boot — aucun lock nécessaire en lecture.
- L'`App` en entier est read-only après le démarrage du serveur.

### Zero-value Safety

`App`, `Group`, et `Context` doivent toujours être créés via leurs constructeurs (`core.New()`, `app.Group()`, `acquireContext()`). Leur zero-value n'est pas un état valide.

---

## 12. Decisions Log

### CDL-001 — Handler retourne Response, pas error

**Décision** `func(*Context) Response`

**Alternatives** `func(*Context) error` · `func(*Context) (Response, error)` · `func(*Context)`

**Raison** Une erreur est une Response. Unifier les deux chemins élimine les patterns incohérents et rend les handlers testables sans infrastructure HTTP.

---

### CDL-002 — Response est une interface, pas une struct

**Décision** `type Response interface { Status() int; Headers() http.Header; Write(w) error }`

**Alternatives** struct concrète exportée, struct concrète non-exportée

**Raison** Permet aux plugins GX de wrapper une Response sans briser le contrat. Un plugin de cache peut retourner une `cachedResponse` qui implémente la même interface.

---

### CDL-003 — La Response remonte la chaîne middleware

**Décision** `type Middleware func(c *Context, next Handler) Response`

**Alternatives** `next()` callback comme Express, `defer` pattern, double-pass

**Raison** Le middleware peut observer et modifier la réponse après que le handler l'ait produite. Le logging de status code, l'injection de headers CORS, la modification des erreurs — tous ces cas sont impossibles avec le modèle callback d'Express.

---

### CDL-004 — Context poolé via sync.Pool

**Décision** Pool de `*Context` réutilisés entre requêtes

**Alternatives** Allocation par requête, stack allocation

**Raison** Sur un serveur HTTP/2 à forte charge (multiplexing = nombreuses requêtes concurrentes), réduire la pression sur le GC est mesurable. Le coût est une contrainte d'usage documentée : ne pas stocker le Context au-delà du handler.

---

### CDL-005 — Conflits de routes détectés au boot

**Décision** Panic immédiat si deux routes entrent en conflit non-résolvable

**Alternatives** Dernière route enregistrée gagne, erreur retournée au caller

**Raison** Un conflit de routing est un bug de programmation, pas une condition d'erreur runtime. Le faire échouer tôt (au démarrage) avec un message clair est préférable à un comportement imprévisible en production.

---

### CDL-006 — TLS 1.3 minimum

**Décision** `tls.VersionTLS13` comme version minimale

**Alternatives** TLS 1.2 pour compatibilité maximale

**Raison** HTTP/3 impose TLS 1.3 (RFC 9001). Uniformiser sur TLS 1.3 simplifie la config et améliore la sécurité. TLS 1.2 représente moins de 2% du trafic global en 2024.

---

### CDL-007 — ServeHTTP comme point d'entrée unique

**Décision** `App` implémente `http.Handler` via `ServeHTTP`

**Alternatives** Adapter séparé, wrapping de la stdlib

**Raison** Compatibilité totale avec l'écosystème Go. N'importe quelle lib qui accepte `http.Handler` peut utiliser une `App` GX Core directement — tests, proxies, intégration dans des apps existantes.

---

*Document vivant — mis à jour à chaque décision de conception validée sur le Core.*