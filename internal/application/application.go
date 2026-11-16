// Package application assemble les composants du serveur et gère son cycle de vie.
package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"

	"github.com/felix-marchenay/coinchenet-core/internal/api"
	"github.com/felix-marchenay/coinchenet-core/internal/auth"
	"github.com/felix-marchenay/coinchenet-core/internal/bot"
	"github.com/felix-marchenay/coinchenet-core/internal/config"
	"github.com/felix-marchenay/coinchenet-core/internal/matchmaking"
	"github.com/felix-marchenay/coinchenet-core/internal/observabilite"
	"github.com/felix-marchenay/coinchenet-core/internal/stockage/postgres"
	"github.com/felix-marchenay/coinchenet-core/internal/stockage/redisstore"
	"github.com/felix-marchenay/coinchenet-core/internal/table"
)

type Application struct {
	cfg      config.Config
	log      *slog.Logger
	base     *postgres.Base
	redis    *redis.Client
	bus      *redisstore.Bus
	registre *table.Registre
	api      *api.Serveur
	http     *http.Server
	admin    *http.Server
	adresse  net.Addr
	annuler  context.CancelFunc
	// annulerFile arrête la direction de la file classée, avant les tables.
	annulerFile context.CancelFunc
	fondsFini   chan struct{}
}

// Demarrer ouvre les connexions, applique les migrations et commence à servir.
func Demarrer(ctx context.Context, cfg config.Config, log *slog.Logger, version string) (*Application, error) {
	a := &Application{cfg: cfg, log: log}
	ok := false
	defer func() {
		if !ok {
			a.fermerStockage()
		}
	}()

	var err error
	if a.base, err = postgres.Ouvrir(ctx, cfg.PostgresURL); err != nil {
		return nil, fmt.Errorf("postgres : %w", err)
	}
	if cfg.MigrerAuDemarrage {
		if err := a.base.Migrer(ctx); err != nil {
			return nil, fmt.Errorf("migrations : %w", err)
		}
	}
	if a.redis, err = redisstore.Ouvrir(ctx, cfg.RedisURL); err != nil {
		return nil, fmt.Errorf("redis : %w", err)
	}

	fond, annuler := context.WithCancel(context.WithoutCancel(ctx))
	a.annuler = annuler
	a.bus = redisstore.NouveauBus(a.redis, log)
	if err := a.bus.Demarrer(fond); err != nil {
		annuler()
		return nil, fmt.Errorf("bus : %w", err)
	}

	diffuseur := redisstore.NouveauDiffuseur(a.bus, log)
	annuaire := redisstore.NouvelAnnuaire(a.redis)
	verrous := redisstore.NouveauxVerrous(a.redis)

	var metriques *observabilite.Metriques
	a.registre = table.NouveauRegistre(table.ConfigRegistre{
		Instance:          cfg.Instance,
		IntervalleReprise: cfg.IntervalleReprise,
		OptionsClassees:   table.Options{Regles: table.OptionsClassees().Regles, DelaiAction: cfg.DelaiActionClassee},
		Table:             a.configTable(),
	}, table.Dependances{
		Journal:    a.base,
		Archiviste: a.base,
		Diffuseur:  diffuseur,
		Annuaire:   annuaire,
		Decideur:   bot.Bots{},
		Metriques:  metriquesTable{&metriques},
		Logger:     log,
	}, a.bus, verrous, redisstore.NouveauxCodes(a.redis), a.base)
	metriques = observabilite.NouvellesMetriques(a.registre.NbTables)

	file := matchmaking.NouveauService(matchmaking.Config{
		Instance:   cfg.Instance,
		Parametres: matchmaking.ParametresParDefaut(),
	}, redisstore.NouvelleFileMatchmaking(a.redis), redisstore.NouvelEtatMatchmaking(a.redis), verrous, a.registre, diffuseur, metriques, log)

	authentification := auth.NouveauService(auth.Config{
		URLFront:      cfg.URLFront,
		CookieSecure:  cfg.CookieSecure,
		CookieDomaine: cfg.CookieDomaine,
		DureeSession:  cfg.DureeSession,
		Dev:           cfg.AuthDev,
	}, etatsOAuth{redisstore.NouveauxEtatsOAuth(a.redis)},
		redisstore.NouvellesSessions(a.redis, cfg.DureeSession),
		redisstore.NouveauxTickets(a.redis, 30*time.Second),
		a.base, log, a.fournisseurs()...)
	if cfg.AuthDev {
		log.Warn("connexion de développement activée (COINCHE_AUTH_DEV) : à ne jamais utiliser en production")
	}

	a.api = api.NouveauServeur(api.Dependances{
		Auth:       authentification,
		Joueurs:    a.base,
		Historique: a.base,
		Journal:    a.base,
		Tables:     a.registre,
		Annuaire:   annuaire,
		File:       file,
		Bus:        a.bus,
		Metriques:  metriques,
		Verifications: []api.Verification{
			{Nom: "postgres", Verifier: a.base.Ping},
			{Nom: "redis", Verifier: func(ctx context.Context) error { return a.redis.Ping(ctx).Err() }},
		},
		Origines: cfg.Origines,
		HotesWS:  cfg.HotesOrigines(),
		DebitWS:  rate.Limit(cfg.DebitWS),
		Version:  version,
		Log:      log,
	})

	ecoute, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.Adresse)
	if err != nil {
		annuler()
		return nil, fmt.Errorf("écoute sur %s : %w", cfg.Adresse, err)
	}
	a.adresse = ecoute.Addr()
	a.http = &http.Server{
		Handler:           a.api,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	a.admin = &http.Server{Addr: cfg.AdresseAdmin, Handler: routesAdmin(metriques), ReadHeaderTimeout: 10 * time.Second}

	a.registre.Demarrer(fond)
	a.fondsFini = make(chan struct{})
	ctxFile, annulerFile := context.WithCancel(fond)
	a.annulerFile = annulerFile
	go func() {
		defer close(a.fondsFini)
		file.Executer(ctxFile)
	}()
	go a.servir(a.http, ecoute)
	go func() {
		if err := a.admin.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("serveur d'administration arrêté", "erreur", err)
		}
	}()

	log.Info("serveur démarré", "adresse", a.adresse.String(), "admin", cfg.AdresseAdmin, "instance", cfg.Instance, "version", version)
	ok = true
	return a, nil
}

// Adresse renvoie l'adresse d'écoute effective (utile avec le port 0).
func (a *Application) Adresse() string {
	return a.adresse.String()
}

// Arreter cesse d'accepter des connexions, ferme les WebSocket et rend les tables
// pour qu'une autre instance les reprenne.
func (a *Application) Arreter(ctx context.Context) error {
	a.log.Info("arrêt en cours")
	errHTTP := a.http.Shutdown(ctx)
	a.api.Fermer()
	// La file d'abord : elle ne doit plus créer de tables sur une instance qui s'en va.
	a.annulerFile()
	select {
	case <-a.fondsFini:
	case <-ctx.Done():
	}
	a.registre.Arreter(ctx)
	_ = a.admin.Shutdown(ctx)
	a.annuler()
	a.fermerStockage()
	a.log.Info("arrêt terminé")
	return errHTTP
}

func (a *Application) servir(s *http.Server, ecoute net.Listener) {
	if err := s.Serve(ecoute); err != nil && !errors.Is(err, http.ErrServerClosed) {
		a.log.Error("serveur HTTP arrêté", "erreur", err)
	}
}

func (a *Application) fermerStockage() {
	if a.bus != nil {
		_ = a.bus.Fermer()
	}
	if a.redis != nil {
		_ = a.redis.Close()
	}
	if a.base != nil {
		a.base.Fermer()
	}
}

func (a *Application) configTable() table.Config {
	c := table.ConfigParDefaut()
	c.DelaiBot = a.cfg.DelaiBot
	c.DelaiDemarrage = a.cfg.DelaiDemarrage
	c.DelaiEntreMenes = a.cfg.DelaiEntreMenes
	c.DelaiRetour = a.cfg.DelaiRetour
	c.DelaiLobby = a.cfg.DelaiLobby
	return c
}

func (a *Application) fournisseurs() []auth.Fournisseur {
	var fs []auth.Fournisseur
	if a.cfg.Google.Active() {
		fs = append(fs, auth.NouveauGoogle(a.cfg.Google.ClientID, a.cfg.Google.ClientSecret, a.cfg.URLPublique+"/auth/google/retour"))
	}
	if a.cfg.Discord.Active() {
		fs = append(fs, auth.NouveauDiscord(a.cfg.Discord.ClientID, a.cfg.Discord.ClientSecret, a.cfg.URLPublique+"/auth/discord/retour"))
	}
	return fs
}

func routesAdmin(m *observabilite.Metriques) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", m.Handler())
	mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
	return mux
}

type etatsOAuth struct {
	*redisstore.EtatsOAuth
}

func (e etatsOAuth) Sauver(ctx context.Context, state string, v auth.Etat, ttl time.Duration) error {
	return e.EtatsOAuth.Sauver(ctx, state, redisstore.EtatOAuth(v), ttl)
}

func (e etatsOAuth) Consommer(ctx context.Context, state string) (auth.Etat, bool, error) {
	v, ok, err := e.EtatsOAuth.Consommer(ctx, state)
	return auth.Etat(v), ok, err
}

// metriquesTable fait le lien avec des métriques créées après le registre (qui en a besoin pour compter ses tables).
type metriquesTable struct {
	m **observabilite.Metriques
}

func (t metriquesTable) PartieTerminee(typ table.Type, c table.Cause) {
	if *t.m != nil {
		(*t.m).PartieTerminee(typ, c)
	}
}

func (t metriquesTable) ActionTraitee(commande string, duree time.Duration) {
	if *t.m != nil {
		(*t.m).ActionTraitee(commande, duree)
	}
}
