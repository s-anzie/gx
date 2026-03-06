# GX Core — ListenH3 & Alt-Svc Bootstrap

> **Décision** CDL-008  
> **Remplace** `ListenDual` — supprimé  
> **Statut** Validé

---

## Principe

`ListenH3` est le seul point d'entrée HTTP/3. Il démarre le serveur QUIC **et** gère lui-même le bootstrap Alt-Svc — le développeur ne s'en occupe pas.

```go
app.Listen(":8443")     // HTTP/2 seul (TCP)
app.ListenH3(":8443")   // HTTP/3 — shim TCP + Alt-Svc inclus par défaut
```

---

## Le problème de bootstrap HTTP/3

HTTP/3 a une contrainte de découverte : un client apprend qu'un serveur parle HTTP/3 via le header `Alt-Svc` d'une réponse HTTP précédente. Sans ça, le client essaie TCP, n'obtient rien, et abandonne.

```
1ère visite — client naïf
  Client → TCP :8443 → shim répond : Alt-Svc: h3=":8443"; ma=86400
                                      308 Redirect → même URL

Visites suivantes — client informé
  Client → UDP :8443 → HTTP/3 directement
                        (shim n'intervient plus)
```

Le shim TCP n'a qu'un seul rôle : annoncer HTTP/3 une fois. Ensuite le client mémorise l'annonce pendant `ma` secondes (86400 = 24h par défaut) et se connecte directement en QUIC.

---

## Implémentation interne

Le shim est invisible pour le développeur. Il est démarré automatiquement par `ListenH3`.

```go
func (a *App) ListenH3(addr string, opts ...H3Option) error {
    cfg := defaultH3Config()
    for _, o := range opts {
        o(cfg)
    }

    // Serveur QUIC principal
    h3srv := &http3.Server{
        Addr:      addr,
        Handler:   a,
        TLSConfig: a.tlsConfig,
    }

    // Shim TCP — bootstrap Alt-Svc uniquement
    if cfg.altSvcShim {
        go a.startAltSvcShim(addr, cfg.altSvcMaxAge)
    }

    return h3srv.ListenAndServe()
}

func (a *App) startAltSvcShim(addr string, maxAge time.Duration) {
    altSvc := fmt.Sprintf(`h3="%s"; ma=%.0f`, addr, maxAge.Seconds())

    shim := &http.Server{
        Addr:      addr,
        TLSConfig: a.tlsConfig,
        Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.Header().Set("Alt-Svc", altSvc)
            http.Redirect(w, r,
                "https://"+r.Host+r.RequestURI,
                http.StatusPermanentRedirect,
            )
        }),
    }

    // Erreur non-fatale — le serveur H3 principal continue
    if err := shim.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
        a.logger.Warn("alt-svc shim failed", "err", err)
    }
}
```

---

## Options

Tout est opt-out. Le comportement par défaut est le plus utile possible.

```go
// Défaut — shim actif, max-age 24h
app.ListenH3(":8443")

// Désactiver le shim — clients internes qui connaissent déjà le protocole
app.ListenH3(":8443", gx.WithoutAltSvcShim())

// Personnaliser le max-age du header Alt-Svc
app.ListenH3(":8443", gx.AltSvcMaxAge(12 * time.Hour))
```

### Quand désactiver le shim

- Microservices internes dont le client est un service Go configuré explicitement pour HTTP/3
- Environnements où TCP est bloqué et seul UDP est disponible
- Benchmarking pour isoler les performances QUIC pures

---

## Alt-Svc et DNS HTTPS Record

Pour les déploiements publics, le shim couvre le cas général mais pas le cas de la toute première connexion d'un client qui n'a jamais vu le site. Pour ça, un `HTTPS` DNS record permet d'annoncer HTTP/3 avant même la première requête TCP :

```
_443._tcp.example.com.  HTTPS  1  . alpn="h3,h2" port=443
```

C'est hors du périmètre du framework — c'est une config DNS — mais c'est la solution complète pour du HTTP/3-first en production.

---

## Graceful Shutdown

Le shim TCP suit le même cycle de shutdown que le serveur principal. Les deux sont drainés ensemble lors de la réception de `SIGTERM`.

```
SIGTERM
  ↓
Stop HTTP/3 (QUIC)   ← serveur principal
Stop shim TCP        ← bootstrap
  ↓
Drain (ShutdownTimeout)
  ↓
Exit 0
```

---

## Résumé des décisions

| Avant | Après |
|-------|-------|
| `Listen()` + `ListenDual()` + `ListenH3()` | `Listen()` + `ListenH3()` |
| Alt-Svc injecté dans le handler de `ListenDual` | Alt-Svc géré par le shim interne de `ListenH3` |
| Développeur choisit dual ou H3 seul | Développeur choisit son protocole cible, bootstrap automatique |
| `ListenDual` requis pour exposition publique | `ListenH3` suffisant dans tous les cas |