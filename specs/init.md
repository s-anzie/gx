# GX — Framework Design Document

> **Version** 0.1.0-concept  
> **Status** Design Phase  
> **Language** Go 1.22+  
> **Protocols** HTTP/1.1 · HTTP/2 · HTTP/3 (QUIC)

---

## Table of Contents

1. [Vision & Philosophy](#1-vision--philosophy)
2. [Architecture Overview](#2-architecture-overview)
3. [Core Layer — Transport & Routing](#3-core-layer--transport--routing)
4. [GX Layer — Application Framework](#4-gx-layer--application-framework)
5. [Contracts & Schema](#5-contracts--schema)
6. [Error Taxonomy](#6-error-taxonomy)
7. [Middleware & Plugin System](#7-middleware--plugin-system)
8. [Observability](#8-observability)
9. [App Lifecycle](#9-app-lifecycle)
10. [QUIC & Real-Time Primitives](#10-quic--real-time-primitives)
11. [Configuration](#11-configuration)
12. [Conventions & DX](#12-conventions--dx)
13. [Standard Plugins](#13-standard-plugins)
14. [Package Structure](#14-package-structure)
15. [Design Decisions Log](#15-design-decisions-log)

---

## 1. Vision & Philosophy

### Ce que GX n'est pas

GX n'est pas un wrapper autour de la stdlib. Il n'est pas non plus un port d'Express en Go. Il emprunte à Express son **minimalisme et sa lisibilité**, mais tire pleinement parti de ce que Go apporte : typage statique, concurrence native, valeurs de retour multiples, interfaces structurelles.

### Les trois principes

**1. Explicite plutôt que magique**  
Aucun comportement ne se produit sans que le développeur l'ait déclaré. Pas de tags de struct incantatoires, pas de réflexion cachée à l'exécution, pas de globals implicites.

**2. Composable plutôt que monolithique**  
La couche Core est utilisable sans la couche GX. La couche GX est utilisable sans les plugins standard. Chaque concept est indépendant et testable en isolation.

**3. Le framework travaille, le développeur exprime**  
La validation, la documentation, l'observabilité et la gestion d'erreur ne sont pas des tâches à configurer — ce sont des conséquences automatiques d'avoir correctement défini ce que fait son app.

### Positionnement

```
Stdlib          Bas niveau, verbose, aucune opinion
Core (GX)       Routing + Context + Response — inspiration Express
GX              Framework complet avec opinions claires         ← ici
Full-stack      Pas l'objectif (pas de template, pas d'ORM)
```

---

## 2. Architecture Overview

```
┌──────────────────────────────────────────────────────────────────┐
│                        APPLICATION                               │
│          (code métier du développeur, handlers, domaine)         │
├──────────────────────────────────────────────────────────────────┤
│                         GX LAYER                                 │
│                                                                  │
│   Contracts · Schema · Error Taxonomy · Observability            │
│   Plugin System · App Lifecycle · Channel (QUIC/WS)              │
├──────────────────────────────────────────────────────────────────┤
│                        CORE LAYER                                │
│                                                                  │
│   Router (radix-tree) · Context · Response (chainable)           │
│   Middleware · Group · Handler func(*Context) Response           │
├───────────────────┬──────────────────────────────────────────────┤
│   net/http        │  golang.org/x/net/http2                      │
│   HTTP/1.1        │  HTTP/2 + TLS                                │
├───────────────────┴──────────────────────────────────────────────┤
│                   quic-go/http3                                   │
│                   HTTP/3 + QUIC (UDP)                            │
└──────────────────────────────────────────────────────────────────┘
```

### Séparation des responsabilités

| Couche | Responsabilité | Dépendances |
|--------|---------------|-------------|
| **Core** | Transport, routing, request/response primitives | stdlib, quic-go |
| **GX** | Contracts, errors, plugins, lifecycle, observabilité | Core + OTEL |
| **Plugins** | Cache, auth, rate-limit, etc. | GX |
| **App** | Handlers, domaine | GX |

---

## 3. Core Layer — Transport & Routing

### 3.1 Handler Signature

La décision fondamentale du framework. Un handler retourne toujours une `Response`.

```go
type Handler func(*Context) Response
```

**Pourquoi pas `(Response, error)` ?**  
Parce qu'une erreur est une Response. Séparer les deux crée deux chemins de code distincts pour la même chose : écrire quelque chose au client. Un `c.Fail(ErrNotFound)` retourne une `Response` d'erreur — le handler de la retourner ou de la propager est une décision du développeur, pas du framework.

**Pourquoi pas `error` seul comme Echo ?**  
Parce que cela oblige à utiliser des side-effects (`c.JSON(...)` avant `return err`) pour les cas non-error, ce qui brise la cohérence.

### 3.2 Context

Le `Context` est **poolé** (`sync.Pool`) pour éviter les allocations par requête. Il est le point d'entrée unique pour toute interaction avec la requête et la réponse.

```go
type Context struct {
    // Accès brut si besoin
    Request  *http.Request
    Writer   http.ResponseWriter

    // Interne — non exportés
    params   Params
    handlers []Handler
    index    int
    store    map[string]any
    app      *App
}
```

**Méthodes de lecture — Request**

```go
// Routing
c.Param("id")                    // paramètre de path :id
c.Query("page")                  // query string
c.QueryDefault("limit", "20")    // avec fallback

// Corps
c.BindJSON(&dto)                 // désérialise + valide si Contract attaché
c.BindXML(&dto)
c.Body()                         // []byte brut

// Métadonnées
c.Header("Authorization")
c.ClientIP()
c.Proto()                        // "HTTP/1.1" | "HTTP/2.0" | "HTTP/3"
c.IsHTTP2() bool
c.IsHTTP3() bool
c.Method() string
c.Path() string

// Context Go standard (pour propagation aux libs tierces)
c.GoContext() context.Context
```

**Méthodes d'écriture — Response builders**

```go
c.JSON(v)                        // 200 + application/json
c.Created(v)                     // 201 + application/json
c.NoContent()                    // 204
c.Text(format, args...)          // 200 + text/plain
c.HTML(html)                     // 200 + text/html
c.File(path)                     // stream fichier avec Content-Type détecté
c.Stream(contentType, reader)    // stream générique
c.Redirect(url)                  // 302
c.Fail(appErr)                   // erreur structurée (voir §6)
```

**Store par requête**

```go
c.Set("user", user)              // typé any
c.Get("user")    (any, bool)
c.MustGet("user") any            // panic si absent
```

**Typage fort avec GX layer**

```go
// GX ajoute une fonction générique au-dessus du store
gx.Typed[UserClaims](c)          // récupère et caste, panic explicite si absent
gx.TryTyped[UserClaims](c)       // retourne (UserClaims, bool)
```

### 3.3 Response — Interface Chainable

`Response` est une interface, pas une struct. Cela permet aux plugins de la wrapper sans casser le contrat.

```go
type Response interface {
    Status() int
    Headers() http.Header
    Write(w http.ResponseWriter) error
}
```

Le builder est fluent et immutable (chaque méthode retourne une nouvelle Response) :

```go
return c.JSON(data)
    .Status(201)
    .Header("X-Resource-ID", id)
    .Header("Location", "/users/"+id)
    .Cache(5 * time.Minute)

return c.Redirect("/login")
    .Permanent()               // 301 au lieu de 302
    .Header("X-Reason", "session-expired")

return c.Fail(ErrNotFound)
    .With("resource", "user")
    .With("id", userID)
```

### 3.4 Router

**Algorithme** : radix-tree par méthode HTTP. O(log n) en lookup, zéro allocation pour les routes statiques.

**Syntaxe des paramètres**

```
/users/:id              param obligatoire
/users/:id?             param optionnel (nil si absent)
/files/*path            wildcard — capture le reste du chemin
/v:version/users        param en milieu de segment
```

**Conflits de routes** — résolus à l'enregistrement, pas au runtime. Ordre de priorité :

```
1. Route statique exacte     /users/me
2. Route paramétrique        /users/:id
3. Route wildcard            /users/*path
```

Un conflit non-résolvable **panic au boot**, pas à la première requête.

### 3.5 Middleware Core

Le middleware Core a accès au handler suivant — il peut observer et modifier la Response **après** son exécution.

```go
type Middleware func(c *Context, next Handler) Response
```

```go
// Exemple — logger qui voit la response finale
app.Use(func(c *Context, next Handler) Response {
    start := time.Now()
    res := next(c)
    log.Printf("%s %s → %d (%v)", c.Method(), c.Path(), res.Status(), time.Since(start))
    return res
})
```

C'est fondamentalement différent d'Express où `next()` est un callback fire-and-forget. Ici, **la Response remonte la chaîne** comme une valeur.

### 3.6 Groups

```go
app := core.New()

// Préfixe seul
v1 := app.Group("/api/v1")

// Préfixe + middleware local
admin := app.Group("/admin", requireAdmin)

// Imbrication
users := v1.Group("/users")
users.GET("",       listUsers)
users.GET("/:id",   getUser)
users.POST("",      createUser)
users.PATCH("/:id", updateUser)
users.DELETE("/:id",deleteUser)
```

Convention : pas de slash trailing. `""` désigne la racine du groupe.

### 3.7 Serveur Dual-Stack

```go
// HTTP/2 seul (TCP + TLS)
app.Listen(":8443")

// HTTP/2 + HTTP/3 simultanément
// Alt-Svc header injecté automatiquement pour l'upgrade navigateur
app.ListenDual(":8443", ":8443")  // TCP addr, UDP addr

// Custom TLS
app.WithTLS("cert.pem", "key.pem")
app.WithTLSConfig(tlsCfg)

// Dev — génère un certificat self-signed
app.WithDevTLS("localhost", "127.0.0.1")
```

---

## 4. GX Layer — Application Framework

La couche GX s'instancie au-dessus de Core. Elle partage le même router mais y ajoute un cycle de vie, un système de plugins, et des primitives de haut niveau.

```go
app := gx.New(
    gx.WithTracing("my-service"),
    gx.WithMetrics(),
    gx.WithStructuredLogs(),
)
```

`gx.App` **embed** `core.App` — toutes les méthodes Core sont disponibles directement. La couche GX n'est pas un wrapper, c'est une extension.

---

## 5. Contracts & Schema

### 5.1 Concept

Un Contract est une **valeur Go** qui décrit ce qu'un endpoint accepte et retourne. Il n'est pas généré depuis des annotations — il *est* la source de vérité.

```go
type Contract struct {
    Summary     string
    Description string
    Tags        []string
    Deprecated  bool
    Params      SchemaRef   // path params
    Query       SchemaRef   // query string
    Body        SchemaRef   // request body
    Output      SchemaRef   // réponse succès
    Errors      []AppError  // erreurs possibles déclarées
}
```

### 5.2 SchemaRef — Typage Générique

```go
// Déclarer un schéma à partir d'un type Go
gx.Schema[T]()     SchemaRef
```

Le schéma est inféré par réflexion **une seule fois au boot**, pas à chaque requête.

Les tags de struct utilisés :

```go
type CreateUserRequest struct {
    Name     string `json:"name"     validate:"required,min=2,max=100"`
    Email    string `json:"email"    validate:"required,email"`
    Role     string `json:"role"     validate:"oneof=user admin"  default:"user"`
    Age      int    `json:"age"      validate:"omitempty,min=18"`
}
```

Tags reconnus : `validate`, `default`, `example`, `description`. Pas de tags propriétaires — compatibilité avec l'écosystème existant (`go-playground/validator`).

### 5.3 Déclaration et Attachement

```go
// Déclarer le contract — valeur réutilisable, testable unitairement
var CreateUser = gx.Contract{
    Summary: "Create a new user",
    Tags:    []string{"users"},
    Body:    gx.Schema[CreateUserRequest](),
    Output:  gx.Schema[UserResponse](),
    Errors:  []gx.AppError{ErrEmailTaken, ErrInvalidPassword},
}

// Attacher au handler — le contract précède le handler
users.POST("", CreateUser, func(c *gx.Context) gx.Response {
    req := gx.Typed[CreateUserRequest](c)  // déjà validé, garanti non-nil
    // ...
})
```

Si la validation du `Body` échoue, le framework retourne une erreur `422` avant même d'appeler le handler. Le handler ne reçoit que des données valides.

### 5.4 OpenAPI Automatique

Le framework génère une spec OpenAPI 3.1 complète à partir des Contracts enregistrés.

```
GET  /openapi.json   → spec JSON
GET  /docs           → UI Swagger/Scalar (configurable)
```

La spec est construite au boot, pas régénérée à chaque requête. Elle est immutable en production.

```go
app.Install(gx.OpenAPI(
    gx.APITitle("My Service"),
    gx.APIVersion("2.0.0"),
    gx.APIDocsPath("/docs"),           // défaut
    gx.APIDisableInProduction(true),   // désactiver /docs en prod
))
```

### 5.5 Typed — Accès aux Données Validées

```go
// Dans un handler dont le Contract déclare Body: gx.Schema[CreateUserRequest]()
req := gx.Typed[CreateUserRequest](c)
// req est *CreateUserRequest, jamais nil ici — le framework a garanti la validation
```

Si `Typed` est appelé pour un type non enregistré dans le Contract : **panic au boot** lors d'une vérification statique, pas à l'exécution en production.

---

## 6. Error Taxonomy

### 6.1 AppError — La Brique de Base

```go
type AppError struct {
    Status  int            // HTTP status code
    Code    string         // machine-readable, snake_case
    Message string         // human-readable, anglais par défaut
    Details map[string]any // contexte additionnel optionnel
}
```

### 6.2 Déclaration des Erreurs du Domaine

Les erreurs sont déclarées comme des **variables de package**, pas des strings.

```go
// errors.go — dans le package domaine
var (
    ErrUserNotFound    = gx.E(404, "USER_NOT_FOUND",    "User does not exist")
    ErrEmailTaken      = gx.E(409, "EMAIL_TAKEN",       "Email already registered")
    ErrInvalidPassword = gx.E(422, "INVALID_PASSWORD",  "Password does not meet requirements")
    ErrPaymentDeclined = gx.E(402, "PAYMENT_DECLINED",  "Payment could not be processed")
    ErrUnauthorized    = gx.E(401, "UNAUTHORIZED",      "Authentication required")
    ErrForbidden       = gx.E(403, "FORBIDDEN",         "Insufficient permissions")
)
```

### 6.3 Utilisation dans les Handlers

```go
return c.Fail(ErrUserNotFound)

// Avec contexte additionnel
return c.Fail(ErrEmailTaken).
    With("email", req.Email).
    With("suggestion", "Try signing in instead")

// Wrapping d'une erreur Go standard
if err := db.Find(&user, id); err != nil {
    return c.Fail(ErrUserNotFound).Wrap(err)  // err loggé, non-exposé au client
}
```

### 6.4 Error Handler Global

```go
app.OnError(func(c *gx.Context, err gx.AppError) gx.Response {
    return c.JSON(map[string]any{
        "error": map[string]any{
            "code":    err.Code,
            "message": err.Message,
            "details": err.Details,
        },
        "request_id": c.MustGet("request_id"),
    }).Status(err.Status)
})
```

Un seul endroit. Format garanti identique sur toute l'application.

### 6.5 Erreurs de Validation

Générées automatiquement par le framework lors de l'échec d'un Contract. Format standardisé :

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Request validation failed",
    "details": {
      "fields": [
        { "field": "email", "rule": "email", "message": "must be a valid email" },
        { "field": "age",   "rule": "min",   "message": "must be at least 18" }
      ]
    }
  }
}
```

---

## 7. Middleware & Plugin System

### 7.1 Deux Niveaux

| Niveau | Interface | Accès | Usage |
|--------|-----------|-------|-------|
| **Middleware** | `func(c *Context, next Handler) Response` | Requête/Réponse | Transformation, auth, logging |
| **Plugin** | `Plugin interface` | Cycle de vie complet | Cache, métriques, features cross-cutting |

### 7.2 Middleware

Le middleware Core vu précédemment. Chaîné par ordre d'enregistrement, la Response remonte la chaîne.

```go
// Global
app.Use(gx.Logger(), gx.Recovery(), gx.RequestID())

// Par groupe
api := app.Group("/api", gx.Auth(verifier))

// Par route
app.GET("/admin", gx.RequireRole("admin"), handler)
```

### 7.3 Plugin Interface

```go
type Plugin interface {
    Name() string
    OnBoot(app *App) error
    OnShutdown(ctx context.Context) error
}

// Extensions optionnelles — implementées selon les besoins
type RequestPlugin interface {
    Plugin
    OnRequest(c *Context, next Handler) Response
}

type ErrorPlugin interface {
    Plugin
    OnError(c *Context, err AppError) Response
}

type RoutePlugin interface {
    Plugin
    OnRoute(route RouteInfo)   // appelé pour chaque route enregistrée au boot
}
```

Un plugin n'implémente que les interfaces dont il a besoin. Duck-typing Go.

### 7.4 Décorateurs de Route

Les plugins peuvent enrichir les routes avec des méthodes fluentes :

```go
// Cache plugin — ajoute .Cache() aux routes
users.GET("/:id", getUser).Cache(2 * time.Minute)
users.GET("",     listUsers).Cache(30 * time.Second).VaryBy("Authorization")

// Rate limit plugin — granularité par route
app.POST("/login", handler).RateLimit(gx.PerIP(5, time.Minute))

// Auth plugin
app.GET("/profile", handler).Auth()           // authentification requise
app.GET("/admin",   handler).Roles("admin")   // rôle requis
```

Ces méthodes sont ajoutées par les plugins via une `RouteBuilder` extensible — pas de modifications du Core.

### 7.5 Ordre d'Exécution

```
Boot
 └─ Plugin.OnBoot() × N (ordre d'installation)

Request
 └─ Plugin.OnRequest() × N
     └─ Middleware × M
         └─ Handler
         ↑ Response remonte
     ↑ Middleware peut modifier
 ↑ Plugin peut modifier

Error
 └─ Plugin.OnError() × N (premier qui retourne non-nil gagne)
 └─ app.OnError() (fallback)

Shutdown
 └─ Plugin.OnShutdown() × N (ordre inverse d'installation)
```

---

## 8. Observability

### 8.1 Philosophie

L'observabilité n'est pas un plugin optionnel — c'est une option de configuration de l'app. Elle est donc présente dès le démarrage, avant même que les routes soient enregistrées.

```go
app := gx.New(
    gx.WithTracing("payment-service",
        gx.OTELExporter("http://jaeger:4318"),
    ),
    gx.WithMetrics(
        gx.PrometheusExporter(":9090"),
    ),
    gx.WithStructuredLogs(
        gx.LogLevel(slog.LevelInfo),
        gx.LogFormat(gx.LogJSON),       // JSON en production
    ),
)
```

### 8.2 Tracing — OpenTelemetry

Chaque requête crée automatiquement un span racine. Le span est accessible depuis le Context.

```go
func(c *gx.Context) gx.Response {
    // Span racine créé automatiquement avec :
    // - http.method, http.route, http.status_code
    // - net.peer.ip, user_agent.original
    // - protocol (HTTP/2 ou HTTP/3)

    // Enrichir le span racine
    c.Span().SetAttr("user.id", userID)

    // Créer un span enfant
    span := c.Trace("fetch-user")
    defer span.End()

    user, err := db.FindUser(span.Context(), userID)
    span.SetAttr("db.rows", 1)

    // ...
}
```

La propagation du context (`W3C TraceContext`) est automatique pour les requêtes sortantes si les clients HTTP fournis par GX sont utilisés.

### 8.3 Metrics — Prometheus

Métriques automatiques sans configuration :

```
http_requests_total{method, route, status, protocol}    counter
http_request_duration_seconds{method, route, protocol}  histogram
http_request_size_bytes{method, route}                  histogram
http_response_size_bytes{method, route}                  histogram
http_active_requests{protocol}                           gauge
```

Métriques custom :

```go
payments := gx.Counter("payments_total", "Total payment attempts",
    gx.Label("status"),   // success | declined | error
    gx.Label("method"),   // card | bank_transfer
)

// Dans un handler
payments.With("status", "success", "method", "card").Inc()
```

### 8.4 Structured Logging

```go
// Logger corrélé au trace ID de la requête courante
c.Log().Info("user authenticated",
    "user_id", userID,
    "method",  "oauth",
)

c.Log().Warn("rate limit approaching",
    "ip",       c.ClientIP(),
    "requests", current,
    "limit",    max,
)

c.Log().Error("payment failed",
    "error",      err,
    "payment_id", paymentID,
    "amount",     amount,
)
```

Chaque log contient automatiquement `trace_id`, `span_id`, `request_id`, `route`, `method`. La corrélation log↔trace fonctionne out-of-the-box dans Grafana, Datadog, etc.

### 8.5 Automatic Instrumentation

Le framework instrumente automatiquement :
- Toutes les requêtes HTTP entrantes (span + métriques)
- Les erreurs (compteur par code d'erreur)
- Le boot et le shutdown (durée, succès/échec)
- Les health checks (résultat par dépendance)

---

## 9. App Lifecycle

### 9.1 Phases

```
INIT → BOOT → RUNNING → DRAINING → SHUTDOWN
```

| Phase | Description |
|-------|-------------|
| **INIT** | Enregistrement routes, plugins, hooks |
| **BOOT** | `OnBoot()` de tous les plugins, puis hooks utilisateur |
| **RUNNING** | Serveur accepte les requêtes |
| **DRAINING** | Signal reçu — nouvelles connexions refusées, en-cours terminées |
| **SHUTDOWN** | `OnShutdown()` inverse, ressources libérées |

### 9.2 Hooks Utilisateur

```go
app.OnBoot(func(ctx context.Context) error {
    return db.Connect(ctx)
})

app.OnBoot(func(ctx context.Context) error {
    return cache.Connect(ctx)
})

app.OnShutdown(func(ctx context.Context) error {
    return db.Close(ctx)
})
```

Les hooks `OnBoot` s'exécutent dans l'ordre de déclaration. Les hooks `OnShutdown` dans l'ordre inverse. Un échec de `OnBoot` arrête le démarrage immédiatement.

### 9.3 Health Checks

```go
// Enregistrer une dépendance à surveiller
app.Health("database", func(ctx context.Context) error {
    return db.PingContext(ctx)
})

app.Health("redis", func(ctx context.Context) error {
    return cache.Ping(ctx)
})

app.Health("external-api", func(ctx context.Context) error {
    _, err := http.Get("https://api.partner.com/health")
    return err
}, gx.HealthInterval(30*time.Second),   // fréquence du check
   gx.HealthTimeout(5*time.Second),     // timeout du check
   gx.HealthCritical(false),            // non-critique : ne fail pas /ready
)
```

**Endpoints exposés automatiquement**

```
GET /health         → état global (all checks)
GET /health/live    → liveness : l'app tourne (toujours 200 si running)
GET /health/ready   → readiness : toutes les dépendances critiques OK
```

Format de réponse :

```json
{
  "status": "degraded",
  "checks": {
    "database":     { "status": "ok",       "latency_ms": 3  },
    "redis":        { "status": "ok",       "latency_ms": 1  },
    "external-api": { "status": "degraded", "error": "timeout" }
  }
}
```

Status : `ok` | `degraded` | `down`.

### 9.4 Graceful Shutdown

```go
// Configurer la fenêtre de drain (défaut 30s)
app := gx.New(
    gx.ShutdownTimeout(30 * time.Second),
)
```

Comportement à réception de `SIGTERM` / `SIGINT` :
1. Stop d'accepter de nouvelles connexions TCP/QUIC
2. Attente de la fin des requêtes en cours (jusqu'au timeout)
3. Exécution des hooks `OnShutdown` avec le context de timeout
4. Sortie propre code 0

---

## 10. QUIC & Real-Time Primitives

### 10.1 Détection du Protocole

Transparent dans les handlers ordinaires. Accessible si besoin :

```go
c.Proto()     // "HTTP/1.1" | "HTTP/2.0" | "HTTP/3"
c.IsHTTP3()   // true si QUIC
```

### 10.2 Server Push (HTTP/2)

```go
app.GET("/dashboard", func(c *gx.Context) gx.Response {
    // Push des assets avant que le navigateur les demande
    c.Push("/static/app.js", nil)
    c.Push("/static/style.css", nil)
    return c.HTML(dashboardHTML)
})
```

No-op silencieux si le client est HTTP/3 (le push est remplacé par les Early Hints en HTTP/3) ou HTTP/1.1.

### 10.3 Server-Sent Events

```go
app.GET("/stream/prices", func(c *gx.Context) gx.Response {
    sse := c.SSE()
    defer sse.Close()

    feed := market.Subscribe(c.Param("ticker"))
    defer feed.Unsubscribe()

    for {
        select {
        case price := <-feed:
            sse.Send("price", price)
        case <-c.GoContext().Done():
            return c.NoContent()
        }
    }
})
```

### 10.4 Channels — Bidirectionnel QUIC/WebSocket

La primitive de plus haut niveau pour le temps réel. Abstrait QUIC streams en HTTP/3 et WebSocket en HTTP/1.1/2. Même code, protocole optimal selon le client.

```go
// Déclaration — comme une route normale
app.Channel("/chat/:room", ChatChannel, func(c *gx.Context, ch gx.Channel) error {
    room := gx.Typed[ChatParams](c).Room

    broker := chat.Join(room)
    defer broker.Leave()

    // Goroutine de lecture
    go func() {
        for {
            var msg ChatMessage
            if err := ch.Receive(&msg); err != nil {
                return
            }
            broker.Broadcast(msg)
        }
    }()

    // Écriture — envoi des messages du broker au client
    for {
        select {
        case msg := <-broker.Messages():
            if err := ch.Send(msg); err != nil {
                return err
            }
        case <-ch.Done():
            return nil
        }
    }
})
```

**Interface Channel**

```go
type Channel interface {
    Send(v any) error           // sérialise et envoie (JSON par défaut)
    Receive(v any) error        // désérialise le prochain message
    SendRaw(b []byte) error     // bytes bruts
    ReceiveRaw() ([]byte, error)
    Done() <-chan struct{}       // fermé quand le client se déconnecte
    Proto() string              // protocole utilisé réellement
    Close() error
}
```

### 10.5 0-RTT (QUIC Early Data)

```go
// Marquer une route comme safe pour le 0-RTT (requêtes idempotentes)
app.GET("/catalog", listProducts).Allow0RTT()

// Les routes POST/PATCH/DELETE ne peuvent pas bénéficier de 0-RTT
// (violation de sécurité — replay attacks)
// Le framework refuse la compilation si Allow0RTT() est appelé sur une route non-idempotente
```

### 10.6 Migration de Connexion

```go
// Hook appelé quand un client QUIC migre (wifi → 4G, changement d'IP)
app.OnMigrate(func(event gx.MigrateEvent) {
    log.Info("client migrated",
        "old_addr", event.OldAddr,
        "new_addr", event.NewAddr,
        "session",  event.SessionID,
    )
})
```

La migration est transparente pour les handlers et les Channels actifs.

---

## 11. Configuration

### 11.1 Options au Boot

```go
app := gx.New(
    // Protocoles
    gx.WithHTTP2(),                          // défaut activé
    gx.WithHTTP3(),                          // défaut activé si TLS présent
    gx.WithDevTLS("localhost"),              // self-signed pour dev

    // Timeouts
    gx.ReadTimeout(15 * time.Second),
    gx.WriteTimeout(15 * time.Second),
    gx.IdleTimeout(60 * time.Second),
    gx.ShutdownTimeout(30 * time.Second),

    // Observabilité
    gx.WithTracing("service-name"),
    gx.WithMetrics(),
    gx.WithStructuredLogs(),

    // Comportement
    gx.TrustedProxies("10.0.0.0/8", "172.16.0.0/12"),
    gx.MaxBodySize(10 << 20),               // 10 MB
    gx.Environment(gx.Production),          // désactive /docs, active optimisations
)
```

### 11.2 Environments

```go
const (
    Development Environment = "development"
    Staging     Environment = "staging"
    Production  Environment = "production"
)
```

Comportements différents par environment :

| Feature | Development | Production |
|---------|-------------|------------|
| `/docs` OpenAPI | ✓ activé | ✗ désactivé |
| Stack trace dans les erreurs | ✓ | ✗ |
| Pretty-print logs | ✓ | ✗ (JSON) |
| Self-signed TLS | ✓ autorisé | ✗ refusé |
| Panic → 500 silencieux | ✗ | ✓ |
| 0-RTT | ✗ désactivé | ✓ |

---

## 12. Conventions & DX

### 12.1 Nommage

```
Packages    lowercase, court, sans underscore        gx, core, gxtest
Types       PascalCase                               AppError, Contract, Channel
Variables   camelCase                                userID, maxRetries
Erreurs     ErrXxx (variables), préfixe Err          ErrNotFound, ErrEmailTaken
Constantes  PascalCase ou SCREAMING_SNAKE            Production, MAX_BODY_SIZE
```

### 12.2 Déclaration des Erreurs

Par convention, les erreurs d'un domaine sont déclarées dans `errors.go` à la racine du package.

```go
// users/errors.go
package users

import "github.com/example/myapp/gx"

var (
    ErrNotFound        = gx.E(404, "USER_NOT_FOUND",    "User does not exist")
    ErrEmailTaken      = gx.E(409, "USER_EMAIL_TAKEN",  "Email already in use")
    ErrInvalidPassword = gx.E(422, "INVALID_PASSWORD",  "Password requirements not met")
)
```

### 12.3 Organisation d'un Handler

Ordre recommandé à l'intérieur d'un handler :

```go
func createUser(c *gx.Context) gx.Response {
    // 1. Extraire les inputs (déjà validés par le Contract)
    req := gx.Typed[CreateUserRequest](c)
    claims := gx.Typed[AuthClaims](c)

    // 2. Instrumenter
    span := c.Span()
    span.SetAttr("user.email", req.Email)

    // 3. Logique métier
    user, err := users.Create(c.GoContext(), req)
    if err != nil {
        return c.Fail(ErrEmailTaken).Wrap(err)
    }

    // 4. Réponse
    return c.JSON(UserResponse{...}).Status(201)
}
```

### 12.4 Testing

```go
// gxtest — package de test fourni par le framework
import "github.com/example/gx/gxtest"

func TestGetUser(t *testing.T) {
    app := gx.New()
    app.GET("/users/:id", GetUser, getUser)

    res := gxtest.GET(app, "/users/123").
        Header("Authorization", "Bearer "+testToken).
        Do(t)

    res.AssertStatus(200)
    res.AssertJSON(t, &UserResponse{}, func(u *UserResponse) {
        assert.Equal(t, "123", u.ID)
    })
}

// Test d'un middleware
func TestAuthMiddleware(t *testing.T) {
    res := gxtest.GET(app, "/protected").Do(t)
    res.AssertStatus(401)
    res.AssertErrorCode(t, "UNAUTHORIZED")
}
```

Le package `gxtest` n'utilise pas `httptest.Server` — il appelle directement `ServeHTTP` pour des tests unitaires rapides.

---

## 13. Standard Plugins

Plugins fournis dans `gx/plugins/` — optionnels, aucun n'est activé par défaut.

### gx/plugins/cors
```go
app.Install(cors.New(cors.Config{
    Origins:     []string{"https://app.example.com"},
    Methods:     []string{"GET", "POST", "PUT", "DELETE"},
    Headers:     []string{"Authorization", "Content-Type"},
    Credentials: true,
    MaxAge:      12 * time.Hour,
}))
```

### gx/plugins/ratelimit
```go
app.Install(ratelimit.New(
    ratelimit.PerIP(100, time.Minute),
    ratelimit.Backend(redis),          // défaut : in-memory
    ratelimit.OnExceeded(func(c *gx.Context) gx.Response {
        return c.Fail(ErrRateLimitExceeded)
    }),
))

// Granularité par route
app.POST("/login", handler).RateLimit(ratelimit.PerIP(5, time.Minute))
```

### gx/plugins/cache
```go
app.Install(cache.New(
    cache.Backend(redis),
    cache.DefaultTTL(5 * time.Minute),
    cache.KeyFunc(func(c *gx.Context) string {
        return c.Method() + c.Path() + c.Query("page")
    }),
))

// Par route
users.GET("/:id", getUser).Cache(2 * time.Minute)
users.POST("", createUser).Invalidates("/api/v1/users/*")
```

### gx/plugins/auth
```go
app.Install(auth.JWT(
    auth.Secret(os.Getenv("JWT_SECRET")),
    auth.ClaimsType[MyClaims](),
    auth.OnInvalid(func(c *gx.Context) gx.Response {
        return c.Fail(ErrUnauthorized)
    }),
))

// Dans un handler
claims := gx.Typed[MyClaims](c)
```

### gx/plugins/openapi
```go
app.Install(openapi.New(
    openapi.Title("My API"),
    openapi.Version("1.0.0"),
    openapi.UI(openapi.Scalar),   // Scalar | Swagger | Redoc
    openapi.DocsPath("/docs"),
))
```

### gx/plugins/compress
```go
app.Install(compress.New(
    compress.Algorithms(compress.Brotli, compress.Gzip),
    compress.MinSize(1024),        // ne pas compresser < 1KB
))
```

---

## 14. Package Structure

```
github.com/example/gx/
│
├── core/                   # Couche transport (utilisable standalone)
│   ├── app.go              # App, Listen, ListenDual
│   ├── router.go           # Radix-tree, params, wildcards
│   ├── context.go          # Context (poolé), lecture requête
│   ├── response.go         # Response interface + builders JSON/Text/File/...
│   ├── middleware.go       # type Middleware, chain execution
│   ├── group.go            # Group, préfixes, middleware local
│   ├── sse.go              # Server-Sent Events
│   └── tls.go              # WithTLS, WithDevTLS (self-signed)
│
├── gx.go                   # Point d'entrée — gx.New(), options
├── contract.go             # Contract, SchemaRef, gx.Schema[T]()
├── errors.go               # AppError, gx.E(), c.Fail()
├── typed.go                # gx.Typed[T](), gx.TryTyped[T]()
├── plugin.go               # Plugin interface, RequestPlugin, ErrorPlugin
├── lifecycle.go            # OnBoot, OnShutdown, Health
├── observability.go        # WithTracing, WithMetrics, WithStructuredLogs
├── channel.go              # Channel interface, app.Channel()
│
├── plugins/                # Plugins standard (tous optionnels)
│   ├── cors/
│   ├── ratelimit/
│   ├── cache/
│   ├── auth/
│   ├── openapi/
│   └── compress/
│
└── gxtest/                 # Utilitaires de test
    ├── request.go          # gxtest.GET(), POST(), etc.
    └── assert.go           # AssertStatus(), AssertJSON(), AssertErrorCode()
```

---

## 15. Design Decisions Log

Journal des décisions importantes et des alternatives rejetées.

---

### DDL-001 — Handler retourne Response, pas error

**Décision** : `func(*Context) Response`  
**Alternatives considérées** : `func(*Context) error` (Echo), `func(*Context) (Response, error)`, `func(*Context)` (Gin)  
**Raison** : Une erreur est une Response. Unifier les deux chemins élimine les patterns incohérents. Les tests unitaires deviennent triviaux.

---

### DDL-002 — La Response remonte la chaîne middleware

**Décision** : `func(c *Context, next Handler) Response`  
**Alternatives considérées** : `next()` callback comme Express, `defer` pattern  
**Raison** : Permet aux middleware d'observer et modifier la réponse après exécution du handler. Impossible avec le modèle callback d'Express.

---

### DDL-003 — Contracts comme valeurs Go, pas annotations

**Décision** : `var MyContract = gx.Contract{...}`  
**Alternatives considérées** : tags de struct, annotations `//go:generate`, AST parsing  
**Raison** : Les valeurs Go sont testables, composables, refactorisables par l'IDE. Pas de toolchain externe.

---

### DDL-004 — Erreurs déclarées comme variables de package

**Décision** : `var ErrNotFound = gx.E(404, "NOT_FOUND", "...")`  
**Alternatives considérées** : types d'erreur custom (struct), erreurs inline dans les handlers  
**Raison** : Comparaison par valeur dans les tests, listage statique possible, documentation automatique dans OpenAPI.

---

### DDL-005 — Channel abstrait QUIC streams et WebSocket

**Décision** : `app.Channel("/path", contract, handler)` — protocole transparent  
**Alternatives considérées** : routes séparées pour WS et QUIC, exposer l'API quic-go directement  
**Raison** : Le développeur déclare un comportement temps-réel, pas un protocole. Le framework choisit le meilleur transport disponible.

---

### DDL-006 — Observabilité en option de `gx.New()`, pas en plugin

**Décision** : `gx.New(gx.WithTracing(...), gx.WithMetrics())`  
**Alternatives considérées** : plugin installable comme les autres  
**Raison** : L'observabilité doit être active avant le boot, avant même l'enregistrement des routes. Un plugin s'installe trop tard pour instrumenter le cycle de vie complet.

---

*Document vivant — mis à jour à chaque décision de conception validée.*