// Package api expose l'API REST et la passerelle WebSocket.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"

	"github.com/felix-marchenay/coinchenet-core/internal/donnees"
	"github.com/felix-marchenay/coinchenet-core/internal/matchmaking"
	"github.com/felix-marchenay/coinchenet-core/internal/protocole"
	"github.com/felix-marchenay/coinchenet-core/internal/table"
)

type Authentificateur interface {
	Routes(mux *http.ServeMux)
	Authentifier(r *http.Request) (uuid.UUID, bool)
	AuthentifierWebSocket(r *http.Request) (uuid.UUID, bool)
}

type Tables interface {
	CreerPrivee(ctx context.Context, createur table.Joueur, o table.Options) (uuid.UUID, string, error)
	Envoyer(ctx context.Context, table uuid.UUID, c table.Commande) error
	ResoudreCode(ctx context.Context, code string) (uuid.UUID, bool, error)
	Vivante(ctx context.Context, table uuid.UUID) (bool, error)
}

type Annuaire interface {
	TableDe(ctx context.Context, joueur uuid.UUID) (uuid.UUID, bool, error)
	Dissocier(ctx context.Context, joueur, table uuid.UUID) error
}

type File interface {
	Envoyer(ctx context.Context, c matchmaking.Commande) error
}

type Bus interface {
	AbonnerJoueur(ctx context.Context, joueur uuid.UUID, recevoir func([]byte)) (func(), error)
}

type Metriques interface {
	ConnexionOuverte()
	ConnexionFermee()
	Requete(methode, route string, statut int, duree time.Duration)
}

// Verification est un contrôle de disponibilité (base, cache…) exposé sur /pret.
type Verification struct {
	Nom      string
	Verifier func(ctx context.Context) error
}

type Dependances struct {
	Auth          Authentificateur
	Joueurs       donnees.Joueurs
	Historique    donnees.Historique
	Journal       table.Journal
	Tables        Tables
	Annuaire      Annuaire
	File          File
	Bus           Bus
	Metriques     Metriques
	Verifications []Verification
	Origines      []string
	HotesWS       []string
	// DebitWS borne les messages d'un client WebSocket (par seconde) ; 10 par défaut.
	DebitWS rate.Limit
	Version string
	Log     *slog.Logger
}

type Serveur struct {
	d            Dependances
	log          *slog.Logger
	handler      http.Handler
	limiteur     *limiteur
	limiteurAuth *limiteur
	ws           *passerelle
}

func NouveauServeur(d Dependances) *Serveur {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.DebitWS == 0 {
		d.DebitWS = 10
	}
	s := &Serveur{
		d:            d,
		log:          d.Log.With("composant", "api"),
		limiteur:     nouveauLimiteur(rate.Every(6*time.Second), 10),
		limiteurAuth: nouveauLimiteur(rate.Every(time.Second), 30),
	}
	s.ws = nouvellePasserelle(d, s.log)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /sante", s.sante)
	mux.HandleFunc("GET /pret", s.pret)
	mux.HandleFunc("GET /version", s.version)
	mux.HandleFunc("GET /regles", s.regles)
	mux.HandleFunc("GET /bots", s.bots)
	d.Auth.Routes(mux)
	mux.HandleFunc("GET /moi", s.authentifie(s.moi))
	mux.HandleFunc("PATCH /moi", s.authentifie(s.modifierMoi))
	mux.HandleFunc("GET /joueurs", s.chercherJoueur)
	mux.HandleFunc("GET /joueurs/{id}", s.joueur)
	mux.HandleFunc("GET /joueurs/{id}/parties", s.partiesDuJoueur)
	mux.HandleFunc("GET /joueurs/{id}/elo", s.evolutionElo)
	mux.HandleFunc("GET /classement", s.classement)
	mux.HandleFunc("GET /parties/{id}", s.partie)
	mux.HandleFunc("GET /parties/{id}/evenements", s.evenements)
	mux.HandleFunc("POST /tables", s.authentifie(s.creerTable))
	mux.HandleFunc("GET /ws", s.ws.ServeHTTP)

	s.handler = s.recuperation(s.journaliser(s.cors(entetesSecurite(s.limiterAuth(mux)))))
	return s
}

// limiterAuth borne par adresse IP les routes de connexion, qui créent un état en Redis sans authentification.
func (s *Serveur) limiterAuth(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/auth/") && !s.limiteurAuth.autoriser(adresseClient(r)) {
			ecrireErreur(w, http.StatusTooManyRequests, protocole.CodeTropDeRequetes, "trop de tentatives, patientez un peu")
			return
		}
		h.ServeHTTP(w, r)
	})
}

// tableActuelle renvoie la table du joueur. Une association vers une table qu'aucune instance
// n'exécute plus (salon perdu lors d'un arrêt, partie annulée…) est effacée : une partie reprise
// ailleurs réassocie elle-même ses joueurs.
func tableActuelle(ctx context.Context, d Dependances, joueur uuid.UUID) (uuid.UUID, bool) {
	t, ok, err := d.Annuaire.TableDe(ctx, joueur)
	if err != nil || !ok {
		return uuid.Nil, false
	}
	vivante, err := d.Tables.Vivante(ctx, t)
	if err != nil {
		return t, true
	}
	if !vivante {
		_ = d.Annuaire.Dissocier(ctx, joueur, t)
		return uuid.Nil, false
	}
	return t, true
}

func (s *Serveur) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

// Fermer coupe proprement les connexions WebSocket ouvertes.
func (s *Serveur) Fermer() {
	s.ws.fermer()
}

type cleJoueur struct{}

func joueurDe(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(cleJoueur{}).(uuid.UUID)
	return id
}

func (s *Serveur) authentifie(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := s.d.Auth.Authentifier(r)
		if !ok {
			ecrireErreur(w, http.StatusUnauthorized, protocole.CodeNonAuthentifie, "connexion requise")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), cleJoueur{}, id)))
	}
}

type reponseEnregistree struct {
	http.ResponseWriter
	statut int
}

func (r *reponseEnregistree) WriteHeader(statut int) {
	r.statut = statut
	r.ResponseWriter.WriteHeader(statut)
}

func (r *reponseEnregistree) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (s *Serveur) journaliser(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		debut := time.Now()
		rec := &reponseEnregistree{ResponseWriter: w, statut: http.StatusOK}
		h.ServeHTTP(rec, r)
		duree := time.Since(debut)
		if s.d.Metriques != nil {
			s.d.Metriques.Requete(r.Method, r.Pattern, rec.statut, duree)
		}
		if r.URL.Path == "/sante" || r.URL.Path == "/pret" {
			return
		}
		s.log.LogAttrs(r.Context(), slog.LevelInfo, "requête",
			slog.String("methode", r.Method),
			slog.String("chemin", r.URL.Path),
			slog.Int("statut", rec.statut),
			slog.Duration("duree", duree),
			slog.String("ip", adresseClient(r)),
		)
	})
}

func (s *Serveur) recuperation(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(v)
				}
				s.log.Error("panique dans un handler", "valeur", v, "pile", string(debug.Stack()))
				ecrireErreur(w, http.StatusInternalServerError, protocole.CodeInterne, "erreur interne")
			}
		}()
		h.ServeHTTP(w, r)
	})
}

// cors autorise le client web (origines configurées) à appeler l'API avec ses cookies.
func (s *Serveur) cors(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origine := r.Header.Get("Origin")
		if origine != "" && slices.Contains(s.d.Origines, origine) {
			w.Header().Set("Access-Control-Allow-Origin", origine)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Add("Vary", "Origin")
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}

func entetesSecurite(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

func adresseClient(r *http.Request) string {
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		premier, _, _ := strings.Cut(f, ",")
		return strings.TrimSpace(premier)
	}
	hote, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return hote
}

func ecrireJSON(w http.ResponseWriter, statut int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statut)
	_ = json.NewEncoder(w).Encode(v)
}

func ecrireErreur(w http.ResponseWriter, statut int, code, message string) {
	ecrireJSON(w, statut, map[string]protocole.Erreur{"erreur": {Code: code, Message: message}})
}

var errCorpsInvalide = errors.New("corps JSON invalide")

// lireJSON exige un Content-Type JSON : un formulaire d'un autre site ne peut pas forger la requête.
func lireJSON(w http.ResponseWriter, r *http.Request, v any) error {
	if t, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || t != "application/json" {
		return errCorpsInvalide
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errCorpsInvalide
	}
	return nil
}

// limiteur borne le débit par clé (IP ou joueur) ; les entrées inactives sont oubliées.
type limiteur struct {
	mu      sync.Mutex
	debit   rate.Limit
	rafale  int
	entrees map[string]*entreeLimiteur
	menage  time.Time
}

type entreeLimiteur struct {
	l  *rate.Limiter
	vu time.Time
}

func nouveauLimiteur(debit rate.Limit, rafale int) *limiteur {
	return &limiteur{debit: debit, rafale: rafale, entrees: map[string]*entreeLimiteur{}, menage: time.Now()}
}

func (l *limiteur) autoriser(cle string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	maintenant := time.Now()
	if maintenant.Sub(l.menage) > 10*time.Minute {
		for k, e := range l.entrees {
			if maintenant.Sub(e.vu) > 10*time.Minute {
				delete(l.entrees, k)
			}
		}
		l.menage = maintenant
	}
	e, ok := l.entrees[cle]
	if !ok {
		e = &entreeLimiteur{l: rate.NewLimiter(l.debit, l.rafale)}
		l.entrees[cle] = e
	}
	e.vu = maintenant
	return e.l.Allow()
}
