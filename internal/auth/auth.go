// Package auth gère la connexion OAuth, les sessions et les tickets WebSocket.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"github.com/felix-marchenay/coinchenet-core/internal/donnees"
)

const (
	NomCookie = "coinche_session"
	// cookieEtat lie la connexion OAuth au navigateur qui l'a commencée (anti « login CSRF »).
	cookieEtat = "coinche_oauth"
	dureeEtat  = 10 * time.Minute
)

type Etat struct {
	Fournisseur string
	Verifier    string
	Nonce       string
	Retour      string
}

// StockEtats conserve l'état d'une connexion OAuth entre la redirection et le retour.
type StockEtats interface {
	Sauver(ctx context.Context, state string, e Etat, ttl time.Duration) error
	Consommer(ctx context.Context, state string) (Etat, bool, error)
}

type Sessions interface {
	Creer(ctx context.Context, joueur uuid.UUID) (string, error)
	Resoudre(ctx context.Context, token string) (uuid.UUID, bool, error)
	Supprimer(ctx context.Context, token string) error
}

type Tickets interface {
	Emettre(ctx context.Context, joueur uuid.UUID) (string, error)
	Consommer(ctx context.Context, ticket string) (uuid.UUID, bool, error)
}

type Config struct {
	URLFront      string
	CookieSecure  bool
	CookieDomaine string
	DureeSession  time.Duration
	// Dev active la connexion sans fournisseur, par simple pseudo : jamais en production.
	Dev bool
}

type Service struct {
	cfg          Config
	fournisseurs map[string]Fournisseur
	etats        StockEtats
	sessions     Sessions
	tickets      Tickets
	joueurs      donnees.Joueurs
	log          *slog.Logger
}

func NouveauService(cfg Config, etats StockEtats, sessions Sessions, tickets Tickets, joueurs donnees.Joueurs, log *slog.Logger, fournisseurs ...Fournisseur) *Service {
	s := &Service{
		cfg:          cfg,
		fournisseurs: map[string]Fournisseur{},
		etats:        etats,
		sessions:     sessions,
		tickets:      tickets,
		joueurs:      joueurs,
		log:          log.With("composant", "auth"),
	}
	for _, f := range fournisseurs {
		s.fournisseurs[f.Nom()] = f
	}
	return s
}

// Routes enregistre les routes /auth/….
func (s *Service) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/fournisseurs", s.listerFournisseurs)
	mux.HandleFunc("GET /auth/{fournisseur}/connexion", s.connexion)
	mux.HandleFunc("GET /auth/{fournisseur}/retour", s.retour)
	mux.HandleFunc("POST /auth/deconnexion", s.deconnexion)
	mux.HandleFunc("POST /auth/ticket", s.ticket)
	if s.cfg.Dev {
		mux.HandleFunc("POST /auth/dev", s.connexionDev)
	}
}

// Authentifier identifie le joueur d'une requête : cookie de session ou en-tête Authorization: Bearer.
func (s *Service) Authentifier(r *http.Request) (uuid.UUID, bool) {
	token := jetonDe(r)
	if token == "" {
		return uuid.Nil, false
	}
	id, ok, err := s.sessions.Resoudre(r.Context(), token)
	if err != nil {
		s.log.Warn("résolution de session impossible", "erreur", err)
		return uuid.Nil, false
	}
	return id, ok
}

// AuthentifierWebSocket accepte en plus un ticket à usage unique (?ticket=…),
// les navigateurs ne pouvant pas poser d'en-tête sur un WebSocket.
func (s *Service) AuthentifierWebSocket(r *http.Request) (uuid.UUID, bool) {
	if t := r.URL.Query().Get("ticket"); t != "" {
		id, ok, err := s.tickets.Consommer(r.Context(), t)
		if err != nil {
			s.log.Warn("consommation de ticket impossible", "erreur", err)
		}
		return id, ok && err == nil
	}
	return s.Authentifier(r)
}

func jetonDe(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		if t, ok := strings.CutPrefix(h, "Bearer "); ok {
			return strings.TrimSpace(t)
		}
	}
	if c, err := r.Cookie(NomCookie); err == nil {
		return c.Value
	}
	return ""
}

func (s *Service) listerFournisseurs(w http.ResponseWriter, _ *http.Request) {
	noms := []string{}
	for _, nom := range []string{"google", "discord"} {
		if _, ok := s.fournisseurs[nom]; ok {
			noms = append(noms, nom)
		}
	}
	if s.cfg.Dev {
		noms = append(noms, "dev")
	}
	ecrireJSON(w, http.StatusOK, map[string]any{"fournisseurs": noms})
}

func (s *Service) connexion(w http.ResponseWriter, r *http.Request) {
	f, ok := s.fournisseurs[r.PathValue("fournisseur")]
	if !ok {
		ecrireErreur(w, http.StatusNotFound, "introuvable", "fournisseur inconnu")
		return
	}
	etat := Etat{
		Fournisseur: f.Nom(),
		Verifier:    oauth2.GenerateVerifier(),
		Nonce:       rand.Text(),
		Retour:      cheminDeRetour(r.URL.Query().Get("retour")),
	}
	state := rand.Text()
	if err := s.etats.Sauver(r.Context(), state, etat, dureeEtat); err != nil {
		s.log.Error("sauvegarde de l'état OAuth impossible", "erreur", err)
		ecrireErreur(w, http.StatusServiceUnavailable, "indisponible", "connexion momentanément impossible")
		return
	}
	http.SetCookie(w, s.cookieEtat(state, dureeEtat))
	cible, err := f.URLAutorisation(r.Context(), state, etat.Verifier, etat.Nonce)
	if err != nil {
		s.log.Error("fournisseur indisponible", "fournisseur", f.Nom(), "erreur", err)
		ecrireErreur(w, http.StatusBadGateway, "indisponible", "le fournisseur d'identité ne répond pas")
		return
	}
	http.Redirect(w, r, cible, http.StatusFound)
}

func (s *Service) retour(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("error") != "" {
		s.rediriger(w, r, "/connexion", url.Values{"erreur": {"refusee"}})
		return
	}
	state := q.Get("state")
	http.SetCookie(w, s.cookieEtat("", -1))
	// Un lien de retour forgé par un tiers ne porte pas le cookie de ce navigateur.
	if c, err := r.Cookie(cookieEtat); err != nil || state == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(state)) != 1 {
		s.rediriger(w, r, "/connexion", url.Values{"erreur": {"expiree"}})
		return
	}
	etat, ok, err := s.etats.Consommer(r.Context(), state)
	if err != nil || !ok || etat.Fournisseur != r.PathValue("fournisseur") {
		s.rediriger(w, r, "/connexion", url.Values{"erreur": {"expiree"}})
		return
	}
	f := s.fournisseurs[etat.Fournisseur]
	id, err := f.Identifier(r.Context(), q.Get("code"), etat.Verifier, etat.Nonce)
	if err != nil {
		s.log.Warn("identification refusée", "fournisseur", f.Nom(), "erreur", err)
		s.rediriger(w, r, "/connexion", url.Values{"erreur": {"identification"}})
		return
	}
	joueur, _, cree, err := s.ouvrirSession(w, r, id)
	if err != nil {
		s.log.Error("ouverture de session impossible", "erreur", err)
		s.rediriger(w, r, "/connexion", url.Values{"erreur": {"indisponible"}})
		return
	}
	s.log.Info("connexion", "joueur", joueur.ID, "fournisseur", f.Nom(), "nouveau", cree)
	params := url.Values{}
	if cree {
		params.Set("nouveau", "1")
	}
	s.rediriger(w, r, etat.Retour, params)
}

func (s *Service) connexionDev(w http.ResponseWriter, r *http.Request) {
	var corps struct {
		Pseudo string `json:"pseudo"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&corps); err != nil {
		ecrireErreur(w, http.StatusBadRequest, "requete_invalide", "corps JSON attendu : {\"pseudo\": \"…\"}")
		return
	}
	if err := donnees.ValiderPseudo(corps.Pseudo); err != nil {
		ecrireErreur(w, http.StatusBadRequest, "pseudo_invalide", err.Error())
		return
	}
	joueur, token, _, err := s.ouvrirSession(w, r, donnees.Identite{Fournisseur: "dev", Sujet: strings.ToLower(corps.Pseudo), Nom: corps.Pseudo})
	if err != nil {
		s.log.Error("connexion de développement impossible", "erreur", err)
		ecrireErreur(w, http.StatusServiceUnavailable, "indisponible", "connexion momentanément impossible")
		return
	}
	ecrireJSON(w, http.StatusOK, map[string]any{"joueur": joueur, "token": token})
}

func (s *Service) ouvrirSession(w http.ResponseWriter, r *http.Request, id donnees.Identite) (donnees.Joueur, string, bool, error) {
	joueur, cree, err := s.joueurs.TrouverOuCreer(r.Context(), id)
	if err != nil {
		return donnees.Joueur{}, "", false, err
	}
	token, err := s.sessions.Creer(r.Context(), joueur.ID)
	if err != nil {
		return donnees.Joueur{}, "", false, err
	}
	http.SetCookie(w, s.cookie(token, s.cfg.DureeSession))
	return joueur, token, cree, nil
}

func (s *Service) deconnexion(w http.ResponseWriter, r *http.Request) {
	if token := jetonDe(r); token != "" {
		if err := s.sessions.Supprimer(r.Context(), token); err != nil {
			s.log.Warn("suppression de session impossible", "erreur", err)
		}
	}
	http.SetCookie(w, s.cookie("", -1))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) ticket(w http.ResponseWriter, r *http.Request) {
	joueur, ok := s.Authentifier(r)
	if !ok {
		ecrireErreur(w, http.StatusUnauthorized, "non_authentifie", "connexion requise")
		return
	}
	t, err := s.tickets.Emettre(r.Context(), joueur)
	if err != nil {
		s.log.Error("émission de ticket impossible", "erreur", err)
		ecrireErreur(w, http.StatusServiceUnavailable, "indisponible", "ticket momentanément indisponible")
		return
	}
	ecrireJSON(w, http.StatusOK, map[string]string{"ticket": t})
}

func (s *Service) cookieEtat(state string, duree time.Duration) *http.Cookie {
	c := &http.Cookie{ //nolint:gosec // Secure est désactivable pour le développement en HTTP.
		Name:     cookieEtat,
		Value:    state,
		Path:     "/auth/",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(duree.Seconds()),
	}
	if duree < 0 {
		c.MaxAge = -1
	}
	return c
}

func (s *Service) cookie(valeur string, duree time.Duration) *http.Cookie {
	c := &http.Cookie{ //nolint:gosec // Secure est désactivable pour le développement en HTTP.
		Name:     NomCookie,
		Value:    valeur,
		Path:     "/",
		Domain:   s.cfg.CookieDomaine,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
	if duree < 0 {
		c.MaxAge = -1
	} else {
		c.MaxAge = int(duree.Seconds())
	}
	return c
}

func (s *Service) rediriger(w http.ResponseWriter, r *http.Request, chemin string, params url.Values) {
	cible := s.cfg.URLFront + chemin
	if len(params) > 0 {
		cible += "?" + params.Encode()
	}
	http.Redirect(w, r, cible, http.StatusFound)
}

// cheminDeRetour n'accepte qu'un chemin relatif, pour éviter toute redirection ouverte.
func cheminDeRetour(retour string) string {
	if !strings.HasPrefix(retour, "/") || strings.HasPrefix(retour, "//") || strings.Contains(retour, `\`) {
		return "/"
	}
	if u, err := url.Parse(retour); err != nil || u.Host != "" || u.Scheme != "" {
		return "/"
	}
	return retour
}

func ecrireJSON(w http.ResponseWriter, statut int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statut)
	_ = json.NewEncoder(w).Encode(v)
}

func ecrireErreur(w http.ResponseWriter, statut int, code, message string) {
	ecrireJSON(w, statut, map[string]any{"erreur": map[string]string{"code": code, "message": message}})
}
