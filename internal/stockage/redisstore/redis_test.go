package redisstore

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/felix-marchenay/coinchenet-core/internal/matchmaking"
	"github.com/felix-marchenay/coinchenet-core/internal/protocole"
	"github.com/felix-marchenay/coinchenet-core/internal/table"
)

func nouveauRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	client, err := Ouvrir(t.Context(), "redis://"+mr.Addr()+"/0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	return mr, client
}

func TestOuvrir(t *testing.T) {
	_, err := Ouvrir(t.Context(), "pas une url")
	assert.Error(t, err)

	mr := miniredis.RunT(t)
	addr := mr.Addr()
	mr.Close()
	_, err = Ouvrir(t.Context(), "redis://"+addr)
	assert.Error(t, err)
}

func TestVerrous(t *testing.T) {
	mr, client := nouveauRedis(t)
	v := NouveauxVerrous(client)
	ctx := t.Context()

	ok, err := v.Prendre(ctx, "table:1", "i1", 30*time.Second)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = v.Prendre(ctx, "table:1", "i2", 30*time.Second)
	require.NoError(t, err)
	assert.False(t, ok, "un verrou détenu ne se prend pas")

	ok, err = v.Prolonger(ctx, "table:1", "i2", time.Minute)
	require.NoError(t, err)
	assert.False(t, ok, "seul le propriétaire prolonge")

	ok, err = v.Prolonger(ctx, "table:1", "i1", time.Minute)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, time.Minute, mr.TTL("coinche:verrou:table:1"))

	require.NoError(t, v.Liberer(ctx, "table:1", "i2"))
	assert.True(t, mr.Exists("coinche:verrou:table:1"), "seul le propriétaire libère")

	require.NoError(t, v.Liberer(ctx, "table:1", "i1"))
	ok, err = v.Prendre(ctx, "table:1", "i2", 30*time.Second)
	require.NoError(t, err)
	assert.True(t, ok)

	mr.FastForward(31 * time.Second)
	ok, err = v.Prendre(ctx, "table:1", "i1", 30*time.Second)
	require.NoError(t, err)
	assert.True(t, ok, "un verrou expiré se reprend")

	ok, err = v.Prolonger(ctx, "table:1", "i2", time.Minute)
	require.NoError(t, err)
	assert.False(t, ok, "l'ancien propriétaire a perdu le verrou")

	proprietaire, tenu, err := v.Proprietaire(ctx, "table:1")
	require.NoError(t, err)
	assert.True(t, tenu)
	assert.Equal(t, "i1", proprietaire)

	_, tenu, err = v.Proprietaire(ctx, "table:2")
	require.NoError(t, err)
	assert.False(t, tenu)
}

type recepteur struct {
	mu       sync.Mutex
	messages []string
}

func (r *recepteur) recevoir(b []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, string(b))
}

func (r *recepteur) lire() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.messages...)
}

func nouveauBus(t *testing.T, client *redis.Client) *Bus {
	t.Helper()
	ctx, annuler := context.WithCancel(context.Background())
	b := NouveauBus(client, nil)
	require.NoError(t, b.Demarrer(ctx))
	t.Cleanup(annuler)
	return b
}

func TestBusJoueur(t *testing.T) {
	_, client := nouveauRedis(t)
	b := nouveauBus(t, client)
	ctx := t.Context()
	joueur := uuid.New()

	var onglet1, onglet2 recepteur
	desabonner1, err := b.AbonnerJoueur(ctx, joueur, onglet1.recevoir)
	require.NoError(t, err)
	desabonner2, err := b.AbonnerJoueur(ctx, joueur, onglet2.recevoir)
	require.NoError(t, err)

	for _, m := range []string{"un", "deux", "trois"} {
		require.NoError(t, b.PublierJoueur(ctx, joueur, []byte(m)))
	}
	require.NoError(t, b.PublierJoueur(ctx, uuid.New(), []byte("autre joueur")))

	attendu := []string{"un", "deux", "trois"}
	require.Eventually(t, func() bool { return len(onglet2.lire()) == 3 }, time.Second, 5*time.Millisecond)
	require.Eventually(t, func() bool { return len(onglet1.lire()) == 3 }, time.Second, 5*time.Millisecond)
	assert.Equal(t, attendu, onglet1.lire(), "les messages arrivent dans l'ordre")
	assert.Equal(t, attendu, onglet2.lire())

	desabonner1()
	desabonner1()
	require.NoError(t, b.PublierJoueur(ctx, joueur, []byte("quatre")))
	require.Eventually(t, func() bool { return len(onglet2.lire()) == 4 }, time.Second, 5*time.Millisecond)
	assert.Len(t, onglet1.lire(), 3, "un onglet désabonné ne reçoit plus rien")

	desabonner2()
	require.Eventually(t, func() bool {
		n, _ := client.PubSubNumSub(ctx, canalJoueur(joueur)).Result()
		return n[canalJoueur(joueur)] == 0
	}, time.Second, 5*time.Millisecond, "le canal est quitté après le dernier désabonnement")
}

func TestBusTable(t *testing.T) {
	_, client := nouveauRedis(t)
	b := nouveauBus(t, client)
	ctx := t.Context()
	id := uuid.New()

	var mu sync.Mutex
	var recues []table.Commande
	desabonner, err := b.AbonnerTable(ctx, id, func(c table.Commande) {
		mu.Lock()
		defer mu.Unlock()
		recues = append(recues, c)
	})
	require.NoError(t, err)
	defer desabonner()

	require.NoError(t, client.Publish(ctx, canalTable(id), "{illisible").Err())
	cmd := table.Commande{Type: table.CmdJouer, Joueur: table.Joueur{ID: uuid.New(), Pseudo: "Alice"}, Connexion: "c1", Donnees: json.RawMessage(`{"carte":"V-coeur"}`)}
	require.NoError(t, b.PublierTable(ctx, id, cmd))

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(recues) == 1
	}, time.Second, 5*time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, cmd, recues[0], "la commande illisible est ignorée, la suivante arrive")
}

func TestBusNonDemarreOuFerme(t *testing.T) {
	_, client := nouveauRedis(t)
	b := NouveauBus(client, nil)

	_, err := b.AbonnerJoueur(t.Context(), uuid.New(), func([]byte) {})
	require.ErrorIs(t, err, ErrBusArrete)

	require.NoError(t, b.Demarrer(t.Context()))
	require.Error(t, b.Demarrer(t.Context()), "un bus ne démarre qu'une fois")
	require.NoError(t, b.Fermer())
	require.NoError(t, b.Fermer())

	_, err = b.AbonnerJoueur(t.Context(), uuid.New(), func([]byte) {})
	require.ErrorIs(t, err, ErrBusArrete)
}

func TestDiffuseur(t *testing.T) {
	_, client := nouveauRedis(t)
	b := nouveauBus(t, client)
	ctx := t.Context()
	joueur := uuid.New()

	var r recepteur
	desabonner, err := b.AbonnerJoueur(ctx, joueur, r.recevoir)
	require.NoError(t, err)
	defer desabonner()

	NouveauDiffuseur(b, nil).Envoyer(ctx, joueur, protocole.Message{Type: "partie.carte", Data: map[string]string{"carte": "A-pique"}})

	require.Eventually(t, func() bool { return len(r.lire()) == 1 }, time.Second, 5*time.Millisecond)
	assert.JSONEq(t, `{"type":"partie.carte","data":{"carte":"A-pique"}}`, r.lire()[0])
}

func TestAnnuaire(t *testing.T) {
	mr, client := nouveauRedis(t)
	a := NouvelAnnuaire(client)
	ctx := t.Context()
	joueur, t1, t2 := uuid.New(), uuid.New(), uuid.New()

	_, ok, err := a.TableDe(ctx, joueur)
	require.NoError(t, err)
	assert.False(t, ok)

	require.NoError(t, a.Associer(ctx, joueur, t1))
	assert.Equal(t, DureeAnnuaire, mr.TTL(cleAnnuaire(joueur)))
	require.NoError(t, a.Associer(ctx, joueur, t2))

	require.NoError(t, a.Dissocier(ctx, joueur, t1))
	table, ok, err := a.TableDe(ctx, joueur)
	require.NoError(t, err)
	assert.True(t, ok, "dissocier une ancienne table ne touche pas à la table courante")
	assert.Equal(t, t2, table)

	require.NoError(t, a.Dissocier(ctx, joueur, t2))
	_, ok, err = a.TableDe(ctx, joueur)
	require.NoError(t, err)
	assert.False(t, ok)

	require.NoError(t, mr.Set(cleAnnuaire(joueur), "pas-un-uuid"))
	_, _, err = a.TableDe(ctx, joueur)
	assert.Error(t, err)
}

func TestCodes(t *testing.T) {
	mr, client := nouveauRedis(t)
	c := NouveauxCodes(client)
	ctx := t.Context()
	id := uuid.New()

	ok, err := c.Reserver(ctx, "ABC123", id, time.Hour)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = c.Reserver(ctx, "abc123", uuid.New(), time.Hour)
	require.NoError(t, err)
	assert.False(t, ok, "un code déjà pris")

	table, ok, err := c.Resoudre(ctx, " abc 123 ")
	require.NoError(t, err)
	assert.True(t, ok, "insensible à la casse et aux espaces")
	assert.Equal(t, id, table)

	_, ok, err = c.Resoudre(ctx, "   ")
	require.NoError(t, err)
	assert.False(t, ok)

	require.NoError(t, c.Liberer(ctx, "Abc123", uuid.New()))
	_, ok, err = c.Resoudre(ctx, "ABC123")
	require.NoError(t, err)
	assert.True(t, ok, "une autre table ne libère pas ce code")

	require.NoError(t, c.Liberer(ctx, "Abc123", id))
	_, ok, err = c.Resoudre(ctx, "ABC123")
	require.NoError(t, err)
	assert.False(t, ok)

	_, err = c.Reserver(ctx, "XYZ999", id, time.Minute)
	require.NoError(t, err)
	mr.FastForward(2 * time.Minute)
	_, ok, err = c.Resoudre(ctx, "XYZ999")
	require.NoError(t, err)
	assert.False(t, ok, "le code expire")
}

func TestSessions(t *testing.T) {
	mr, client := nouveauRedis(t)
	s := NouvellesSessions(client, time.Hour)
	ctx := t.Context()
	joueur := uuid.New()

	jeton, err := s.Creer(ctx, joueur)
	require.NoError(t, err)
	assert.Len(t, jeton, 43, "32 octets en base64url sans padding")

	for _, cle := range mr.Keys() {
		assert.NotContains(t, cle, jeton, "le jeton n'est jamais stocké en clair")
		assert.True(t, strings.HasPrefix(cle, "coinche:session:"))
	}

	mr.FastForward(50 * time.Minute)
	id, ok, err := s.Resoudre(ctx, jeton)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, joueur, id)
	mr.FastForward(50 * time.Minute)
	_, ok, err = s.Resoudre(ctx, jeton)
	require.NoError(t, err)
	assert.True(t, ok, "l'expiration glisse à chaque utilisation")

	_, ok, err = s.Resoudre(ctx, "inconnu")
	require.NoError(t, err)
	assert.False(t, ok)
	_, ok, err = s.Resoudre(ctx, "")
	require.NoError(t, err)
	assert.False(t, ok)

	require.NoError(t, s.Supprimer(ctx, jeton))
	require.NoError(t, s.Supprimer(ctx, ""))
	_, ok, err = s.Resoudre(ctx, jeton)
	require.NoError(t, err)
	assert.False(t, ok)

	jeton, err = s.Creer(ctx, joueur)
	require.NoError(t, err)
	mr.FastForward(2 * time.Hour)
	_, ok, err = s.Resoudre(ctx, jeton)
	require.NoError(t, err)
	assert.False(t, ok, "une session inutilisée expire")
}

func TestTickets(t *testing.T) {
	mr, client := nouveauRedis(t)
	tk := NouveauxTickets(client, 30*time.Second)
	ctx := t.Context()
	joueur := uuid.New()

	ticket, err := tk.Emettre(ctx, joueur)
	require.NoError(t, err)

	id, ok, err := tk.Consommer(ctx, ticket)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, joueur, id)

	_, ok, err = tk.Consommer(ctx, ticket)
	require.NoError(t, err)
	assert.False(t, ok, "un ticket ne sert qu'une fois")

	ticket, err = tk.Emettre(ctx, joueur)
	require.NoError(t, err)
	mr.FastForward(time.Minute)
	_, ok, err = tk.Consommer(ctx, ticket)
	require.NoError(t, err)
	assert.False(t, ok, "un ticket expire")

	_, ok, err = tk.Consommer(ctx, "")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestEtatsOAuth(t *testing.T) {
	_, client := nouveauRedis(t)
	o := NouveauxEtatsOAuth(client)
	ctx := t.Context()
	e := EtatOAuth{Fournisseur: "google", Verifier: "v", Nonce: "n", Retour: "/profil"}

	require.NoError(t, o.Sauver(ctx, "state", e, time.Minute))

	lu, ok, err := o.Consommer(ctx, "state")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, e, lu)

	_, ok, err = o.Consommer(ctx, "state")
	require.NoError(t, err)
	assert.False(t, ok, "un état ne se consomme qu'une fois")

	_, ok, err = o.Consommer(ctx, "")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestFileMatchmaking(t *testing.T) {
	_, client := nouveauRedis(t)
	f := NouvelleFileMatchmaking(client)
	ctx := t.Context()

	_, ok, err := f.Recevoir(ctx, 50*time.Millisecond)
	require.NoError(t, err)
	assert.False(t, ok, "rien à lire : délai expiré")

	j := table.Joueur{ID: uuid.New(), Pseudo: "Alice", Elo: 1500}
	premiere := matchmaking.Commande{Type: matchmaking.CmdRejoindre, Joueur: j, Ref: "r1"}
	seconde := matchmaking.Commande{Type: matchmaking.CmdQuitter, Joueur: j}
	require.NoError(t, f.Publier(ctx, premiere))
	require.NoError(t, f.Publier(ctx, seconde))
	require.NoError(t, client.RPush(ctx, cleCommandesMatchmaking, "{illisible").Err())

	c, ok, err := f.Recevoir(ctx, time.Second)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, premiere, c, "ordre d'arrivée")

	c, ok, err = f.Recevoir(ctx, 100*time.Millisecond)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, seconde, c)

	_, ok, err = f.Recevoir(ctx, 100*time.Millisecond)
	require.NoError(t, err)
	assert.False(t, ok, "une commande illisible est écartée")

	annule, annuler := context.WithCancel(ctx)
	annuler()
	_, _, err = f.Recevoir(annule, time.Second)
	assert.Error(t, err)
}

func TestEtatMatchmaking(t *testing.T) {
	mr, client := nouveauRedis(t)
	e := NouvelEtatMatchmaking(client)
	ctx := t.Context()

	vide, err := e.Charger(ctx)
	require.NoError(t, err)
	assert.Equal(t, matchmaking.NouvelEtat(), vide)

	etat := matchmaking.NouvelEtat()
	j := table.Joueur{ID: uuid.New(), Pseudo: "Alice", Elo: 1500}
	etat.Tickets["t1"] = &matchmaking.Ticket{ID: "t1", Joueurs: []table.Joueur{j}, EntreeLe: time.Date(2026, 11, 16, 20, 0, 0, 0, time.UTC)}
	require.NoError(t, e.Sauver(ctx, etat))

	relu, err := e.Charger(ctx)
	require.NoError(t, err)
	assert.Equal(t, etat, relu)

	require.NoError(t, mr.Set(cleEtatMatchmaking, `{"tickets":null}`))
	relu, err = e.Charger(ctx)
	require.NoError(t, err)
	assert.NotNil(t, relu.Tickets)
	assert.NotNil(t, relu.Propositions)
	assert.NotNil(t, relu.Invitations)

	require.NoError(t, mr.Set(cleEtatMatchmaking, "{illisible"))
	_, err = e.Charger(ctx)
	assert.Error(t, err)
}
