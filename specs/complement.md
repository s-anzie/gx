# GX Framework — Compléments de Conception

> **Version** 0.2.0-concept  
> **Statut** Design Phase  
> **Complète** GX_FRAMEWORK_DESIGN.md · GX_CORE_DESIGN.md

---

## Table of Contents

1. [Cookies](#1-cookies)
2. [Upload de Fichiers](#2-upload-de-fichiers)
3. [Static File Serving](#3-static-file-serving)
4. [HTTPS Redirect](#4-https-redirect)
5. [Content Negotiation](#5-content-negotiation)
6. [gxtest — Package de Test](#6-gxtest--package-de-test)
7. [Validation Standalone](#7-validation-standalone)
8. [i18n des Erreurs](#8-i18n-des-erreurs)
9. [Configuration depuis l'Environnement](#9-configuration-depuis-lenvironnement)
10. [Plugin Ordering](#10-plugin-ordering)
11. [Early Hints — HTTP/3](#11-early-hints--http3)
12. [Panic dans les Goroutines](#12-panic-dans-les-goroutines)
13. [CLI Tooling](#13-cli-tooling)
14. [Décisions Log](#14-décisions-log)

---

## 1. Cookies

### Principe

Les cookies sont aussi fondamentaux que les headers. Ils méritent des méthodes de première classe sur le `Context`, pas un accès brut via `c.Request.Cookie()`.

### API — Lecture

```go
// Lire un cookie
session, err := c.Cookie("session")   // (string, error) — error si absent

// Lire avec fallback
token := c.CookieDefault("token", "")

// Lire le cookie brut — accès à tous les champs
cookie, err := c.RawCookie("session")  // (*http.Cookie, error)
```

### API — Écriture

```go
// Écriture simple
c.SetCookie("session", sessionID)

// Écriture avec options complètes — retourne une Response chainable
return c.JSON(user).
    Cookie(gx.Cookie{
        Name:     "session",
        Value:    sessionID,
        MaxAge:   int(7 * 24 * time.Hour / time.Second),
        Path:     "/",
        HttpOnly: true,
        Secure:   true,
        SameSite: http.SameSiteLaxMode,
    })

// Supprimer un cookie
return c.NoContent().ClearCookie("session")
```

### Type gx.Cookie

```go
type Cookie struct {
    Name     string
    Value    string
    Path     string           // défaut "/"
    Domain   string
    MaxAge   int              // secondes — 0 = session cookie
    Expires  time.Time
    Secure   bool             // défaut true si HTTPS
    HttpOnly bool             // défaut true
    SameSite http.SameSite    // défaut SameSiteLaxMode
}
```

### Valeurs par défaut sécurisées

Le framework applique des défauts sécurisés automatiquement. Pas besoin de penser à `HttpOnly` et `Secure` — ils sont actifs par défaut, et il faut les désactiver explicitement si besoin.

```go
// Équivalent à HttpOnly: true, Secure: true, SameSite: Lax
c.SetCookie("session", id)

// Désactiver explicitement pour un cas particulier
return c.JSON(data).Cookie(gx.Cookie{
    Name:     "tracking",
    Value:    id,
    HttpOnly: false,   // accessible en JS — intentionnel
    Secure:   true,
})
```

### Cookie signing (GX layer)

La signature de cookies est une responsabilité de la couche GX, pas du Core. Le plugin `auth` expose un helper :

```go
// Signer avant d'écrire
signed := gx.SignCookie("session", sessionID, secret)
return c.JSON(user).Cookie(signed)

// Vérifier à la lecture
value, err := gx.VerifyCookie(c, "session", secret)
```

---

## 2. Upload de Fichiers

### Principe

L'upload est un cas d'usage fréquent avec ses propres contraintes : taille maximale, types acceptés, stockage temporaire. Le Core fournit les primitives d'accès, la configuration de sécurité est au niveau de l'app.

### Configuration

```go
app := gx.New(
    gx.MaxBodySize(10 << 20),      // 10 MB — s'applique à tout
    gx.MaxFileSize(5 << 20),       // 5 MB par fichier
    gx.MaxMemoryUpload(32 << 20),  // 32 MB en mémoire avant spill sur disque
)
```

### API — Fichier unique

```go
func uploadAvatar(c *gx.Context) gx.Response {
    file, err := c.FormFile("avatar")
    if err != nil {
        return c.Fail(ErrNoFile)
    }

    // Métadonnées
    file.Filename    // string — nom original
    file.Size        // int64 — taille en bytes
    file.ContentType // string — MIME type détecté
    file.Header      // textproto.MIMEHeader

    // Lire le contenu
    src, err := file.Open()
    if err != nil {
        return c.Fail(ErrFileOpen)
    }
    defer src.Close()

    // Sauvegarder sur disque
    if err := file.Save("/uploads/"+file.Filename); err != nil {
        return c.Fail(ErrFileSave)
    }

    return c.Created(map[string]string{"path": "/uploads/" + file.Filename})
}
```

### API — Fichiers multiples

```go
func uploadAttachments(c *gx.Context) gx.Response {
    files, err := c.FormFiles("attachments")
    if err != nil {
        return c.Fail(ErrNoFiles)
    }

    paths := make([]string, 0, len(files))
    for _, file := range files {
        dest := "/uploads/" + file.Filename
        if err := file.Save(dest); err != nil {
            return c.Fail(ErrFileSave).With("file", file.Filename)
        }
        paths = append(paths, dest)
    }

    return c.Created(map[string][]string{"paths": paths})
}
```

### Type gx.FileHeader

```go
type FileHeader struct {
    Filename    string
    Size        int64
    ContentType string
    Header      textproto.MIMEHeader

    Open() (multipart.File, error)
    Save(dst string) error         // helper — copie vers dst
    Bytes() ([]byte, error)        // charge tout en mémoire
}
```

### Validation du type MIME

La validation du type de fichier est un cas d'usage commun. Elle se fait par Contract dans la couche GX, ou manuellement dans le handler :

```go
allowed := map[string]bool{
    "image/jpeg": true,
    "image/png":  true,
    "image/webp": true,
}

file, _ := c.FormFile("avatar")
if !allowed[file.ContentType] {
    return c.Fail(ErrInvalidFileType).
        With("got", file.ContentType).
        With("allowed", []string{"image/jpeg", "image/png", "image/webp"})
}
```

**Note** : le `ContentType` est détecté par lecture des magic bytes du fichier, pas par l'extension ou le header MIME fourni par le client — qui peut être falsifié.

---

## 3. Static File Serving

### Principe

Servir des fichiers statiques est une opération courante en développement. En production on préfère un CDN ou Nginx en amont, mais le framework doit pouvoir le faire sans dépendance externe.

### API

```go
// Répertoire complet
app.Static("/assets", "./public")
// GET /assets/style.css  → ./public/style.css
// GET /assets/js/app.js  → ./public/js/app.js

// Fichier unique
app.StaticFile("/favicon.ico", "./public/favicon.ico")
app.StaticFile("/robots.txt",  "./public/robots.txt")

// Embed — fichiers embarqués dans le binaire (go:embed)
//go:embed public/*
var staticFS embed.FS
app.StaticFS("/assets", staticFS)
```

### Comportement

- **Index** : `GET /assets/` sert `./public/index.html` si présent. Sinon `404`, jamais de directory listing.
- **Compression** : si le fichier `style.css.br` ou `style.css.gz` existe à côté de `style.css`, il est servi automatiquement si le client annonce `Accept-Encoding: br` ou `gzip`.
- **Cache-Control** : header `ETag` + `Last-Modified` générés automatiquement. Réponse `304 Not Modified` si le client a la version à jour.
- **Security** : path traversal bloqué (`../` nettoyé). Impossible de sortir du répertoire racine.
- **Content-Type** : détecté par extension.

### Configuration

```go
app.Static("/assets", "./public",
    gx.StaticMaxAge(7 * 24 * time.Hour),   // Cache-Control: max-age=604800
    gx.StaticIndex("index.html"),          // fichier index — défaut "index.html"
    gx.StaticBrowse(false),                // directory listing — défaut false
)
```

### Embed — Recommandé pour la production

Embarquer les fichiers dans le binaire élimine la dépendance au système de fichiers au runtime :

```go
//go:embed public
var publicFS embed.FS

func main() {
    app := gx.New()
    app.StaticFS("/", publicFS)
    app.ListenH3(":8443")
}
```

Le binaire est autosuffisant — aucun fichier externe nécessaire au déploiement.

---

## 4. HTTPS Redirect

### Principe

Middleware qui écoute sur le port HTTP non-sécurisé et redirige toutes les requêtes vers HTTPS. Indispensable en production mais souvent oublié.

### API

```go
// Middleware — à utiliser si l'app gère elle-même les deux ports
app.Use(gx.HTTPSRedirect())

// Serveur dédié — démarre un listener HTTP sur :80 qui redirige vers :443
// Recommandé — ne pas polluer l'app principale
app.ListenHTTPRedirect(":80", ":443")
```

### Comportement du middleware

```go
// Requête entrante
GET http://example.com/users/123

// Réponse
308 Permanent Redirect
Location: https://example.com/users/123
```

`308 Permanent Redirect` plutôt que `301` : conserve la méthode HTTP originale. Un `POST` redirigé reste un `POST`, pas un `GET`. C'est la sémantique correcte.

### ListenHTTPRedirect — Implémentation interne

```go
func (a *App) ListenHTTPRedirect(httpAddr, httpsAddr string) {
    // Extrait le port HTTPS pour construire la Location
    _, httpsPort, _ := net.SplitHostPort(httpsAddr)

    srv := &http.Server{
        Addr: httpAddr,
        Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            host := r.Host
            if h, _, err := net.SplitHostPort(host); err == nil {
                host = h
            }
            target := "https://" + net.JoinHostPort(host, httpsPort) + r.RequestURI
            http.Redirect(w, r, target, http.StatusPermanentRedirect)
        }),
    }

    go srv.ListenAndServe()
}
```

### Usage typique en production

```go
func main() {
    app := gx.New()
    app.WithTLS("cert.pem", "key.pem")

    // Redirige :80 → :443 dans une goroutine interne
    app.ListenHTTPRedirect(":80", ":443")

    // Serveur principal
    app.ListenH3(":443")
}
```

### HSTS

`ListenHTTPRedirect` et le middleware `HTTPSRedirect` injectent automatiquement le header HSTS sur les réponses HTTPS :

```
Strict-Transport-Security: max-age=63072000; includeSubDomains; preload
```

Cela indique au navigateur de toujours utiliser HTTPS pour ce domaine, même sans la redirection.

---

## 5. Content Negotiation

### Principe

Le client annonce le format qu'il accepte via `Accept: application/json` ou `Accept: application/xml`. Le serveur devrait honorer cette préférence. Actuellement le framework retourne toujours du JSON.

### API — Négociation automatique

```go
func getUser(c *gx.Context) gx.Response {
    user := User{ID: "1", Name: "Alice"}

    // Retourne JSON ou XML selon le header Accept du client
    return c.Negotiate(user)
}
```

`Negotiate` évalue `Accept` dans l'ordre de priorité (qualité `q=`), choisit le format supporté le plus approprié, et retourne la bonne réponse. Si aucun format ne correspond : `406 Not Acceptable`.

### Formats supportés par défaut

```
application/json        → c.JSON()
application/xml         → c.XML()
text/plain              → c.Text() — représentation string de v
text/html               → c.HTML() — si v implémente HTMLRenderer
```

### Étendre les formats

```go
app := gx.New(
    gx.WithNegotiator(
        "application/msgpack", func(w http.ResponseWriter, v any) error {
            w.Header().Set("Content-Type", "application/msgpack")
            return msgpack.NewEncoder(w).Encode(v)
        },
    ),
)
```

### Négociation manuelle

Si `Negotiate` est trop magique, le développeur peut inspecter et décider :

```go
func getUser(c *gx.Context) gx.Response {
    user := findUser(c.Param("id"))

    switch c.Accepts("application/json", "application/xml") {
    case "application/xml":
        return c.XML(user)
    default:
        return c.JSON(user)
    }
}
```

`c.Accepts(candidates...)` retourne le format préféré parmi les candidats fournis, en respectant les poids `q=` du header `Accept`. Retourne `""` si aucun ne correspond.

### Convention — JSON par défaut

Si le client n'envoie pas de header `Accept`, ou envoie `Accept: */*`, le framework retourne `application/json`. C'est le comportement le plus raisonnable pour une API REST.

---

## 6. gxtest — Package de Test

### Principe

`gxtest` est un package fourni par le framework pour tester les handlers et les routers sans démarrer de serveur HTTP. Il appelle directement `ServeHTTP` et expose une API fluent pour construire les requêtes et asserter les réponses.

### Positionnement

```
gxtest ne remplace pas httptest — il le complète.
- gxtest : tests unitaires de handlers et routers, rapides, sans réseau
- httptest : tests d'intégration avec un serveur réel, pour tester TLS, HTTP/2, etc.
```

### Construire une requête — Request Builder

```go
// Verbes HTTP
gxtest.GET(target, path)
gxtest.POST(target, path)
gxtest.PUT(target, path)
gxtest.PATCH(target, path)
gxtest.DELETE(target, path)

// target peut être *gx.App, *gx.Router, ou gx.Handler
```

Le builder est fluent :

```go
res := gxtest.POST(app, "/api/v1/users").
    Header("Authorization", "Bearer "+testToken).
    Header("Content-Type", "application/json").
    JSON(CreateUserRequest{Name: "Alice", Email: "alice@example.com"}).
    Do(t)
```

### Méthodes du Request Builder

```go
.Header(key, value string)         // ajouter un header
.JSON(v any)                       // body JSON — set Content-Type automatiquement
.XML(v any)                        // body XML
.Body(r io.Reader, contentType)    // body brut
.Cookie(name, value string)        // ajouter un cookie
.Query(key, value string)          // ajouter un query param
.Param(key, value string)          // simuler un path param (pour tests unitaires)
.Do(t *testing.T) *TestResponse    // exécuter la requête
```

### Assertions — TestResponse

```go
res.AssertStatus(t, 201)

res.AssertHeader(t, "Content-Type", "application/json; charset=utf-8")
res.AssertHeaderContains(t, "Location", "/users/")

res.AssertBodyContains(t, "alice@example.com")

// JSON — unmarshal + callback d'assertion
res.AssertJSON(t, &UserResponse{}, func(u *UserResponse) {
    assert.Equal(t, "Alice", u.Name)
    assert.NotEmpty(t, u.ID)
})

// Erreur structurée GX
res.AssertError(t, "USER_NOT_FOUND")
res.AssertErrorStatus(t, 404)

// Cookie
res.AssertCookie(t, "session")
res.AssertNoCookie(t, "session")
```

### Exemples

```go
// Test d'un handler seul
func TestCreateUser(t *testing.T) {
    res := gxtest.POST(createUser, "/").
        JSON(CreateUserRequest{Name: "Alice", Email: "alice@test.com"}).
        Do(t)

    res.AssertStatus(t, 201)
    res.AssertJSON(t, &UserResponse{}, func(u *UserResponse) {
        assert.Equal(t, "Alice", u.Name)
        assert.Equal(t, "alice@test.com", u.Email)
    })
}

// Test d'un Router isolé
func TestUsersRouter(t *testing.T) {
    r := users.Router()

    t.Run("list", func(t *testing.T) {
        res := gxtest.GET(r, "").
            Header("Authorization", "Bearer "+testToken).
            Do(t)
        res.AssertStatus(t, 200)
    })

    t.Run("not found", func(t *testing.T) {
        res := gxtest.GET(r, "/unknown-id").
            Header("Authorization", "Bearer "+testToken).
            Do(t)
        res.AssertStatus(t, 404)
        res.AssertError(t, "USER_NOT_FOUND")
    })
}

// Test de l'app complète
func TestApp(t *testing.T) {
    app := setupTestApp()

    res := gxtest.GET(app, "/health").Do(t)
    res.AssertStatus(t, 200)
}
```

### MockContext — Test unitaire pur d'un handler

Pour tester un handler sans aucun routing :

```go
func TestGetUserHandler(t *testing.T) {
    ctx := gxtest.NewContext(
        gxtest.WithParam("id", "123"),
        gxtest.WithHeader("Authorization", "Bearer "+testToken),
    )

    res := getUser(ctx)

    assert.Equal(t, 200, res.Status())
}
```

### Fixtures

```go
// gxtest.Fixture — charger des fichiers JSON de test
req := gxtest.POST(app, "/users").
    Fixture("testdata/create_user.json").
    Do(t)

// gxtest.Golden — comparer la réponse à un fichier de référence
res.AssertGolden(t, "testdata/create_user_response.json")
// Si le fichier n'existe pas, il est créé. Commit le fichier en gold.
// -update flag pour mettre à jour les goldens
```

---

## 7. Validation Standalone

### Principe

La validation est mentionnée dans les Contracts mais n'existe pas indépendamment. Il faut un `gx.Validate()` utilisable seul — dans un handler sans Contract, dans un service, dans un test.

### API

```go
// Valider une struct
if err := gx.Validate(req); err != nil {
    return c.Fail(gx.ErrValidation).WithValidation(err)
}

// Valider et retourner directement
if res, ok := gx.ValidateOrFail(c, req); !ok {
    return res   // 422 avec détail des champs invalides
}
```

### Tags de validation

Compatibles avec `go-playground/validator` — pas de tags propriétaires.

```go
type CreateUserRequest struct {
    Name     string `json:"name"     validate:"required,min=2,max=100"`
    Email    string `json:"email"    validate:"required,email"`
    Age      int    `json:"age"      validate:"omitempty,min=18,max=120"`
    Role     string `json:"role"     validate:"oneof=user admin moderator"`
    Website  string `json:"website"  validate:"omitempty,url"`
    Password string `json:"password" validate:"required,min=8"`
}
```

Tags additionnels définis par GX :

```
default:"value"     valeur par défaut si champ absent
example:"value"     utilisé dans la génération OpenAPI
description:"..."   idem
trim                trim whitespace avant validation
```

### Format d'erreur de validation

Cohérent avec le format `AppError` défini dans l'Error Taxonomy :

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Request validation failed",
    "details": {
      "fields": [
        {
          "field": "email",
          "rule": "email",
          "value": "not-an-email",
          "message": "must be a valid email address"
        },
        {
          "field": "age",
          "rule": "min",
          "value": 16,
          "message": "must be at least 18"
        }
      ]
    }
  }
}
```

### Règles custom

```go
// Enregistrer une règle custom — au boot de l'app
gx.RegisterValidator("slug", func(fl validator.FieldLevel) bool {
    return slugRegex.MatchString(fl.Field().String())
})

// Utiliser
type Post struct {
    Slug string `json:"slug" validate:"required,slug"`
}
```

### Validation de types non-struct

```go
// Valider une valeur simple
if err := gx.ValidateVar(email, "required,email"); err != nil {
    return c.Fail(ErrInvalidEmail)
}

// Valider une slice
if err := gx.ValidateVar(tags, "min=1,max=5,dive,required,min=2"); err != nil {
    return c.Fail(ErrInvalidTags)
}
```

---

## 8. i18n des Erreurs

### Principe

Les `AppError` ont un `Message` string fixe. En production, on veut des messages dans la langue du client. La solution doit rester simple — pas un système i18n complet, juste ce qu'il faut pour les messages d'erreur.

### Approche

On n'introduit pas de clé de traduction ou de système de catalogue. On garde la déclaration d'erreur lisible, et on y ajoute un map de traductions optionnel.

```go
var ErrUserNotFound = gx.E(404, "USER_NOT_FOUND", "User does not exist",
    gx.Translations{
        "fr": "Cet utilisateur n'existe pas",
        "es": "Este usuario no existe",
        "de": "Dieser Benutzer existiert nicht",
    },
)
```

La traduction anglaise reste la valeur par défaut — c'est le fallback si la langue du client n'est pas dans le map.

### Détection de la langue

```go
// Dans le OnError global
app.OnError(func(c *gx.Context, err gx.AppError) gx.Response {
    lang := c.AcceptLanguage()   // parse Accept-Language, retourne "fr", "en", etc.
    msg  := err.MessageFor(lang) // retourne la traduction, fallback anglais

    return c.JSON(map[string]any{
        "error": map[string]any{
            "code":    err.Code,
            "message": msg,
        },
    }).Status(err.Status)
})
```

### c.AcceptLanguage()

```go
// Header : Accept-Language: fr-FR, fr;q=0.9, en;q=0.8
c.AcceptLanguage()          // "fr" — langue primaire, tag normalisé
c.AcceptLanguages()         // ["fr", "en"] — toutes, par ordre de préférence
```

### Validation messages

Les messages de validation sont également traduisibles :

```go
gx.RegisterTranslations("fr", map[string]string{
    "required": "Ce champ est obligatoire",
    "email":    "Doit être une adresse email valide",
    "min":      "Doit contenir au moins {param} caractères",
    "max":      "Ne peut pas dépasser {param} caractères",
})
```

Les traductions de validation sont enregistrées une fois au boot, pas par erreur individuelle.

### Ce qu'on ne fait pas

- Pas de fichiers `.po`, `.mo`, ou `.yaml` de traduction
- Pas de pluralisation complexe
- Pas de formatage de dates et nombres localisés

Pour un vrai système i18n applicatif, utiliser une lib dédiée (`go-i18n`, `spreak`). GX ne couvre que les messages d'erreur du framework.

---

## 9. Configuration depuis l'Environnement

### Principe

L'app est configurée avec des options Go au boot. Mais en production, certaines valeurs viennent des variables d'environnement. Une convention explicite évite que chaque projet réinvente son propre chargement de config.

### Variables reconnues automatiquement

```bash
GX_ENV=production           # development | staging | production
GX_PORT=8443                # port d'écoute — override de Listen
GX_HOST=0.0.0.0             # host d'écoute
GX_LOG_LEVEL=info           # debug | info | warn | error
GX_LOG_FORMAT=json          # text | json
GX_READ_TIMEOUT=15s         # format time.Duration
GX_WRITE_TIMEOUT=15s
GX_SHUTDOWN_TIMEOUT=30s
GX_MAX_BODY_SIZE=10MB       # format avec unité
GX_TRUSTED_PROXIES=10.0.0.0/8,172.16.0.0/12
```

Ces variables sont lues **en dernier** — elles surchargent les options Go mais peuvent être désactivées.

### Priorité de résolution

```
1. Options Go  gx.New(gx.ReadTimeout(30*time.Second))  ← plus haute priorité
2. Variables d'environnement GX_READ_TIMEOUT=30s
3. Valeurs par défaut du framework                      ← plus basse priorité
```

L'inverse de ce qu'on pourrait attendre — intentionnel. Le code est la source de vérité. L'environnement surcharge pour les déploiements, mais une option explicite dans le code prend toujours le dessus. Cela évite les surprises en prod où une variable d'env oubliée change un comportement critique.

### Désactiver la lecture d'env

```go
app := gx.New(
    gx.IgnoreEnv(),   // ignorer toutes les variables GX_*
)
```

### Config custom — gx.Config

Pour des besoins plus complexes, un helper de chargement de config applicative :

```go
type AppConfig struct {
    DatabaseURL  string        `env:"DATABASE_URL"  required:"true"`
    RedisURL     string        `env:"REDIS_URL"     default:"redis://localhost:6379"`
    JWTSecret    string        `env:"JWT_SECRET"    required:"true"`
    FeatureFlags FeatureFlags
}

cfg, err := gx.LoadConfig[AppConfig]()
if err != nil {
    log.Fatal("config:", err)
}
```

`gx.LoadConfig[T]()` lit les variables d'environnement selon les tags `env`, applique les `default`, et retourne une erreur si une variable `required` est absente. Pas de fichier `.env` par défaut — c'est le rôle d'un outil externe (`direnv`, Docker, Kubernetes secrets).

---

## 10. Plugin Ordering

### Principe

Quand plusieurs plugins interceptent le même hook (`OnRequest`, `OnError`), l'ordre d'exécution doit être déterministe et contrôlable.

### Ordre par défaut — ordre d'installation

```go
app.Install(pluginA)
app.Install(pluginB)
app.Install(pluginC)

// OnRequest : A → B → C → handler → C → B → A
// OnShutdown : C → B → A (inverse)
```

L'ordre d'installation définit l'ordre d'exécution. C'est simple, prévisible, et documenté.

### Priorité explicite

Pour les plugins qui doivent s'exécuter avant ou après les autres indépendamment de l'ordre d'installation :

```go
app.Install(recovery.New(),
    gx.PluginPriority(1000),    // s'exécute en premier — plus le nombre est grand, plus tôt
)

app.Install(logger.New(),
    gx.PluginPriority(900),
)

app.Install(auth.New(),
    gx.PluginPriority(500),     // après logger
)

app.Install(ratelimit.New(),
    gx.PluginPriority(490),     // juste après auth
)
```

Les plugins standard du framework ont des priorités prédéfinies documentées. Les plugins custom reçoivent la priorité `0` par défaut (après tous les plugins standard).

### Priorités réservées des plugins standard

| Plugin | Priorité OnRequest |
|--------|-------------------|
| `recovery` | 1000 |
| `requestid` | 950 |
| `logger` | 900 |
| `cors` | 850 |
| `ratelimit` | 800 |
| `auth` | 750 |
| `cache` | 700 |
| `compress` | 100 |

### Conflit de priorité

Deux plugins avec la même priorité sont exécutés dans l'ordre d'installation. Pas de panic, pas d'erreur — comportement déterministe documenté.

### OnError — Premier gagnant

Pour `OnError`, le premier plugin qui retourne une Response non-nil court-circuite les suivants. Si aucun plugin ne gère l'erreur, `app.OnError()` est appelé en dernier recours.

```
OnError chain :
  plugin A → retourne nil (ne gère pas)
  plugin B → retourne nil (ne gère pas)
  plugin C → retourne Response ← gagnant, chaîne stoppée
  app.OnError → jamais appelé
```

---

## 11. Early Hints — HTTP/3

### Principe

Le Server Push HTTP/2 est absent en HTTP/3 — il a été délibérément retiré de la spec. Son remplaçant est `103 Early Hints` : une réponse informelle envoyée avant la vraie réponse, qui indique au client quelles ressources il devrait commencer à charger.

```
Client → GET /page HTTP/3
Serveur → 103 Early Hints : Link: </style.css>; rel=preload
          (le handler continue de s'exécuter)
          ...
Serveur → 200 OK : <html>...</html>
```

Le client reçoit le `103` et commence à charger `/style.css` pendant que le serveur finit de traiter la page. Résultat équivalent au push HTTP/2, sans ses problèmes (duplication inutile, impossibilité d'annuler).

### API

```go
func getPage(c *gx.Context) gx.Response {
    // Envoyer les Early Hints avant de construire la réponse
    c.EarlyHints(
        gx.Preload("/style.css",  "style"),
        gx.Preload("/app.js",     "script"),
        gx.Preload("/logo.png",   "image"),
        gx.Prefetch("/api/data",  "fetch"),
    )

    // Construire la page normalement (peut prendre du temps)
    page := buildPage()

    return c.HTML(page)
}
```

### Type gx.Link

```go
type Link struct {
    URI      string
    Rel      string    // preload | prefetch | preconnect | dns-prefetch
    As       string    // style | script | image | font | fetch
    CrossOrigin bool
}

// Helpers
gx.Preload(uri, as string) Link
gx.Prefetch(uri, as string) Link
gx.Preconnect(uri string) Link
```

### Comportement selon le protocole

- **HTTP/3** : envoi immédiat d'un frame `103 Early Hints`. Idéal.
- **HTTP/2** : envoi immédiat d'un frame `103`. Supporté depuis Go 1.20.
- **HTTP/1.1** : no-op silencieux. `103` n'est pas supporté de manière fiable sur HTTP/1.1.

Le développeur écrit le même code quel que soit le protocole. Le framework fait le nécessaire.

### Comparaison Server Push HTTP/2 vs Early Hints HTTP/3

| | Server Push HTTP/2 | Early Hints HTTP/3 |
|---|---|---|
| Mécanisme | Serveur envoie la ressource directement | Serveur indique au client quoi charger |
| Cache du client | Ignoré — doublon possible | Respecté — pas de doublon |
| Annulation | Impossible | Implicite — le client décide |
| Disponibilité | HTTP/2 uniquement | HTTP/2 et HTTP/3 |
| Adoption navigateurs | Abandonnée (Chrome l'a retiré) | Active et croissante |

---

## 12. Panic dans les Goroutines

### Le problème

Le middleware `Recovery()` attrape les panics dans la goroutine principale de la requête. Mais une goroutine lancée depuis un handler qui panic fait crasher le serveur entier — `Recovery()` n'a aucun moyen de l'intercepter.

```go
// Dangereux — la panic fait crasher le serveur
func handler(c *gx.Context) gx.Response {
    go func() {
        panic("boom")   // Recovery() ne peut pas attraper ça
    }()
    return c.NoContent()
}
```

### Solution — gx.Go()

Un wrapper de goroutine fourni par le framework qui récupère les panics et les logue proprement sans crasher le serveur.

```go
// Recommandé
func handler(c *gx.Context) gx.Response {
    gx.Go(func() {
        // panic récupérée ici — loggée, serveur continue
        doAsyncWork()
    })
    return c.NoContent()
}

// Avec contexte propagé
func handler(c *gx.Context) gx.Response {
    ctx := c.GoContext()
    gx.GoCtx(ctx, func(ctx context.Context) {
        doAsyncWork(ctx)
    })
    return c.NoContent()
}
```

### Implémentation

```go
func Go(fn func()) {
    go func() {
        defer func() {
            if r := recover(); r != nil {
                defaultLogger.Error("goroutine panic",
                    "panic", r,
                    "stack", string(debug.Stack()),
                )
            }
        }()
        fn()
    }()
}
```

### Erreur vers canal — Pattern avancé

Pour les goroutines qui doivent reporter un résultat ou une erreur :

```go
func handler(c *gx.Context) gx.Response {
    errCh := make(chan error, 1)

    gx.Go(func() {
        errCh <- doAsyncWork()
    })

    // Attendre avec timeout
    select {
    case err := <-errCh:
        if err != nil {
            return c.Fail(ErrAsyncFailed).Wrap(err)
        }
    case <-time.After(5 * time.Second):
        return c.Fail(ErrTimeout)
    case <-c.GoContext().Done():
        return c.NoContent()
    }

    return c.NoContent()
}
```

### Convention documentée

Règle claire dans la documentation et les reviews de code :

> **Ne jamais lancer `go func()` depuis un handler sans `gx.Go()`.**  
> Ne jamais stocker `*gx.Context` dans une goroutine.  
> Toujours extraire les données nécessaires avant de lancer la goroutine.

```go
// Incorrect
go func() { process(c) }()

// Correct
id  := c.Param("id")
ctx := c.GoContext()
gx.GoCtx(ctx, func(ctx context.Context) {
    process(ctx, id)
})
```

---

## 13. CLI Tooling

### Principe

Un outil en ligne de commande minimal pour les tâches de développement courantes. Pas un générateur de code complexe — juste ce qui évite des tâches répétitives.

### Installation

```bash
go install github.com/example/gx/cmd/gx@latest
```

### Commandes

**`gx new`** — scaffold un nouveau projet

```bash
gx new my-api
gx new my-api --template minimal   # minimal | api | fullstack
```

Génère :

```
my-api/
├── main.go
├── go.mod
├── .env.example
├── Makefile
└── internal/
    ├── users/
    │   ├── handlers.go
    │   ├── router.go
    │   └── errors.go
    └── health/
        └── handlers.go
```

**`gx routes`** — liste toutes les routes enregistrées

```bash
$ gx routes

METHOD  PATH                         MIDDLEWARE
GET     /health                      logger, requestID
GET     /api/v1/users                logger, requestID, auth
GET     /api/v1/users/:id            logger, requestID, auth
POST    /api/v1/users                logger, requestID, auth
PATCH   /api/v1/users/:id            logger, requestID, auth
DELETE  /api/v1/users/:id            logger, requestID, auth
GET     /api/v1/admin/stats          logger, requestID, auth, requireAdmin
```

**`gx openapi`** — exporter la spec OpenAPI

```bash
gx openapi export                    # stdout
gx openapi export --out openapi.json
gx openapi export --format yaml --out openapi.yaml
gx openapi validate                  # valide la spec générée
```

**`gx dev`** — démarrer en mode développement avec hot-reload

```bash
gx dev                               # lance main.go avec rechargement sur modification
gx dev --port 8443
```

Hot-reload via `air` en sous-process — `gx dev` est un wrapper qui installe et lance `air` avec une configuration adaptée à GX.

**`gx cert`** — générer un certificat de développement

```bash
gx cert                              # génère cert.pem + key.pem pour localhost
gx cert --host api.local,127.0.0.1  # hosts custom
gx cert --install                   # installe dans le trust store système (mkcert)
```

### Ce que le CLI ne fait pas

- Pas de génération de code (handlers, models) — trop opinionné sur le domaine
- Pas de migration de base de données — hors périmètre
- Pas de déploiement — utiliser les outils existants (Docker, fly.io, etc.)

---

## 14. Décisions Log

### CDL-010 — Cookies avec défauts sécurisés

**Décision** `HttpOnly: true`, `Secure: true`, `SameSite: Lax` par défaut  
**Raison** La vaste majorité des cookies applicatifs n'ont pas besoin d'être accessibles en JavaScript. Opt-out explicite force une décision consciente.

---

### CDL-011 — ContentType détecté par magic bytes pour les uploads

**Décision** Lire les premiers bytes du fichier pour détecter le MIME type  
**Alternatives** Faire confiance au Content-Type envoyé par le client  
**Raison** Le Content-Type client peut être falsifié. Un attaquant peut uploader un `.php` déguisé en `.jpg`. La détection par magic bytes est la seule approche fiable.

---

### CDL-012 — Static file serving sans directory listing

**Décision** `GET /assets/` retourne `404` si pas d'`index.html`, jamais la liste des fichiers  
**Raison** L'exposition de l'arborescence est rarement intentionnelle et souvent un risque de sécurité.

---

### CDL-013 — HTTPS redirect en 308, pas 301

**Décision** `308 Permanent Redirect`  
**Alternatives** `301 Moved Permanently`  
**Raison** `301` transforme les `POST` en `GET`. `308` conserve la méthode originale. Pour une API REST qui reçoit des `POST` sur HTTP par erreur, `308` est la sémantique correcte.

---

### CDL-014 — i18n limitée aux messages d'erreur

**Décision** Pas de système i18n complet dans GX  
**Raison** L'i18n applicative (dates, pluriels, templates) est un domaine complexe mieux adressé par des libs dédiées. GX se contente de rendre les messages d'erreur traduisibles — c'est le minimum utile sans sur-ingénierie.

---

### CDL-015 — Variables d'environnement surchargées par les options Go

**Décision** Options Go > Variables d'environnement > Défauts  
**Alternatives** Variables d'environnement > Options Go  
**Raison** Le code est la source de vérité. Une option explicite dans le code doit pouvoir résister à une variable d'environnement mal configurée. Le comportement inverse crée des surprises en production difficiles à débugger.

---

### CDL-016 — Early Hints plutôt que Server Push en HTTP/3

**Décision** `c.EarlyHints()` comme primitive principale, `c.ServerPush()` déprécié  
**Raison** Chrome a abandonné le Server Push HTTP/2 en 2022. Early Hints a une adoption croissante et un comportement plus correct (respect du cache client). L'API `EarlyHints` fonctionne sur HTTP/2 et HTTP/3.

---

### CDL-017 — gx.Go() comme wrapper de goroutine obligatoire

**Décision** Convention documentée + wrapper fourni par le framework  
**Alternatives** Laisser le développeur gérer, linter custom  
**Raison** Un panic non-récupéré dans une goroutine fait crasher le serveur entier. Le wrapper est trivial à utiliser et élimine une classe entière de bugs de production.

---

*Document vivant — mis à jour à chaque décision de conception validée.*