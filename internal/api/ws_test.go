package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/felix-marchenay/coinchenet-core/internal/donnees"
	"github.com/felix-marchenay/coinchenet-core/internal/matchmaking"
	"github.com/felix-marchenay/coinchenet-core/internal/table"
)

type clientWS struct {
	conn *websocket.Conn
}

func dialer(t *testing.T, srv *httptest.Server, joueur donnees.Joueur) *clientWS {
	t.Helper()
	ctx, annuler := context.WithTimeout(context.Background(), 5*time.Second)
	defer annuler()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: http.Header{enteteJoueur: {joueur.ID.String()}}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.CloseNow() })
	c := &clientWS{conn: conn}
	assert.Equal(t, MsgBienvenue, c.lire(t).Type)
	return c
}

func (c *clientWS) envoyer(t *testing.T, typ string, data any) {
	t.Helper()
	b, err := json.Marshal(map[string]any{"type": typ, "data": data, "ref": "r-" + typ})
	require.NoError(t, err)
	require.NoError(t, c.conn.Write(context.Background(), websocket.MessageText, b))
}

type recu struct {
	Type string         `json:"type"`
	Ref  string         `json:"ref"`
	Data map[string]any `json:"data"`
}

func (c *clientWS) lire(t *testing.T) recu {
	t.Helper()
	ctx, annuler := context.WithTimeout(context.Background(), 5*time.Second)
	defer annuler()
	_, b, err := c.conn.Read(ctx)
	require.NoError(t, err)
	var m recu
	require.NoError(t, json.Unmarshal(b, &m))
	return m
}

func (c *clientWS) erreur(t *testing.T) string {
	t.Helper()
	m := c.lire(t)
	require.Equal(t, "erreur", m.Type, "%+v", m)
	return m.Data["code"].(string)
}

func attendreCommandes(t *testing.T, f *faux, n int) []table.Commande {
	t.Helper()
	require.Eventually(t, func() bool { return len(f.dernieresCommandes()) >= n }, 5*time.Second, 5*time.Millisecond)
	return f.dernieresCommandes()
}

func TestWebSocketNonAuthentifie(t *testing.T) {
	srv := httptest.NewServer(serveurDeTest(nouveauFaux()))
	defer srv.Close()

	rep, err := http.Get(srv.URL + "/ws")
	require.NoError(t, err)
	defer rep.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, rep.StatusCode)
}

func TestWebSocketTable(t *testing.T) {
	f := nouveauFaux()
	srv := httptest.NewServer(serveurDeTest(f))
	defer srv.Close()
	alice := f.ajouterJoueur("Alice")
	enCours := uuid.New()
	f.tablesDe[alice.ID] = enCours

	c := dialer(t, srv, alice)

	cmds := attendreCommandes(t, f, 1)
	assert.Equal(t, table.CmdConnexion, cmds[0].Type, "reconnexion automatique à la table en cours")
	assert.Equal(t, alice.ID, cmds[0].Joueur.ID)
	connexion := cmds[0].Connexion
	assert.NotEmpty(t, connexion)

	c.envoyer(t, CliJouer, map[string]string{"carte": "V-coeur"})
	cmds = attendreCommandes(t, f, 2)
	assert.Equal(t, table.CmdJouer, cmds[1].Type)
	assert.Equal(t, connexion, cmds[1].Connexion)
	assert.Equal(t, "r-"+CliJouer, cmds[1].Ref)
	assert.JSONEq(t, `{"carte":"V-coeur"}`, string(cmds[1].Donnees))

	c.envoyer(t, CliTableRejoindre, map[string]string{"code": "ZZZZZZ"})
	assert.Equal(t, "introuvable", c.erreur(t))
	c.envoyer(t, CliTableRejoindre, map[string]string{"code": "ABC234"})
	assert.Equal(t, "deja_a_une_table", c.erreur(t))

	delete(f.tablesDe, alice.ID)
	c.envoyer(t, CliTableRejoindre, map[string]string{"code": "ABC234"})
	cmds = attendreCommandes(t, f, 4)
	assert.Equal(t, table.CmdDeconnexion, cmds[2].Type, "on quitte l'ancienne table")
	assert.Equal(t, table.CmdConnexion, cmds[3].Type)

	assert.Contains(t, f.typesFile(), matchmaking.CmdQuitter, "rejoindre une table privée fait quitter la file")

	nouvelle := uuid.New()
	b, _ := json.Marshal(map[string]any{"type": table.MsgCommencee, "data": table.MessageCommencee{Table: nouvelle, Type: table.Classee}})
	require.Eventually(t, func() bool { return f.publier(alice.ID, b) }, time.Second, 5*time.Millisecond)
	assert.Equal(t, table.MsgCommencee, c.lire(t).Type, "le message est relayé au client")
	cmds = attendreCommandes(t, f, 6)
	assert.Equal(t, table.CmdConnexion, cmds[5].Type, "et la connexion rejoint la nouvelle table")

	require.NoError(t, c.conn.Close(websocket.StatusNormalClosure, ""))
	cmds = attendreCommandes(t, f, 7)
	assert.Equal(t, table.CmdDeconnexion, cmds[6].Type, "la fermeture prévient la table")
	require.Eventually(t, func() bool {
		ts := f.typesFile()
		return len(ts) > 0 && ts[len(ts)-1] == matchmaking.CmdDeconnexion
	}, 5*time.Second, 5*time.Millisecond, "et la file classée")
}

func TestWebSocketTablePerimee(t *testing.T) {
	f := nouveauFaux()
	srv := httptest.NewServer(serveurDeTest(f))
	defer srv.Close()
	alice := f.ajouterJoueur("Alice")
	morte := uuid.New()
	f.tablesDe[alice.ID] = morte
	f.mortes[morte] = true

	c := dialer(t, srv, alice)
	require.Eventually(t, func() bool { return len(f.typesFile()) > 0 }, time.Second, 5*time.Millisecond)
	assert.Equal(t, []matchmaking.TypeCommande{matchmaking.CmdEtat}, f.typesFile(), "l'état de file est demandé à la connexion")
	assert.Empty(t, f.dernieresCommandes(), "pas de reconnexion vers une table disparue")

	c.envoyer(t, CliFileRejoindre, nil)
	require.Eventually(t, func() bool { return len(f.commandesFile()) == 1 }, time.Second, 5*time.Millisecond)
	assert.Equal(t, matchmaking.CmdRejoindre, f.commandesFile()[0].Type, "le joueur n'est plus bloqué")
}

func TestWebSocketConnexionsParJoueur(t *testing.T) {
	f := nouveauFaux()
	srv := httptest.NewServer(serveurDeTest(f))
	defer srv.Close()
	alice := f.ajouterJoueur("Alice")
	for range connexionsMaxParJoueur {
		dialer(t, srv, alice)
	}
	ctx, annuler := context.WithTimeout(context.Background(), 5*time.Second)
	defer annuler()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	_, rep, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: http.Header{enteteJoueur: {alice.ID.String()}}})
	require.Error(t, err)
	assert.Equal(t, http.StatusTooManyRequests, rep.StatusCode)
}

func TestWebSocketMessagesInvalides(t *testing.T) {
	f := nouveauFaux()
	srv := httptest.NewServer(serveurDeTest(f))
	defer srv.Close()
	c := dialer(t, srv, f.ajouterJoueur("Alice"))

	c.envoyer(t, CliPing, nil)
	pong := c.lire(t)
	assert.Equal(t, MsgPong, pong.Type)
	assert.Equal(t, "r-"+CliPing, pong.Ref)

	c.envoyer(t, "danser", nil)
	assert.Equal(t, "requete_invalide", c.erreur(t))

	c.envoyer(t, CliJouer, map[string]string{"carte": "V-coeur"})
	assert.Equal(t, "aucune_table", c.erreur(t))

	require.NoError(t, c.conn.Write(context.Background(), websocket.MessageText, []byte("pas du json")))
	assert.Equal(t, "requete_invalide", c.erreur(t))

	c.envoyer(t, CliTableRejoindre, map[string]string{})
	assert.Equal(t, "requete_invalide", c.erreur(t))
}

func TestWebSocketFile(t *testing.T) {
	f := nouveauFaux()
	srv := httptest.NewServer(serveurDeTest(f))
	defer srv.Close()
	alice := f.ajouterJoueur("Alice")
	bob := f.ajouterJoueur("Bob")
	c := dialer(t, srv, alice)

	f.mu.Lock()
	j := f.joueurs[alice.ID]
	j.Elo = 1500
	f.joueurs[alice.ID] = j
	f.mu.Unlock()

	c.envoyer(t, CliFileRejoindre, nil)
	require.Eventually(t, func() bool { return len(f.commandesFile()) == 1 }, time.Second, 5*time.Millisecond)
	cmd := f.commandesFile()[0]
	assert.Equal(t, matchmaking.CmdRejoindre, cmd.Type)
	assert.Equal(t, 1500, cmd.Joueur.Elo, "l'ELO est relu avant d'entrer en file")

	c.envoyer(t, CliDuoInviter, map[string]string{"pseudo": "personne"})
	assert.Equal(t, "introuvable", c.erreur(t))

	c.envoyer(t, CliDuoInviter, map[string]string{"pseudo": "bob"})
	c.envoyer(t, CliFileAccepter, map[string]string{"proposition": "p1"})
	require.Eventually(t, func() bool { return len(f.commandesFile()) == 3 }, time.Second, 5*time.Millisecond)
	assert.Equal(t, bob.ID, f.commandesFile()[1].Invite.ID)
	assert.Equal(t, "p1", f.commandesFile()[2].Proposition)

	f.mu.Lock()
	f.tablesDe[alice.ID] = uuid.New()
	f.mu.Unlock()
	c.envoyer(t, CliFileRejoindre, nil)
	assert.Equal(t, "deja_a_une_table", c.erreur(t))
	c.envoyer(t, CliFileAccepter, map[string]string{"proposition": "p2"})
	assert.Equal(t, "deja_a_une_table", c.erreur(t), "pas de partie classée depuis une table privée")
}

func TestWebSocketDebit(t *testing.T) {
	f := nouveauFaux()
	srv := httptest.NewServer(serveurDeTest(f))
	defer srv.Close()
	c := dialer(t, srv, f.ajouterJoueur("Alice"))

	for range 40 {
		c.envoyer(t, CliPing, nil)
	}
	limite := false
	for range 40 {
		if m := c.lire(t); m.Type == "erreur" && m.Data["code"] == "trop_de_requetes" {
			limite = true
			break
		}
	}
	assert.True(t, limite)
}

func TestWebSocketClientLent(t *testing.T) {
	f := nouveauFaux()
	s := serveurDeTest(f)
	srv := httptest.NewServer(s)
	defer srv.Close()
	alice := f.ajouterJoueur("Alice")
	c := dialer(t, srv, alice)

	// Le client ne lit plus rien : au-delà du tampon, la connexion est coupée.
	require.Eventually(t, func() bool { return f.publier(alice.ID, []byte(`{"type":"x"}`)) }, time.Second, 5*time.Millisecond)
	gros := []byte(`{"type":"partie.etat","data":"` + strings.Repeat("x", 64<<10) + `"}`)
	for range 2000 {
		if !f.publier(alice.ID, gros) {
			break
		}
	}
	require.Eventually(t, func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.abonnes[alice.ID] == nil
	}, 10*time.Second, 10*time.Millisecond, "la connexion d'un client trop lent est fermée")
	_ = c
	s.Fermer()
}
