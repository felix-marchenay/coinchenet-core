//go:build integration

package application

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/felix-marchenay/coinchenet-core/internal/config"
	"github.com/felix-marchenay/coinchenet-core/internal/observabilite"
)

// Deux instances partagent Postgres et Redis : les joueurs sont répartis entre elles,
// ce qui vérifie aussi l'acheminement des messages d'une instance à l'autre.
func TestBoutEnBout(t *testing.T) {
	pgURL := baseTemporaire(t)
	redisURL := redisDeTest(t)
	a := demarrer(t, pgURL, redisURL, "e2e-1")
	b := demarrer(t, pgURL, redisURL, "e2e-2")

	t.Run("partie privée avec trois bots", func(t *testing.T) {
		alice := connecter(t, a, "Alice")
		var table struct {
			Table uuid.UUID `json:"table"`
			Code  string    `json:"code"`
		}
		alice.post(t, "/tables", `{"regles":{"objectif":500,"decompte":"faits-et-contrat","distribution":"traditionnelle"},"delai_action":0}`, http.StatusCreated, &table)

		ws := alice.ws(t, b)
		ws.envoyer(t, "table.rejoindre", map[string]string{"code": strings.ToLower(table.Code)})
		ws.attendre(t, "partie.etat")
		for _, s := range []int{1, 2, 3} {
			ws.envoyer(t, "table.bot", map[string]int{"siege": s})
		}
		ws.envoyer(t, "table.lancer", nil)

		fin := ws.jouerJusquALaFin(t)
		assert.Equal(t, "score", fin["cause"])

		var replay struct {
			Evenements []struct {
				Type string `json:"type"`
			} `json:"evenements"`
		}
		alice.get(t, "/parties/"+table.Table.String()+"/evenements", http.StatusOK, &replay)
		require.NotEmpty(t, replay.Evenements)
		assert.Equal(t, "partie_creee", replay.Evenements[0].Type)
		assert.Equal(t, "partie_terminee", replay.Evenements[len(replay.Evenements)-1].Type)

		intrus := connecter(t, a, "Intrus")
		intrus.get(t, "/parties/"+table.Table.String()+"/evenements", http.StatusForbidden, nil)
	})

	t.Run("partie classée entre quatre joueurs", func(t *testing.T) {
		noms := []string{"Nord", "Est", "Sud", "Ouest"}
		var clients []*client
		var sockets []*socket
		for i, nom := range noms {
			instance := a
			if i%2 == 1 {
				instance = b
			}
			c := connecter(t, instance, nom)
			clients = append(clients, c)
			sockets = append(sockets, c.ws(t, instance))
		}
		for _, s := range sockets {
			s.envoyer(t, "file.rejoindre", nil)
			s.attendre(t, "file.rejointe")
		}
		for _, s := range sockets {
			trouvee := s.attendre(t, "file.trouvee")
			s.envoyer(t, "file.accepter", map[string]any{"proposition": trouvee["proposition"]})
		}

		fins := make(chan map[string]any, len(sockets))
		for _, s := range sockets {
			go func() { fins <- s.jouerJusquALaFin(t) }()
		}
		for range sockets {
			select {
			case fin := <-fins:
				require.NotNil(t, fin)
				assert.Len(t, fin["elo"], 4)
			case <-time.After(2 * time.Minute):
				t.Fatal("la partie classée ne s'est pas terminée")
			}
		}

		for _, c := range clients {
			var profil struct {
				Elo       int `json:"elo"`
				NbParties int `json:"nb_parties"`
			}
			c.get(t, "/moi", http.StatusOK, &profil)
			assert.Equal(t, 1, profil.NbParties)
			assert.NotEqual(t, 1200, profil.Elo)
		}
	})

	// La table classée est exécutée par l'instance leader de la file (a) ; les joueurs sont sur b.
	// On arrête a en pleine partie : b doit la reprendre depuis le journal et la mener à son terme.
	t.Run("reprise d'une partie après l'arrêt d'une instance", func(t *testing.T) {
		var sockets []*socket
		for _, nom := range []string{"Reprise1", "Reprise2", "Reprise3", "Reprise4"} {
			s := connecter(t, b, nom).ws(t, b)
			s.tolerees = map[string]bool{"phase": true, "pas_ton_tour": true}
			sockets = append(sockets, s)
		}
		for _, s := range sockets {
			s.envoyer(t, "file.rejoindre", nil)
		}
		var table string
		for _, s := range sockets {
			trouvee := s.attendre(t, "file.trouvee")
			s.envoyer(t, "file.accepter", map[string]any{"proposition": trouvee["proposition"]})
		}
		for _, s := range sockets {
			table, _ = s.attendre(t, "partie.commencee")["table"].(string)
		}
		require.NotEmpty(t, table)
		proprietaire, err := redisDe(t, redisURL).Get(context.Background(), "coinche:verrou:table:"+table).Result()
		require.NoError(t, err)
		require.Equal(t, "e2e-1", proprietaire, "la table est exécutée par le leader de la file")

		fins := make(chan map[string]any, len(sockets))
		for _, s := range sockets {
			go func() { fins <- s.jouerJusquALaFin(t) }()
		}
		time.Sleep(300 * time.Millisecond)
		ctx, annuler := context.WithTimeout(context.Background(), 10*time.Second)
		defer annuler()
		require.NoError(t, a.app.Arreter(ctx))

		for range sockets {
			select {
			case fin := <-fins:
				require.NotNil(t, fin)
				assert.Equal(t, "score", fin["cause"], "personne n'est déclaré forfait")
			case <-time.After(2 * time.Minute):
				t.Fatal("la partie reprise ne s'est pas terminée")
			}
		}
		proprietaire, err = redisDe(t, redisURL).Get(context.Background(), "coinche:verrou:table:"+table).Result()
		if err == nil {
			assert.Equal(t, "e2e-2", proprietaire)
		}
	})
}

func redisDe(t *testing.T, u string) *redis.Client {
	t.Helper()
	opts, err := redis.ParseURL(u)
	require.NoError(t, err)
	c := redis.NewClient(opts)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

type instance struct {
	app *Application
	url string
}

func demarrer(t *testing.T, pgURL, redisURL, nom string) *instance {
	t.Helper()
	cfg := config.Config{
		Adresse:            "127.0.0.1:0",
		AdresseAdmin:       "127.0.0.1:0",
		PostgresURL:        pgURL,
		RedisURL:           redisURL,
		URLPublique:        "http://127.0.0.1",
		URLFront:           "http://127.0.0.1",
		Origines:           []string{"http://127.0.0.1"},
		DureeSession:       time.Hour,
		AuthDev:            true,
		Instance:           nom,
		MigrerAuDemarrage:  true,
		ArretGracieux:      5 * time.Second,
		DebitWS:            10000,
		DelaiActionClassee: 15,
		DelaiBot:           2 * time.Millisecond,
		DelaiDemarrage:     100 * time.Millisecond,
		DelaiEntreMenes:    5 * time.Millisecond,
		DelaiRetour:        30 * time.Second,
		DelaiLobby:         time.Minute,
		IntervalleReprise:  300 * time.Millisecond,
	}
	log := observabilite.NouveauLogger(os.Stderr, slog.LevelWarn, "texte").With("instance", nom)
	ctx, annuler := context.WithTimeout(context.Background(), 30*time.Second)
	defer annuler()
	app, err := Demarrer(ctx, cfg, log, "test")
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, annuler := context.WithTimeout(context.Background(), 10*time.Second)
		defer annuler()
		_ = app.Arreter(ctx)
	})
	return &instance{app: app, url: "http://" + app.Adresse()}
}

type client struct {
	instance *instance
	token    string
}

func connecter(t *testing.T, i *instance, pseudo string) *client {
	t.Helper()
	c := &client{instance: i}
	var reponse struct {
		Token string `json:"token"`
	}
	c.post(t, "/auth/dev", fmt.Sprintf(`{"pseudo":%q}`, pseudo), http.StatusOK, &reponse)
	c.token = reponse.Token
	return c
}

func (c *client) requete(t *testing.T, methode, chemin, corps string, statut int, v any) {
	t.Helper()
	req, err := http.NewRequest(methode, c.instance.url+chemin, strings.NewReader(corps))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	rep, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer rep.Body.Close()
	b, _ := io.ReadAll(rep.Body)
	require.Equal(t, statut, rep.StatusCode, "%s %s : %s", methode, chemin, b)
	if v != nil {
		require.NoError(t, json.Unmarshal(b, v))
	}
}

func (c *client) get(t *testing.T, chemin string, statut int, v any) {
	t.Helper()
	c.requete(t, http.MethodGet, chemin, "", statut, v)
}

func (c *client) post(t *testing.T, chemin, corps string, statut int, v any) {
	t.Helper()
	c.requete(t, http.MethodPost, chemin, corps, statut, v)
}

type message struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

type socket struct {
	conn     *websocket.Conn
	messages chan message
	moi      float64
	// tolerees : codes d'erreur attendus, par exemple une réponse à un tour périmé après une reprise.
	tolerees map[string]bool
}

func (c *client) ws(t *testing.T, i *instance) *socket {
	t.Helper()
	ctx, annuler := context.WithTimeout(context.Background(), 5*time.Second)
	defer annuler()
	u, _ := url.Parse(i.url)
	u.Scheme = "ws"
	u.Path = "/ws"
	conn, _, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + c.token}}})
	require.NoError(t, err)
	conn.SetReadLimit(1 << 20)
	s := &socket{conn: conn, messages: make(chan message, 1024), moi: -1}
	go func() {
		defer close(s.messages)
		for {
			_, b, err := conn.Read(context.Background())
			if err != nil {
				return
			}
			var m message
			if json.Unmarshal(b, &m) == nil {
				s.messages <- m
			}
		}
	}()
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "") })
	s.attendre(t, "bienvenue")
	return s
}

func (s *socket) envoyer(t *testing.T, typ string, data any) {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, json.NewEncoder(&b).Encode(map[string]any{"type": typ, "data": data}))
	ctx, annuler := context.WithTimeout(context.Background(), 5*time.Second)
	defer annuler()
	assert.NoError(t, s.conn.Write(ctx, websocket.MessageText, b.Bytes()))
}

func (s *socket) attendre(t *testing.T, typ string) map[string]any {
	t.Helper()
	delai := time.After(30 * time.Second)
	for {
		select {
		case m, ok := <-s.messages:
			require.True(t, ok, "connexion fermée en attendant %s", typ)
			if m.Type == "erreur" {
				t.Errorf("erreur reçue en attendant %s : %v", typ, m.Data)
			}
			if m.Type == typ {
				return m.Data
			}
		case <-delai:
			t.Fatalf("message %s non reçu", typ)
		}
	}
}

// jouerJusquALaFin joue pour le joueur : le premier à parler prend à 80 cœur, les autres passent,
// et chacun pose sa première carte jouable.
func (s *socket) jouerJusquALaFin(t *testing.T) map[string]any {
	delai := time.After(2 * time.Minute)
	for {
		select {
		case m, ok := <-s.messages:
			if !ok {
				t.Error("connexion fermée avant la fin de la partie")
				return nil
			}
			switch m.Type {
			case "erreur":
				if code, _ := m.Data["code"].(string); !s.tolerees[code] {
					t.Errorf("erreur reçue : %v", m.Data)
				}
			case "partie.etat":
				if moi, ok := m.Data["moi"].(float64); ok {
					s.moi = moi
				}
			case "partie.tour":
				s.repondre(t, m.Data)
			case "partie.terminee":
				return m.Data
			}
		case <-delai:
			t.Error("partie non terminée à temps")
			return nil
		}
	}
}

func (s *socket) repondre(t *testing.T, tour map[string]any) {
	if tour["siege"] != s.moi {
		return
	}
	if cartes, ok := tour["cartes_jouables"].([]any); ok && len(cartes) > 0 {
		s.envoyer(t, "partie.jouer", map[string]any{"carte": cartes[0]})
		return
	}
	if options, ok := tour["options_enchere"].(map[string]any); ok {
		if min, _ := options["annonce_min"].(float64); min == 80 {
			s.envoyer(t, "partie.encherir", map[string]any{"action": "annonce", "valeur": 80, "atout": "coeur"})
			return
		}
		s.envoyer(t, "partie.encherir", map[string]any{"action": "passe"})
	}
}

func baseTemporaire(t *testing.T) string {
	t.Helper()
	brute := os.Getenv("COINCHE_TEST_POSTGRES_URL")
	if brute == "" {
		t.Skip("COINCHE_TEST_POSTGRES_URL non définie")
	}
	u, err := url.Parse(brute)
	require.NoError(t, err)
	nom := "coinche_e2e_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	maintenance := *u
	maintenance.Path = "/postgres"

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, maintenance.String())
	require.NoError(t, err)
	_, err = conn.Exec(ctx, "create database "+nom)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), "drop database if exists "+nom+" with (force)")
		_ = conn.Close(context.Background())
	})
	u.Path = "/" + nom
	return u.String()
}

func redisDeTest(t *testing.T) string {
	t.Helper()
	u := os.Getenv("COINCHE_TEST_REDIS_URL")
	if u == "" {
		t.Skip("COINCHE_TEST_REDIS_URL non définie")
	}
	opts, err := redis.ParseURL(u)
	require.NoError(t, err)
	c := redis.NewClient(opts)
	defer c.Close()
	require.NoError(t, c.FlushDB(context.Background()).Err())
	return u
}
