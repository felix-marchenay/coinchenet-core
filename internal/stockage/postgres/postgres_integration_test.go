//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/felix-marchenay/coinchenet-core/internal/classement"
	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
	"github.com/felix-marchenay/coinchenet-core/internal/donnees"
	"github.com/felix-marchenay/coinchenet-core/internal/table"
)

// baseDeTest crée une base temporaire migrée, supprimée en fin de test.
func baseDeTest(t *testing.T) *Base {
	t.Helper()
	brut := os.Getenv("COINCHE_TEST_POSTGRES_URL")
	if brut == "" {
		t.Skip("COINCHE_TEST_POSTGRES_URL non définie")
	}
	ctx := context.Background()

	u, err := url.Parse(brut)
	require.NoError(t, err)
	nom := fmt.Sprintf("coinche_test_%d_%08x", time.Now().Unix(), rand.Uint32())
	maintenance := *u
	maintenance.Path = "/postgres"
	admin, err := pgx.Connect(ctx, maintenance.String())
	require.NoError(t, err)
	_, err = admin.Exec(ctx, "create database "+nom)
	require.NoError(t, err)

	cible := *u
	cible.Path = "/" + nom
	b, err := Ouvrir(ctx, cible.String())
	require.NoError(t, err)
	t.Cleanup(func() {
		b.Fermer()
		_, err := admin.Exec(context.Background(), "drop database if exists "+nom+" with (force)")
		assert.NoError(t, err)
		_ = admin.Close(context.Background())
	})
	require.NoError(t, b.Migrer(ctx))
	return b
}

func nouveauJoueur(t *testing.T, b *Base, nom string) donnees.Joueur {
	t.Helper()
	j, cree, err := b.TrouverOuCreer(context.Background(), donnees.Identite{Fournisseur: "test", Sujet: uuid.NewString(), Nom: nom})
	require.NoError(t, err)
	require.True(t, cree)
	return j
}

func fixerElo(t *testing.T, b *Base, id uuid.UUID, elo, nbParties int) {
	t.Helper()
	_, err := b.pool.Exec(context.Background(), "update joueurs set elo = $2, elo_max = $2, nb_parties_classees = $3 where id = $1", id, elo, nbParties)
	require.NoError(t, err)
}

func evenement(t *testing.T, seq int64, typ table.TypeEvenement, donnees any) table.Evenement {
	t.Helper()
	b, err := json.Marshal(donnees)
	require.NoError(t, err)
	return table.Evenement{Sequence: seq, Type: typ, Donnees: b, Horodatage: time.Now().UTC().Truncate(time.Microsecond)}
}

func tableJoueur(j donnees.Joueur) *table.Joueur {
	return &table.Joueur{ID: j.ID, Pseudo: j.Pseudo, Elo: j.Elo}
}

func commencer(t *testing.T, b *Base, typ table.Type, sieges [4]table.SiegeInitial, debut time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	err := b.Commencer(context.Background(), table.Debut{
		PartieID:  id,
		Type:      typ,
		Options:   table.OptionsClassees(),
		Sieges:    sieges,
		Evenement: evenement(t, 1, table.EvtPartieCreee, map[string]any{"type": typ}),
		Date:      debut,
	})
	require.NoError(t, err)
	return id
}

func siegesHumains(js ...donnees.Joueur) [4]table.SiegeInitial {
	var s [4]table.SiegeInitial
	for i, j := range js {
		s[i] = table.SiegeInitial{Joueur: tableJoueur(j)}
	}
	return s
}

func fin(t *testing.T, partie uuid.UUID, typ table.Type, sieges [4]table.SiegeInitial, gagnante coinche.Equipe, abandon int, seq int64, date time.Time) table.Fin {
	t.Helper()
	f := table.Fin{
		PartieID:  partie,
		Type:      typ,
		Gagnante:  gagnante,
		Cause:     table.CauseScore,
		Scores:    [2]int{1020, 640},
		Evenement: evenement(t, seq, table.EvtPartieTerminee, map[string]any{"gagnante": gagnante}),
		Date:      date,
	}
	if gagnante == coinche.EquipeB {
		f.Scores = [2]int{640, 1020}
	}
	for s, si := range sieges {
		f.Sieges[s] = table.SiegeFinal{Joueur: si.Joueur, Bot: si.Bot, Abandon: s == abandon}
	}
	if abandon >= 0 {
		f.Cause = table.CauseForfait
	}
	return f
}

func TestMigrationsIdempotentes(t *testing.T) {
	b := baseDeTest(t)
	require.NoError(t, b.Migrer(context.Background()))

	var n int
	require.NoError(t, b.pool.QueryRow(context.Background(), "select count(*) from schema_migrations").Scan(&n))
	ms, err := lireMigrations()
	require.NoError(t, err)
	assert.Equal(t, len(ms), n)
}

func TestMigrationsConcurrentes(t *testing.T) {
	b := baseDeTest(t)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() { assert.NoError(t, b.Migrer(context.Background())) })
	}
	wg.Wait()
}

func TestJournal(t *testing.T) {
	b := baseDeTest(t)
	ctx := context.Background()
	alice := nouveauJoueur(t, b, "Alice")
	partie := commencer(t, b, table.Privee, [4]table.SiegeInitial{{Joueur: tableJoueur(alice)}, {Bot: true}, {Bot: true}, {Bot: true}}, time.Now())

	evts := []table.Evenement{
		evenement(t, 2, table.EvtDonne, map[string]any{"mene": 1}),
		evenement(t, 3, table.EvtEnchere, map[string]any{"siege": 1, "action": "passe"}),
	}
	require.NoError(t, b.Ajouter(ctx, partie, evts))
	require.NoError(t, b.Ajouter(ctx, partie, nil))

	lus, err := b.Charger(ctx, partie)
	require.NoError(t, err)
	require.Len(t, lus, 3)
	assert.Equal(t, table.EvtPartieCreee, lus[0].Type)
	for i, e := range lus[1:] {
		assert.Equal(t, evts[i].Sequence, e.Sequence)
		assert.Equal(t, evts[i].Type, e.Type)
		assert.JSONEq(t, string(evts[i].Donnees), string(e.Donnees))
		assert.True(t, evts[i].Horodatage.Equal(e.Horodatage))
	}

	err = b.Ajouter(ctx, partie, []table.Evenement{evenement(t, 4, table.EvtCarte, nil), evenement(t, 3, table.EvtCarte, nil)})
	assert.ErrorIs(t, err, ErrSequenceExistante)
	lus, err = b.Charger(ctx, partie)
	require.NoError(t, err)
	assert.Len(t, lus, 3, "un lot en conflit n'est pas écrit du tout")

	vide, err := b.Charger(ctx, uuid.New())
	require.NoError(t, err)
	assert.Empty(t, vide)
}

func TestPartieClassee(t *testing.T) {
	b := baseDeTest(t)
	ctx := context.Background()
	js := []donnees.Joueur{nouveauJoueur(t, b, "Nord"), nouveauJoueur(t, b, "Est"), nouveauJoueur(t, b, "Sud"), nouveauJoueur(t, b, "Ouest")}
	fixerElo(t, b, js[0].ID, 1500, 40)
	fixerElo(t, b, js[1].ID, 1400, 3)
	fixerElo(t, b, js[2].ID, 1300, 40)
	fixerElo(t, b, js[3].ID, 1450, 12)
	sieges := siegesHumains(js...)
	debut := time.Now().Add(-20 * time.Minute)
	partie := commencer(t, b, table.Classee, sieges, debut)

	enCours, err := b.PartiesEnCours(ctx)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{partie}, enCours)

	// L'ELO a changé depuis le début de la partie : c'est la valeur en base qui compte.
	fixerElo(t, b, js[0].ID, 1510, 41)

	f := fin(t, partie, table.Classee, sieges, coinche.EquipeA, -1, 2, time.Now())
	variations, err := b.Terminer(ctx, f)
	require.NoError(t, err)

	attendues := classement.Calculer([]classement.Participant{
		{JoueurID: js[0].ID, Equipe: coinche.EquipeA, Elo: 1510, NbParties: 41},
		{JoueurID: js[1].ID, Equipe: coinche.EquipeB, Elo: 1400, NbParties: 3},
		{JoueurID: js[2].ID, Equipe: coinche.EquipeA, Elo: 1300, NbParties: 40},
		{JoueurID: js[3].ID, Equipe: coinche.EquipeB, Elo: 1450, NbParties: 12},
	}, coinche.EquipeA)
	assert.Equal(t, attendues, variations)

	for i, j := range js {
		apres, err := b.JoueurParID(ctx, j.ID)
		require.NoError(t, err)
		assert.Equal(t, attendues[i].Apres, apres.Elo, j.Pseudo)
	}
	nord, _ := b.JoueurParID(ctx, js[0].ID)
	assert.Equal(t, 42, nord.NbParties)
	assert.Equal(t, 1, nord.NbVictoires)
	assert.Equal(t, max(1510, attendues[0].Apres), nord.EloMax)
	est, _ := b.JoueurParID(ctx, js[1].ID)
	assert.Equal(t, 0, est.NbVictoires)
	assert.Less(t, est.EloMax, 1401, "l'ELO max ne monte pas sur une défaite")

	resume, err := b.Partie(ctx, partie)
	require.NoError(t, err)
	assert.Equal(t, "terminee", resume.Statut)
	assert.Equal(t, "score", *resume.Cause)
	assert.Equal(t, 0, *resume.EquipeGagnante)
	assert.Equal(t, &[2]int{1020, 640}, resume.Scores)
	require.Len(t, resume.Participants, 4)
	assert.Equal(t, attendues[1].Avant, *resume.Participants[1].EloAvant)
	assert.Equal(t, attendues[1].Apres, *resume.Participants[1].EloApres)
	assert.JSONEq(t, `{"regles":{"objectif":1000,"decompte":"faits-et-contrat","distribution":"aleatoire"},"delai_action":30}`, string(resume.Options))

	encore, err := b.Terminer(ctx, fin(t, partie, table.Classee, sieges, coinche.EquipeB, -1, 3, time.Now()))
	require.NoError(t, err)
	assert.Equal(t, attendues, encore, "un second Terminer est sans effet")
	nord2, _ := b.JoueurParID(ctx, js[0].ID)
	assert.Equal(t, nord.Elo, nord2.Elo)
	evts, err := b.Charger(ctx, partie)
	require.NoError(t, err)
	assert.Len(t, evts, 2)

	enCours, err = b.PartiesEnCours(ctx)
	require.NoError(t, err)
	assert.Empty(t, enCours)
}

func TestPartieClasseeAbandon(t *testing.T) {
	b := baseDeTest(t)
	ctx := context.Background()
	js := []donnees.Joueur{nouveauJoueur(t, b, "Nord"), nouveauJoueur(t, b, "Est"), nouveauJoueur(t, b, "Sud"), nouveauJoueur(t, b, "Ouest")}
	sieges := siegesHumains(js...)
	partie := commencer(t, b, table.Classee, sieges, time.Now())

	variations, err := b.Terminer(ctx, fin(t, partie, table.Classee, sieges, coinche.EquipeA, 1, 2, time.Now()))
	require.NoError(t, err)

	require.Len(t, variations, 4)
	assert.Equal(t, -20-classement.PenaliteAbandon, variations[1].Delta)
	assert.Equal(t, 0, variations[3].Delta)
	resume, err := b.Partie(ctx, partie)
	require.NoError(t, err)
	assert.True(t, resume.Participants[1].Abandon)
	assert.False(t, resume.Participants[3].Abandon)
	assert.Equal(t, "forfait", *resume.Cause)
}

func TestPartiePrivee(t *testing.T) {
	b := baseDeTest(t)
	ctx := context.Background()
	alice, bob := nouveauJoueur(t, b, "Alice"), nouveauJoueur(t, b, "Bob")
	sieges := [4]table.SiegeInitial{
		{Joueur: tableJoueur(alice)},
		{Bot: true, Joueur: &table.Joueur{Pseudo: "Bot Bravo"}},
		{Joueur: tableJoueur(bob)},
		{Bot: true, Joueur: &table.Joueur{Pseudo: "Bot Delta"}},
	}
	partie := commencer(t, b, table.Privee, sieges, time.Now())

	variations, err := b.Terminer(ctx, fin(t, partie, table.Privee, sieges, coinche.EquipeB, 2, 2, time.Now()))
	require.NoError(t, err)
	assert.Nil(t, variations)

	resume, err := b.Partie(ctx, partie)
	require.NoError(t, err)
	assert.Equal(t, "privee", resume.Type)
	assert.Nil(t, resume.Participants[1].JoueurID)
	assert.True(t, resume.Participants[1].Bot)
	assert.Equal(t, "Bot Bravo", resume.Participants[1].Pseudo)
	assert.True(t, resume.Participants[2].Abandon)
	assert.Nil(t, resume.Participants[0].EloAvant)
	assert.True(t, resume.Participe(alice.ID))

	apres, err := b.JoueurParID(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, classement.EloInitial, apres.Elo, "une partie privée ne change pas l'ELO")
	assert.Equal(t, 0, apres.NbParties)

	encore, err := b.Terminer(ctx, fin(t, partie, table.Privee, sieges, coinche.EquipeB, -1, 3, time.Now()))
	require.NoError(t, err)
	assert.Nil(t, encore)
}

func TestTerminerPartieInconnue(t *testing.T) {
	b := baseDeTest(t)
	_, err := b.Terminer(context.Background(), table.Fin{PartieID: uuid.New()})
	assert.ErrorIs(t, err, donnees.ErrIntrouvable)
}

func TestReprise(t *testing.T) {
	b := baseDeTest(t)
	ctx := context.Background()
	alice := nouveauJoueur(t, b, "Alice")
	sieges := [4]table.SiegeInitial{{Joueur: tableJoueur(alice)}, {Bot: true}, {Bot: true}, {Bot: true}}
	ancienne := commencer(t, b, table.Privee, sieges, time.Now().Add(-time.Hour))
	recente := commencer(t, b, table.Privee, sieges, time.Now())

	ids, err := b.PartiesEnCours(ctx)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{ancienne, recente}, ids)

	require.NoError(t, b.Annuler(ctx, ancienne))
	require.NoError(t, b.Annuler(ctx, ancienne))
	ids, err = b.PartiesEnCours(ctx)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{recente}, ids)

	resume, err := b.Partie(ctx, ancienne)
	require.NoError(t, err)
	assert.Equal(t, "annulee", resume.Statut)
	assert.NotNil(t, resume.Fin)
}

func TestTrouverOuCreer(t *testing.T) {
	b := baseDeTest(t)
	ctx := context.Background()
	id := donnees.Identite{Fournisseur: "google", Sujet: "123", Email: "jean@example.com", Nom: "Jean Dupont"}

	j, cree, err := b.TrouverOuCreer(ctx, id)
	require.NoError(t, err)
	assert.True(t, cree)
	assert.Equal(t, "Jean_Dupont", j.Pseudo)
	assert.Equal(t, classement.EloInitial, j.Elo)
	assert.True(t, j.Provisoire)
	assert.Nil(t, j.Rang)

	encore, cree, err := b.TrouverOuCreer(ctx, id)
	require.NoError(t, err)
	assert.False(t, cree)
	assert.Equal(t, j.ID, encore.ID)

	homonyme, cree, err := b.TrouverOuCreer(ctx, donnees.Identite{Fournisseur: "discord", Sujet: "123", Nom: "jean dupont"})
	require.NoError(t, err)
	assert.True(t, cree)
	assert.NotEqual(t, j.ID, homonyme.ID)
	assert.Regexp(t, `^jean_dupont\d{4}$`, homonyme.Pseudo, "pseudo déjà pris (sans tenir compte de la casse)")
}

func TestTrouverOuCreerConcurrent(t *testing.T) {
	b := baseDeTest(t)
	id := donnees.Identite{Fournisseur: "google", Sujet: "concurrent", Nom: "Simultane"}

	var mu sync.Mutex
	ids := map[uuid.UUID]int{}
	creations := 0
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			j, cree, err := b.TrouverOuCreer(context.Background(), id)
			assert.NoError(t, err)
			mu.Lock()
			defer mu.Unlock()
			ids[j.ID]++
			if cree {
				creations++
			}
		})
	}
	wg.Wait()

	assert.Len(t, ids, 1, "une seule identité, un seul joueur")
	assert.Equal(t, 1, creations)
}

func TestJoueurParPseudo(t *testing.T) {
	b := baseDeTest(t)
	ctx := context.Background()
	j := nouveauJoueur(t, b, "Mathilde")

	trouve, err := b.JoueurParPseudo(ctx, "mAtHiLdE")
	require.NoError(t, err)
	assert.Equal(t, j.ID, trouve.ID)

	_, err = b.JoueurParPseudo(ctx, "Personne")
	assert.ErrorIs(t, err, donnees.ErrIntrouvable)
	_, err = b.JoueurParID(ctx, uuid.New())
	assert.ErrorIs(t, err, donnees.ErrIntrouvable)
}

func TestRenommer(t *testing.T) {
	b := baseDeTest(t)
	ctx := context.Background()
	alice, bob := nouveauJoueur(t, b, "Alice"), nouveauJoueur(t, b, "Bob")

	_, err := b.Renommer(ctx, alice.ID, "a b")
	assert.ErrorIs(t, err, donnees.ErrPseudoInvalide)

	_, err = b.Renommer(ctx, alice.ID, "BOB")
	assert.ErrorIs(t, err, donnees.ErrPseudoPris)

	_, err = b.Renommer(ctx, uuid.New(), "Charlie")
	assert.ErrorIs(t, err, donnees.ErrIntrouvable)

	renomme, err := b.Renommer(ctx, alice.ID, "Alice_2")
	require.NoError(t, err)
	assert.Equal(t, "Alice_2", renomme.Pseudo)

	memeNom, err := b.Renommer(ctx, bob.ID, "BoB")
	require.NoError(t, err, "changer la casse de son propre pseudo est permis")
	assert.Equal(t, "BoB", memeNom.Pseudo)
}

func TestClassement(t *testing.T) {
	b := baseDeTest(t)
	ctx := context.Background()
	elos := map[string][2]int{
		"Premier":    {1800, 50},
		"Second":     {1700, 5},
		"ExAequoA":   {1600, 20},
		"ExAequoB":   {1600, 30},
		"Dernier":    {1100, 11},
		"Provisoire": {2500, 4},
	}
	ids := map[string]uuid.UUID{}
	for nom, e := range elos {
		j := nouveauJoueur(t, b, nom)
		ids[nom] = j.ID
		fixerElo(t, b, j.ID, e[0], e[1])
	}

	page, err := b.Classement(ctx, 1, 3)
	require.NoError(t, err)
	assert.Equal(t, 5, page.Total, "les joueurs provisoires sont exclus")
	require.Len(t, page.Lignes, 3)
	assert.Equal(t, "Premier", page.Lignes[0].Pseudo)
	assert.Equal(t, 1, page.Lignes[0].Rang)
	assert.Equal(t, "Second", page.Lignes[1].Pseudo)
	assert.Equal(t, "ExAequoA", page.Lignes[2].Pseudo)
	assert.Equal(t, 3, page.Lignes[2].Rang)

	page, err = b.Classement(ctx, 2, 3)
	require.NoError(t, err)
	require.Len(t, page.Lignes, 2)
	assert.Equal(t, "ExAequoB", page.Lignes[0].Pseudo)
	assert.Equal(t, 3, page.Lignes[0].Rang, "même rang pour les ex æquo")
	assert.Equal(t, 5, page.Lignes[1].Rang)

	page, err = b.Classement(ctx, 9, 3)
	require.NoError(t, err)
	assert.Empty(t, page.Lignes)
	assert.NotNil(t, page.Lignes)

	page, err = b.Classement(ctx, 0, 1000)
	require.NoError(t, err)
	assert.Equal(t, 1, page.Page)
	assert.Equal(t, TailleClassementMax, page.Taille)

	exAequo, err := b.JoueurParID(ctx, ids["ExAequoB"])
	require.NoError(t, err)
	require.NotNil(t, exAequo.Rang)
	assert.Equal(t, 3, *exAequo.Rang)
	assert.False(t, exAequo.Provisoire)

	provisoire, err := b.JoueurParID(ctx, ids["Provisoire"])
	require.NoError(t, err)
	assert.Nil(t, provisoire.Rang)
	assert.True(t, provisoire.Provisoire)
}

func TestHistoriqueEtEvolutionElo(t *testing.T) {
	b := baseDeTest(t)
	ctx := context.Background()
	js := []donnees.Joueur{nouveauJoueur(t, b, "Nord"), nouveauJoueur(t, b, "Est"), nouveauJoueur(t, b, "Sud"), nouveauJoueur(t, b, "Ouest")}
	sieges := siegesHumains(js...)
	depart := time.Now().Add(-10 * time.Hour).UTC().Truncate(time.Second)

	var parties []uuid.UUID
	for i := range 5 {
		debut := depart.Add(time.Duration(i) * time.Hour)
		p := commencer(t, b, table.Classee, sieges, debut)
		gagnante := coinche.Equipe(i % 2)
		_, err := b.Terminer(ctx, fin(t, p, table.Classee, sieges, gagnante, -1, 2, debut.Add(30*time.Minute)))
		require.NoError(t, err)
		parties = append(parties, p)
	}
	enCours := commencer(t, b, table.Classee, sieges, depart.Add(9*time.Hour))
	privee := commencer(t, b, table.Privee, [4]table.SiegeInitial{{Joueur: tableJoueur(js[0])}, {Bot: true}, {Bot: true}, {Bot: true}}, depart.Add(8*time.Hour))
	_, err := b.Terminer(ctx, fin(t, privee, table.Privee, [4]table.SiegeInitial{{Joueur: tableJoueur(js[0])}, {Bot: true}, {Bot: true}, {Bot: true}}, coinche.EquipeA, -1, 2, depart.Add(9*time.Hour)))
	require.NoError(t, err)

	page1, err := b.PartiesDuJoueur(ctx, js[0].ID, nil, 4)
	require.NoError(t, err)
	require.Len(t, page1, 4)
	assert.Equal(t, privee, page1[0].ID, "la plus récente d'abord")
	assert.Equal(t, parties[4], page1[1].ID)
	assert.NotContains(t, []uuid.UUID{page1[0].ID, page1[1].ID, page1[2].ID, page1[3].ID}, enCours, "pas de partie en cours")
	for _, p := range page1 {
		assert.Len(t, p.Participants, 4)
	}

	avant := page1[3].Debut
	page2, err := b.PartiesDuJoueur(ctx, js[0].ID, &avant, 4)
	require.NoError(t, err)
	require.Len(t, page2, 2)
	assert.Equal(t, parties[1], page2[0].ID)
	assert.Equal(t, parties[0], page2[1].ID)

	aucune, err := b.PartiesDuJoueur(ctx, uuid.New(), nil, 10)
	require.NoError(t, err)
	assert.NotNil(t, aucune)
	assert.Empty(t, aucune)

	require.NoError(t, b.Ajouter(ctx, parties[4], []table.Evenement{
		evenement(t, 3, table.EvtMene, table.MeneTerminee{Mene: 1, Scores: [2]int{0, 160}}),
		evenement(t, 4, table.EvtMene, table.MeneTerminee{Mene: 2, Scores: [2]int{250, 160}}),
	}))
	classees, err := b.PartiesJouees(ctx, []string{string(table.Classee)}, nil, 10)
	require.NoError(t, err)
	require.Len(t, classees, 5, "ni la privée ni celle en cours")
	assert.Equal(t, parties[4], classees[0].ID)
	assert.Equal(t, [][2]int{{0, 160}, {250, 160}}, classees[0].Menes, "scores cumulés, dans l'ordre des mènes")
	assert.Equal(t, [][2]int{}, classees[1].Menes)
	assert.Len(t, classees[0].Participants, 4)
	toutes, err := b.PartiesJouees(ctx, nil, &classees[1].Debut, 10)
	require.NoError(t, err)
	assert.Len(t, toutes, 3, "tous types, avant le curseur")

	points, err := b.EvolutionElo(ctx, js[1].ID, 3)
	require.NoError(t, err)
	require.Len(t, points, 3, "les trois plus récentes, partie privée exclue")
	assert.Equal(t, parties[2], points[0].Partie, "ordre chronologique")
	assert.Equal(t, parties[4], points[2].Partie)
	est, err := b.JoueurParID(ctx, js[1].ID)
	require.NoError(t, err)
	assert.Equal(t, est.Elo, points[2].Elo)
	assert.True(t, points[0].Date.Before(points[1].Date))
	for _, pt := range points {
		assert.NotZero(t, pt.Delta)
	}

	vide, err := b.EvolutionElo(ctx, uuid.New(), 10)
	require.NoError(t, err)
	assert.NotNil(t, vide)
	assert.Empty(t, vide)

	_, err = b.Partie(ctx, uuid.New())
	assert.ErrorIs(t, err, donnees.ErrIntrouvable)
}

func TestPing(t *testing.T) {
	b := baseDeTest(t)
	assert.NoError(t, b.Ping(context.Background()))
}
