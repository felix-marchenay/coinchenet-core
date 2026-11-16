package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/felix-marchenay/coinchenet-core/internal/donnees"
)

type fauxFournisseur struct{}

func (fauxFournisseur) Nom() string {
	return "faux"
}

func (fauxFournisseur) URLAutorisation(_ context.Context, state, verifier, nonce string) (string, error) {
	return "https://idp.test/autoriser?" + url.Values{"state": {state}, "verifier": {verifier}, "nonce": {nonce}}.Encode(), nil
}

func (fauxFournisseur) Identifier(_ context.Context, code, verifier, nonce string) (donnees.Identite, error) {
	if code != "bon" || verifier == "" || nonce == "" {
		return donnees.Identite{}, errors.New("code refusé")
	}
	return donnees.Identite{Fournisseur: "faux", Sujet: "sujet-1", Nom: "Alice"}, nil
}

type memoire struct {
	mu       sync.Mutex
	etats    map[string]Etat
	sessions map[string]uuid.UUID
	tickets  map[string]uuid.UUID
	joueurs  map[string]donnees.Joueur
}

func nouvelleMemoire() *memoire {
	return &memoire{etats: map[string]Etat{}, sessions: map[string]uuid.UUID{}, tickets: map[string]uuid.UUID{}, joueurs: map[string]donnees.Joueur{}}
}

func (m *memoire) Sauver(_ context.Context, state string, e Etat, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.etats[state] = e
	return nil
}

func (m *memoire) Consommer(_ context.Context, state string) (Etat, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.etats[state]
	delete(m.etats, state)
	return e, ok, nil
}

type sessionsMemoire struct{ *memoire }

func (s sessionsMemoire) Creer(_ context.Context, j uuid.UUID) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := uuid.NewString()
	s.sessions[t] = j
	return t, nil
}

func (s sessionsMemoire) Resoudre(_ context.Context, t string) (uuid.UUID, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.sessions[t]
	return j, ok, nil
}

func (s sessionsMemoire) Supprimer(_ context.Context, t string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, t)
	return nil
}

type ticketsMemoire struct{ *memoire }

func (s ticketsMemoire) Emettre(_ context.Context, j uuid.UUID) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := uuid.NewString()
	s.tickets[t] = j
	return t, nil
}

func (s ticketsMemoire) Consommer(_ context.Context, t string) (uuid.UUID, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.tickets[t]
	delete(s.tickets, t)
	return j, ok, nil
}

type joueursMemoire struct{ *memoire }

func (j joueursMemoire) JoueurParID(context.Context, uuid.UUID) (donnees.Joueur, error) {
	return donnees.Joueur{}, donnees.ErrIntrouvable
}

func (j joueursMemoire) JoueurParPseudo(context.Context, string) (donnees.Joueur, error) {
	return donnees.Joueur{}, donnees.ErrIntrouvable
}

func (j joueursMemoire) TrouverOuCreer(_ context.Context, id donnees.Identite) (donnees.Joueur, bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	cle := id.Fournisseur + "/" + id.Sujet
	if existant, ok := j.joueurs[cle]; ok {
		return existant, false, nil
	}
	nouveau := donnees.Joueur{ID: uuid.New(), Pseudo: donnees.SuggererPseudo(id.Nom), Elo: 1200}
	j.joueurs[cle] = nouveau
	return nouveau, true, nil
}

func (j joueursMemoire) Renommer(context.Context, uuid.UUID, string) (donnees.Joueur, error) {
	return donnees.Joueur{}, nil
}

func service(t *testing.T, dev bool) (*Service, *memoire, http.Handler) {
	t.Helper()
	m := nouvelleMemoire()
	s := NouveauService(Config{URLFront: "https://coinche.test", CookieSecure: true, DureeSession: time.Hour, Dev: dev},
		m, sessionsMemoire{m}, ticketsMemoire{m}, joueursMemoire{m}, discard(), fauxFournisseur{})
	mux := http.NewServeMux()
	s.Routes(mux)
	return s, m, mux
}

func executer(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestConnexionComplete(t *testing.T) {
	_, m, h := service(t, false)

	w := executer(h, httptest.NewRequest(http.MethodGet, "/auth/faux/connexion?retour=/salon", nil))
	require.Equal(t, http.StatusFound, w.Code)
	cible, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "idp.test", cible.Host)
	state := cible.Query().Get("state")
	require.Contains(t, m.etats, state)
	assert.Equal(t, "/salon", m.etats[state].Retour)

	lie := cookieNomme(t, w, cookieEtat)
	assert.Equal(t, state, lie.Value, "l'état est lié au navigateur")
	assert.True(t, lie.HttpOnly)

	sansCookie := httptest.NewRequest(http.MethodGet, "/auth/faux/retour?code=bon&state="+state, nil)
	w = executer(h, sansCookie)
	assert.Equal(t, "https://coinche.test/connexion?erreur=expiree", w.Header().Get("Location"), "un lien de retour transmis à un autre navigateur est refusé")
	require.Contains(t, m.etats, state)

	w = executer(h, retour("/auth/faux/retour?code=bon&state="+state, state))
	require.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "https://coinche.test/salon?nouveau=1", w.Header().Get("Location"))
	session := cookieNomme(t, w, NomCookie)
	assert.True(t, session.HttpOnly)
	assert.True(t, session.Secure)
	assert.Equal(t, http.SameSiteLaxMode, session.SameSite)
	assert.Contains(t, m.sessions, session.Value)
	assert.Equal(t, -1, cookieNomme(t, w, cookieEtat).MaxAge, "le cookie d'état est effacé")

	w = executer(h, retour("/auth/faux/retour?code=bon&state="+state, state))
	assert.Equal(t, "https://coinche.test/connexion?erreur=expiree", w.Header().Get("Location"), "un état ne sert qu'une fois")
}

func retour(chemin, state string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, chemin, nil)
	r.AddCookie(&http.Cookie{Name: cookieEtat, Value: state})
	return r
}

func cookieNomme(t *testing.T, w *httptest.ResponseRecorder, nom string) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == nom {
			return c
		}
	}
	t.Fatalf("cookie %s absent", nom)
	return nil
}

func TestRetoursEnErreur(t *testing.T) {
	_, m, h := service(t, false)

	w := executer(h, httptest.NewRequest(http.MethodGet, "/auth/faux/retour?error=access_denied", nil))
	assert.Equal(t, "https://coinche.test/connexion?erreur=refusee", w.Header().Get("Location"))

	m.etats["s1"] = Etat{Fournisseur: "faux", Verifier: "v", Nonce: "n", Retour: "/"}
	w = executer(h, retour("/auth/faux/retour?code=mauvais&state=s1", "s1"))
	assert.Equal(t, "https://coinche.test/connexion?erreur=identification", w.Header().Get("Location"))

	m.etats["s2"] = Etat{Fournisseur: "autre", Retour: "/"}
	w = executer(h, retour("/auth/faux/retour?code=bon&state=s2", "s2"))
	assert.Equal(t, "https://coinche.test/connexion?erreur=expiree", w.Header().Get("Location"), "l'état doit venir du même fournisseur")

	w = executer(h, httptest.NewRequest(http.MethodGet, "/auth/inconnu/connexion", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestFournisseurs(t *testing.T) {
	_, _, h := service(t, true)
	w := executer(h, httptest.NewRequest(http.MethodGet, "/auth/fournisseurs", nil))
	assert.JSONEq(t, `{"fournisseurs":["dev"]}`, w.Body.String(), "seuls les fournisseurs connus sont listés")
}

func TestConnexionDev(t *testing.T) {
	_, _, h := service(t, false)
	w := executer(h, httptest.NewRequest(http.MethodPost, "/auth/dev", strings.NewReader(`{"pseudo":"Bob"}`)))
	assert.NotEqual(t, http.StatusOK, w.Code, "désactivée par défaut")

	s, _, h := service(t, true)
	w = executer(h, httptest.NewRequest(http.MethodPost, "/auth/dev", strings.NewReader(`{"pseudo":"x"}`)))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = executer(h, httptest.NewRequest(http.MethodPost, "/auth/dev", strings.NewReader(`{"pseudo":"Bob"}`)))
	require.Equal(t, http.StatusOK, w.Code)
	var reponse struct {
		Token  string         `json:"token"`
		Joueur donnees.Joueur `json:"joueur"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &reponse))
	assert.Equal(t, "Bob", reponse.Joueur.Pseudo)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer "+reponse.Token)
	id, ok := s.Authentifier(r)
	assert.True(t, ok)
	assert.Equal(t, reponse.Joueur.ID, id)
}

func TestDeconnexionEtTicket(t *testing.T) {
	s, m, h := service(t, false)
	joueur := uuid.New()
	m.sessions["jeton"] = joueur

	w := executer(h, httptest.NewRequest(http.MethodPost, "/auth/ticket", nil))
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	r := httptest.NewRequest(http.MethodPost, "/auth/ticket", nil)
	r.AddCookie(&http.Cookie{Name: NomCookie, Value: "jeton"})
	w = executer(h, r)
	require.Equal(t, http.StatusOK, w.Code)
	var ticket struct {
		Ticket string `json:"ticket"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ticket))

	ws := httptest.NewRequest(http.MethodGet, "/ws?ticket="+ticket.Ticket, nil)
	id, ok := s.AuthentifierWebSocket(ws)
	assert.True(t, ok)
	assert.Equal(t, joueur, id)
	_, ok = s.AuthentifierWebSocket(ws)
	assert.False(t, ok, "un ticket ne sert qu'une fois")

	r = httptest.NewRequest(http.MethodPost, "/auth/deconnexion", nil)
	r.AddCookie(&http.Cookie{Name: NomCookie, Value: "jeton"})
	w = executer(h, r)
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.NotContains(t, m.sessions, "jeton")
	assert.Equal(t, -1, w.Result().Cookies()[0].MaxAge)
}

func TestCheminDeRetour(t *testing.T) {
	for entree, attendu := range map[string]string{
		"":                    "/",
		"/salon":              "/salon",
		"/partie/42?onglet=1": "/partie/42?onglet=1",
		"https://evil.test":   "/",
		"//evil.test":         "/",
		`/\evil.test`:         "/",
		"salon":               "/",
		"/%2F%2Fevil.test":    "/%2F%2Fevil.test",
	} {
		assert.Equal(t, attendu, cheminDeRetour(entree), entree)
	}
}

func discard() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func TestDiscord(t *testing.T) {
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			require.NoError(t, r.ParseForm())
			assert.Equal(t, "code-discord", r.Form.Get("code"))
			assert.Equal(t, "verif", r.Form.Get("code_verifier"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"acces","token_type":"Bearer","expires_in":3600}`))
		case "/moi":
			assert.Equal(t, "Bearer acces", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"id":"8008","username":"coincheur","global_name":"Le Coincheur"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer idp.Close()

	d := NouveauDiscord("id", "secret", "https://api.coinche.test/auth/discord/retour")
	d.config.Endpoint.TokenURL = idp.URL + "/token"
	d.urlMoi = idp.URL + "/moi"
	d.client = idp.Client()

	lien, err := d.URLAutorisation(context.Background(), "etat", "verif", "")
	require.NoError(t, err)
	assert.Contains(t, lien, "code_challenge=")
	assert.Contains(t, lien, "state=etat")

	id, err := d.Identifier(context.Background(), "code-discord", "verif", "")
	require.NoError(t, err)
	assert.Equal(t, donnees.Identite{Fournisseur: "discord", Sujet: "8008", Nom: "Le Coincheur"}, id)
}
