package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/felix-marchenay/coinchenet-core/internal/bot"
	"github.com/felix-marchenay/coinchenet-core/internal/donnees"
	"github.com/felix-marchenay/coinchenet-core/internal/matchmaking"
	"github.com/felix-marchenay/coinchenet-core/internal/table"
)

const enteteJoueur = "X-Test-Joueur"

type fausseAuth struct{}

func (fausseAuth) Routes(*http.ServeMux) {}

func (fausseAuth) Authentifier(r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.Header.Get(enteteJoueur))
	return id, err == nil
}

func (f fausseAuth) AuthentifierWebSocket(r *http.Request) (uuid.UUID, bool) {
	return f.Authentifier(r)
}

type faux struct {
	mu        sync.Mutex
	joueurs   map[uuid.UUID]donnees.Joueur
	parties   map[uuid.UUID]donnees.ResumePartie
	tablesDe  map[uuid.UUID]uuid.UUID
	commandes []table.Commande
	mortes    map[uuid.UUID]bool
	file      []matchmaking.Commande
	abonnes   map[uuid.UUID]func([]byte)
	creerErr  error
}

func nouveauFaux() *faux {
	return &faux{
		joueurs:  map[uuid.UUID]donnees.Joueur{},
		parties:  map[uuid.UUID]donnees.ResumePartie{},
		tablesDe: map[uuid.UUID]uuid.UUID{},
		mortes:   map[uuid.UUID]bool{},
		abonnes:  map[uuid.UUID]func([]byte){},
	}
}

func (f *faux) JoueurParID(_ context.Context, id uuid.UUID) (donnees.Joueur, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, ok := f.joueurs[id]
	if !ok {
		return donnees.Joueur{}, donnees.ErrIntrouvable
	}
	return j, nil
}

func (f *faux) JoueurParPseudo(_ context.Context, pseudo string) (donnees.Joueur, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, j := range f.joueurs {
		if strings.EqualFold(j.Pseudo, pseudo) {
			return j, nil
		}
	}
	return donnees.Joueur{}, donnees.ErrIntrouvable
}

func (f *faux) TrouverOuCreer(context.Context, donnees.Identite) (donnees.Joueur, bool, error) {
	return donnees.Joueur{}, false, errors.New("non utilisé")
}

func (f *faux) Renommer(_ context.Context, id uuid.UUID, pseudo string) (donnees.Joueur, error) {
	if err := donnees.ValiderPseudo(pseudo); err != nil {
		return donnees.Joueur{}, err
	}
	if _, err := f.JoueurParPseudo(context.Background(), pseudo); err == nil {
		return donnees.Joueur{}, donnees.ErrPseudoPris
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	j := f.joueurs[id]
	j.Pseudo = pseudo
	f.joueurs[id] = j
	return j, nil
}

func (f *faux) Classement(_ context.Context, page, taille int) (donnees.PageClassement, error) {
	return donnees.PageClassement{Page: page, Taille: taille, Lignes: []donnees.LigneClassement{}}, nil
}

func (f *faux) PartiesDuJoueur(_ context.Context, _ uuid.UUID, _ *time.Time, limite int) ([]donnees.ResumePartie, error) {
	var res []donnees.ResumePartie
	for i := range limite {
		res = append(res, donnees.ResumePartie{ID: uuid.New(), Debut: time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC)})
	}
	return res, nil
}

func (f *faux) EvolutionElo(context.Context, uuid.UUID, int) ([]donnees.PointElo, error) {
	return []donnees.PointElo{{Elo: 1210, Delta: 10}}, nil
}

func (f *faux) Partie(_ context.Context, id uuid.UUID) (donnees.ResumePartie, error) {
	p, ok := f.parties[id]
	if !ok {
		return donnees.ResumePartie{}, donnees.ErrIntrouvable
	}
	return p, nil
}

func (f *faux) Ajouter(context.Context, uuid.UUID, []table.Evenement) error {
	return nil
}

func (f *faux) Charger(context.Context, uuid.UUID) ([]table.Evenement, error) {
	return []table.Evenement{{Sequence: 1, Type: table.EvtPartieCreee}}, nil
}

func (f *faux) CreerPrivee(_ context.Context, _ table.Joueur, _ table.Options) (uuid.UUID, string, error) {
	if f.creerErr != nil {
		return uuid.Nil, "", f.creerErr
	}
	return uuid.New(), "ABC234", nil
}

func (f *faux) Envoyer(_ context.Context, _ uuid.UUID, c table.Commande) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commandes = append(f.commandes, c)
	return nil
}

func (f *faux) ResoudreCode(_ context.Context, code string) (uuid.UUID, bool, error) {
	if code == "ABC234" {
		return uuid.MustParse("00000000-0000-0000-0000-00000000c0de"), true, nil
	}
	return uuid.Nil, false, nil
}

func (f *faux) Vivante(_ context.Context, t uuid.UUID) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.mortes[t], nil
}

func (f *faux) Dissocier(_ context.Context, joueur, t uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.tablesDe[joueur] == t {
		delete(f.tablesDe, joueur)
	}
	return nil
}

func (f *faux) TableDe(_ context.Context, joueur uuid.UUID) (uuid.UUID, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tablesDe[joueur]
	return t, ok, nil
}

type fausseFile struct{ *faux }

func (f fausseFile) Envoyer(_ context.Context, c matchmaking.Commande) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.file = append(f.file, c)
	return nil
}

func (f *faux) AbonnerJoueur(_ context.Context, joueur uuid.UUID, recevoir func([]byte)) (func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.abonnes[joueur] = recevoir
	return func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		delete(f.abonnes, joueur)
	}, nil
}

func (f *faux) publier(joueur uuid.UUID, b []byte) bool {
	f.mu.Lock()
	recevoir := f.abonnes[joueur]
	f.mu.Unlock()
	if recevoir == nil {
		return false
	}
	recevoir(b)
	return true
}

func (f *faux) dernieresCommandes() []table.Commande {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]table.Commande(nil), f.commandes...)
}

// commandesFile ignore les demandes d'état, envoyées à chaque connexion.
func (f *faux) commandesFile() []matchmaking.Commande {
	f.mu.Lock()
	defer f.mu.Unlock()
	var res []matchmaking.Commande
	for _, c := range f.file {
		if c.Type != matchmaking.CmdEtat {
			res = append(res, c)
		}
	}
	return res
}

func (f *faux) typesFile() []matchmaking.TypeCommande {
	f.mu.Lock()
	defer f.mu.Unlock()
	var res []matchmaking.TypeCommande
	for _, c := range f.file {
		res = append(res, c.Type)
	}
	return res
}

func (f *faux) ajouterJoueur(pseudo string) donnees.Joueur {
	j := donnees.Joueur{ID: uuid.New(), Pseudo: pseudo, Elo: 1234}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.joueurs[j.ID] = j
	return j
}

func serveurDeTest(f *faux, verifications ...Verification) *Serveur {
	return NouveauServeur(Dependances{
		Auth:          fausseAuth{},
		Joueurs:       f,
		Historique:    f,
		Journal:       f,
		Tables:        f,
		Annuaire:      f,
		File:          fausseFile{f},
		Bus:           f,
		Verifications: verifications,
		Origines:      []string{"https://coinche.test"},
		Version:       "1.2.3",
	})
}

func requete(t *testing.T, s http.Handler, methode, chemin string, joueur *donnees.Joueur, corps string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if corps == "" {
		r = httptest.NewRequest(methode, chemin, nil)
	} else {
		r = httptest.NewRequest(methode, chemin, strings.NewReader(corps))
		r.Header.Set("Content-Type", "application/json")
	}
	if joueur != nil {
		r.Header.Set(enteteJoueur, joueur.ID.String())
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func codeErreur(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var e struct {
		Erreur struct {
			Code string `json:"code"`
		} `json:"erreur"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &e), w.Body.String())
	return e.Erreur.Code
}

func TestRoutesPubliques(t *testing.T) {
	s := serveurDeTest(nouveauFaux())

	assert.Equal(t, http.StatusOK, requete(t, s, http.MethodGet, "/sante", nil, "").Code)
	assert.JSONEq(t, `{"version":"1.2.3"}`, requete(t, s, http.MethodGet, "/version", nil, "").Body.String())

	w := requete(t, s, http.MethodGet, "/regles", nil, "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"messages_predefinis"`)
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))

	w = requete(t, s, http.MethodGet, "/bots", nil, "")
	require.Equal(t, http.StatusOK, w.Code)
	var bots struct {
		Bots []map[string]any `json:"bots"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &bots))
	require.Len(t, bots.Bots, 4)
	assert.Equal(t, map[string]any{
		"id": "narvaloche", "famille": "Narvaloche", "style": "casse-cou", "description": bot.Narvaloche.Description,
	}, bots.Bots[3], "seule la présentation est publique, pas les réglages")
}

func TestPret(t *testing.T) {
	s := serveurDeTest(nouveauFaux(),
		Verification{Nom: "postgres", Verifier: func(context.Context) error { return nil }},
		Verification{Nom: "redis", Verifier: func(context.Context) error { return errors.New("injoignable") }},
	)
	w := requete(t, s, http.MethodGet, "/pret", nil, "")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.JSONEq(t, `{"postgres":"ok","redis":"injoignable"}`, w.Body.String())
}

func TestMoi(t *testing.T) {
	f := nouveauFaux()
	s := serveurDeTest(f)
	alice := f.ajouterJoueur("Alice")
	f.ajouterJoueur("Bob")

	assert.Equal(t, http.StatusUnauthorized, requete(t, s, http.MethodGet, "/moi", nil, "").Code)

	table := uuid.New()
	f.tablesDe[alice.ID] = table
	w := requete(t, s, http.MethodGet, "/moi", &alice, "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"table":"`+table.String()+`"`)

	r := httptest.NewRequest(http.MethodPatch, "/moi", strings.NewReader(`{"pseudo":"Alicia"}`))
	r.Header.Set(enteteJoueur, alice.ID.String())
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code, "un corps sans Content-Type JSON est refusé")

	w = requete(t, s, http.MethodPatch, "/moi", &alice, `{"pseudo":"Bob"}`)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, "pseudo_pris", codeErreur(t, w))

	w = requete(t, s, http.MethodPatch, "/moi", &alice, `{"pseudo":"a"}`)
	assert.Equal(t, "pseudo_invalide", codeErreur(t, w))

	w = requete(t, s, http.MethodPatch, "/moi", &alice, `{"pseudo":"Alicia","inconnu":1}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, "les champs inconnus sont refusés")

	w = requete(t, s, http.MethodPatch, "/moi", &alice, `{"pseudo":" Alicia "}`)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"pseudo":"Alicia"`)
}

func TestJoueurs(t *testing.T) {
	f := nouveauFaux()
	s := serveurDeTest(f)
	bob := f.ajouterJoueur("Bob")

	assert.Equal(t, http.StatusBadRequest, requete(t, s, http.MethodGet, "/joueurs", nil, "").Code)
	assert.Equal(t, http.StatusOK, requete(t, s, http.MethodGet, "/joueurs?pseudo=bob", nil, "").Code)
	assert.Equal(t, http.StatusNotFound, requete(t, s, http.MethodGet, "/joueurs?pseudo=personne", nil, "").Code)
	assert.Equal(t, http.StatusOK, requete(t, s, http.MethodGet, "/joueurs/"+bob.ID.String(), nil, "").Code)
	assert.Equal(t, http.StatusBadRequest, requete(t, s, http.MethodGet, "/joueurs/pas-un-uuid", nil, "").Code)
	assert.Equal(t, http.StatusNotFound, requete(t, s, http.MethodGet, "/joueurs/"+uuid.NewString(), nil, "").Code)

	w := requete(t, s, http.MethodGet, "/joueurs/"+bob.ID.String()+"/parties?limite=2", nil, "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"suivant":"2026-01-01T00:00:01Z"`, "une page pleine indique le curseur suivant")
	assert.Equal(t, http.StatusBadRequest, requete(t, s, http.MethodGet, "/joueurs/"+bob.ID.String()+"/parties?avant=hier", nil, "").Code)
	assert.Equal(t, http.StatusBadRequest, requete(t, s, http.MethodGet, "/joueurs/"+bob.ID.String()+"/parties?limite=0", nil, "").Code)
	assert.Equal(t, http.StatusOK, requete(t, s, http.MethodGet, "/joueurs/"+bob.ID.String()+"/elo", nil, "").Code)
}

func TestClassement(t *testing.T) {
	s := serveurDeTest(nouveauFaux())
	w := requete(t, s, http.MethodGet, "/classement?page=2&taille=10", nil, "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"lignes":[],"total":0,"page":2,"taille":10}`, w.Body.String())
	assert.Equal(t, http.StatusBadRequest, requete(t, s, http.MethodGet, "/classement?taille=1000", nil, "").Code)
}

func TestReplay(t *testing.T) {
	f := nouveauFaux()
	s := serveurDeTest(f)
	alice, bob := f.ajouterJoueur("Alice"), f.ajouterJoueur("Bob")
	enCours, privee, classee := uuid.New(), uuid.New(), uuid.New()
	f.parties[enCours] = donnees.ResumePartie{ID: enCours, Type: "classee", Statut: "en_cours"}
	f.parties[privee] = donnees.ResumePartie{ID: privee, Type: "privee", Statut: "terminee", Participants: []donnees.Participant{{JoueurID: &alice.ID}}}
	f.parties[classee] = donnees.ResumePartie{ID: classee, Type: "classee", Statut: "terminee"}

	assert.Equal(t, http.StatusOK, requete(t, s, http.MethodGet, "/parties/"+enCours.String(), nil, "").Code)
	assert.Equal(t, http.StatusForbidden, requete(t, s, http.MethodGet, "/parties/"+enCours.String()+"/evenements", &alice, "").Code, "pas de replay d'une partie en cours")
	assert.Equal(t, http.StatusForbidden, requete(t, s, http.MethodGet, "/parties/"+privee.String()+"/evenements", nil, "").Code)
	assert.Equal(t, http.StatusForbidden, requete(t, s, http.MethodGet, "/parties/"+privee.String()+"/evenements", &bob, "").Code)
	assert.Equal(t, http.StatusOK, requete(t, s, http.MethodGet, "/parties/"+privee.String()+"/evenements", &alice, "").Code)
	assert.Equal(t, http.StatusOK, requete(t, s, http.MethodGet, "/parties/"+classee.String()+"/evenements", nil, "").Code)
	assert.Equal(t, http.StatusNotFound, requete(t, s, http.MethodGet, "/parties/"+uuid.NewString()+"/evenements", nil, "").Code)
}

func TestCreerTable(t *testing.T) {
	f := nouveauFaux()
	s := serveurDeTest(f)
	alice := f.ajouterJoueur("Alice")

	assert.Equal(t, http.StatusUnauthorized, requete(t, s, http.MethodPost, "/tables", nil, `{}`).Code)
	assert.Equal(t, http.StatusBadRequest, requete(t, s, http.MethodPost, "/tables", &alice, `{"delai_action":45}`).Code)

	w := requete(t, s, http.MethodPost, "/tables", &alice, `{"regles":{"objectif":1500,"decompte":"contrat-seul","distribution":"aleatoire"},"delai_action":0}`)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"code":"ABC234"`)
	assert.Contains(t, w.Body.String(), `"objectif":1500`)

	assert.Contains(t, f.typesFile(), matchmaking.CmdQuitter, "créer une table fait quitter la file")

	f.tablesDe[alice.ID] = uuid.New()
	w = requete(t, s, http.MethodPost, "/tables", &alice, `{}`)
	assert.Equal(t, http.StatusConflict, w.Code)

	morte := uuid.New()
	f.tablesDe[alice.ID] = morte
	f.mortes[morte] = true
	w = requete(t, s, http.MethodPost, "/tables", &alice, `{}`)
	assert.Equal(t, http.StatusCreated, w.Code, "une association vers une table disparue est ignorée")
	_, encore := f.tablesDe[alice.ID]
	assert.False(t, encore, "et effacée")

	f.creerErr = errors.New("redis en panne")
	assert.Equal(t, http.StatusServiceUnavailable, requete(t, s, http.MethodPost, "/tables", &alice, `{}`).Code)
	f.creerErr = nil

	limite := false
	for range 20 {
		if requete(t, s, http.MethodPost, "/tables", &alice, `{}`).Code == http.StatusTooManyRequests {
			limite = true
			break
		}
	}
	assert.True(t, limite, "la création de tables est limitée en débit")
}

func TestCORS(t *testing.T) {
	s := serveurDeTest(nouveauFaux())

	r := httptest.NewRequest(http.MethodOptions, "/tables", nil)
	r.Header.Set("Origin", "https://coinche.test")
	r.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "https://coinche.test", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))

	r = httptest.NewRequest(http.MethodGet, "/sante", nil)
	r.Header.Set("Origin", "https://evil.test")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

func TestLimiteur(t *testing.T) {
	l := nouveauLimiteur(1, 2)
	assert.True(t, l.autoriser("a"))
	assert.True(t, l.autoriser("a"))
	assert.False(t, l.autoriser("a"))
	assert.True(t, l.autoriser("b"), "chaque clé a son propre quota")
}

func TestAdresseClient(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	assert.Equal(t, "10.0.0.1", adresseClient(r))
	r.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")
	assert.Equal(t, "203.0.113.7", adresseClient(r))
}

func TestRecuperationDePanique(t *testing.T) {
	s := serveurDeTest(nouveauFaux())
	h := s.recuperation(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boum") }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	body, _ := io.ReadAll(w.Body)
	assert.Contains(t, string(body), "interne")
}
