# GX Core — Router Autonome & Mount

> **Décision** CDL-009  
> **Complète** `Group` — non-remplacé  
> **Statut** Validé

---

## Principe

Un `Router` est une valeur autonome, créée sans référence à l'app. Il peut être instancié dans n'importe quel package, configuré indépendamment, puis attaché à l'app via `Mount`.

```go
// Créer un router indépendant
r := gx.NewRouter()

// L'attacher à un préfixe
app.Mount("/api/v1/users", r)
```

---

## Motivation — Organisation modulaire

Avec uniquement `Group`, chaque domaine doit connaître l'app pour enregistrer ses routes. Ça crée un couplage fort et rend l'organisation en packages difficile.

```go
// Avec Group — couplage fort à l'app
func RegisterUserRoutes(app *gx.App) {
    g := app.Group("/api/v1/users", requireAuth)
    g.GET("", listUsers)
    // ...
}

// Avec Router + Mount — autonome, découplé
func Router() *gx.Router {
    r := gx.NewRouter()
    r.Use(requireAuth)
    r.GET("", listUsers)
    return r
}
```

Le second pattern permet à chaque domaine de **s'ignorer mutuellement**. `main.go` est le seul endroit qui connaît tout le monde.

---

## API

### gx.NewRouter()

```go
r := gx.NewRouter()

// Même API que Group
r.Use(mw ...Middleware)
r.GET(path string, handlers ...Handler)
r.POST(path string, handlers ...Handler)
r.PUT(path string, handlers ...Handler)
r.PATCH(path string, handlers ...Handler)
r.DELETE(path string, handlers ...Handler)
r.OPTIONS(path string, handlers ...Handler)

// Imbrication — un Router peut monter d'autres Routers
r.Mount(prefix string, sub *Router, mw ...Middleware)
```

### app.Mount()

```go
// Monter un Router sur un préfixe
app.Mount(prefix string, r *Router, mw ...Middleware)

// Le middleware passé à Mount s'applique uniquement à ce Router
app.Mount("/api/v1/admin", admin.Router(), requireAdmin)
```

Le middleware de `Mount` s'exécute **avant** le middleware interne du Router. L'ordre est :

```
global app middleware
    └─ Mount middleware       ← injecté à l'attachement
        └─ Router middleware  ← défini dans le Router
            └─ handler
```

---

## Exemple complet

### Structure du projet

```
myapp/
├── main.go
├── users/
│   ├── handlers.go
│   ├── errors.go
│   └── router.go       ← Router autonome
├── posts/
│   ├── handlers.go
│   └── router.go
└── admin/
    ├── handlers.go
    └── router.go
```

### users/router.go

```go
package users

import "github.com/example/gx"

// Router retourne le router du domaine users.
// Aucune référence à l'app — totalement autonome.
func Router() *gx.Router {
    r := gx.NewRouter()

    r.GET("",        listUsers)
    r.GET("/:id",    getUser)
    r.POST("",       createUser)
    r.PATCH("/:id",  updateUser)
    r.DELETE("/:id", deleteUser)

    // Sous-ressource — montée dans le Router
    r.Mount("/:userId/posts", postsByUser())

    return r
}

// Router interne non-exporté — détail d'implémentation
func postsByUser() *gx.Router {
    r := gx.NewRouter()
    r.GET("",     listUserPosts)
    r.GET("/:id", getUserPost)
    return r
}
```

### admin/router.go

```go
package admin

import "github.com/example/gx"

func Router() *gx.Router {
    r := gx.NewRouter()

    r.GET("/stats",    getStats)
    r.GET("/users",    listAllUsers)
    r.DELETE("/users/:id", forceDeleteUser)

    return r
}
```

### main.go

```go
package main

import (
    "github.com/example/gx"
    "myapp/users"
    "myapp/posts"
    "myapp/admin"
)

func main() {
    app := gx.New()

    app.Use(gx.Logger(), gx.Recovery(), gx.RequestID())

    // Routes publiques
    app.GET("/health", healthHandler)

    // Montage des domaines — main.go est le seul à connaître tout le monde
    app.Mount("/api/v1/users",  users.Router(), requireAuth)
    app.Mount("/api/v1/posts",  posts.Router(), requireAuth)
    app.Mount("/api/v1/admin",  admin.Router(), requireAuth, requireAdmin)

    app.ListenH3(":8443")
}
```

---

## Group vs Router — Quand utiliser quoi

| | `Group` | `Router` |
|---|---|---|
| **Création** | `app.Group(prefix)` — lié à l'app | `gx.NewRouter()` — autonome |
| **Usage** | Structurer dans un même fichier ou package | Organiser entre packages distincts |
| **Connaissance de l'app** | Requise | Non requise |
| **Testabilité** | Test via l'app complète | Test du Router isolément |
| **Partage** | Non — couplé à l'instance | Oui — valeur passable, retournable |

Les deux coexistent. `Group` reste utile pour regrouper des routes sans sortir du package courant. `Router` + `Mount` est le pattern pour l'organisation modulaire inter-packages.

---

## Testabilité du Router isolé

Un `Router` autonome peut être testé sans instancier une app complète.

```go
// users/router_test.go
func TestUsersRouter(t *testing.T) {
    r := Router()

    // gxtest accepte un Router directement
    res := gxtest.GET(r, "/").
        Header("Authorization", "Bearer "+testToken).
        Do(t)

    res.AssertStatus(200)
}

func TestGetUser(t *testing.T) {
    res := gxtest.GET(Router(), "/123").Do(t)
    res.AssertStatus(200)
    res.AssertJSON(t, &UserResponse{}, func(u *UserResponse) {
        assert.Equal(t, "123", u.ID)
    })
}
```

---

## Résolution des chemins

`Mount` concatène le préfixe d'attachement avec les chemins déclarés dans le Router.

```
app.Mount("/api/v1/users", users.Router())

Router déclare    →    Route finale
""                →    /api/v1/users
"/:id"            →    /api/v1/users/:id
"/:userId/posts"  →    /api/v1/users/:userId/posts
```

Les paramètres du préfixe de `Mount` sont accessibles depuis les handlers via `c.Param()` comme tout autre paramètre de path.

```go
// Route finale : /api/v1/users/:userId/posts/:id
func getUserPost(c *gx.Context) gx.Response {
    userID := c.Param("userId")   // du préfixe Mount
    postID := c.Param("id")       // du Router interne
    // ...
}
```

---

## Résumé

| Avant | Après |
|-------|-------|
| `Group` uniquement — couplé à l'app | `Group` + `Router` autonome |
| Routes enregistrées depuis main via callbacks | Domaines retournent leur Router |
| Test nécessite l'app complète | Router testable isolément |
| `main.go` délègue aux packages | `main.go` monte des valeurs |