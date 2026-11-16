package table

import (
	"context"
	"encoding/json"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
	"github.com/felix-marchenay/coinchenet-core/internal/protocole"
)

func optionsRapides() Options {
	return Options{Regles: coinche.Regles{Objectif: 500, Decompte: coinche.DecompteFaitsEtContrat, Distribution: coinche.DistributionTraditionnelle}, DelaiAction: 30}
}

func tablePriveeAvecBots(t *testing.T, env *environnement, createur Joueur) *Table {
	t.Helper()
	tb, err := NouvelleTablePrivee(uuid.New(), createur, "ABCDEF", optionsRapides(), env.deps, env.cfg)
	require.NoError(t, err)
	lancerActeur(t, tb)
	conn := "c-" + createur.Pseudo
	tb.Envoyer(commande(CmdConnexion, createur, conn, nil))
	for _, s := range []coinche.Siege{1, 2, 3} {
		tb.Envoyer(commande(CmdBot, createur, conn, DonneesSiege{Siege: s}))
	}
	tb.Envoyer(commande(CmdLancer, createur, conn, nil))
	synctest.Wait()
	return tb
}

type tableClassee struct {
	table   *Table
	joueurs [4]Joueur
	arreter func()
}

// tableClasseeConnectee : sièges 0 et 2 pour l'équipe A, 1 et 3 pour l'équipe B, tous connectés.
func tableClasseeConnectee(t *testing.T, env *environnement) tableClassee {
	t.Helper()
	a1, b1, a2, b2 := joueur("A1", 1500), joueur("B1", 1500), joueur("A2", 1500), joueur("B2", 1500)
	tb, err := NouvelleTableClassee(uuid.New(), [2]Joueur{a1, a2}, [2]Joueur{b1, b2}, OptionsClassees(), env.deps, env.cfg)
	require.NoError(t, err)
	tc := tableClassee{table: tb, joueurs: [4]Joueur{a1, b1, a2, b2}, arreter: lancerActeur(t, tb)}
	for _, j := range tc.joueurs {
		tb.Envoyer(commande(CmdConnexion, j, "c-"+j.Pseudo, nil))
	}
	synctest.Wait()
	return tc
}

func terminee(env *environnement, j Joueur) bool {
	return len(env.diffuseur.de(j.ID, MsgTerminee)) > 0
}

func TestPartiePriveeComplete(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := nouvelEnvironnement()
		alice := joueur("Alice", 1200)
		tb := tablePriveeAvecBots(t, env, alice)

		var etat EtatTable
		require.True(t, env.diffuseur.dernier(t, alice.ID, MsgEtat, &etat))
		assert.Equal(t, StatutEnCours, etat.Statut)
		assert.NotEmpty(t, env.diffuseur.de(alice.ID, MsgCommencee))

		var deja int
		for i := 0; !terminee(env, alice); i++ {
			require.Less(t, i, 20000, "la partie devrait se terminer")
			if repondreAuTour(t, env, tb, alice, 0, &deja) {
				synctest.Wait()
				continue
			}
			patienter(time.Second)
		}

		assert.Empty(t, env.diffuseur.codesErreur(alice.ID), "aucune action refusée")
		assert.NotEmpty(t, env.diffuseur.de(alice.ID, MsgDonne))
		assert.NotEmpty(t, env.diffuseur.de(alice.ID, MsgPli))
		assert.NotEmpty(t, env.diffuseur.de(alice.ID, MsgMene))

		fin := env.archiviste.derniereFin(t)
		assert.Equal(t, Privee, fin.Type)
		assert.Equal(t, CauseScore, fin.Cause)
		assert.GreaterOrEqual(t, fin.Scores[fin.Gagnante], 500)
		assert.Greater(t, fin.Scores[fin.Gagnante], fin.Scores[fin.Gagnante.Adverse()])

		_, associe := env.annuaire.table(alice.ID)
		assert.False(t, associe, "le joueur est libéré à la fin")

		types := env.journal.types(tb.ID())
		assert.Equal(t, EvtPartieCreee, types[0])
		assert.Equal(t, EvtDonne, types[1])
		assert.Equal(t, EvtPartieTerminee, types[len(types)-1])

		evts, err := env.journal.Charger(context.Background(), tb.ID())
		require.NoError(t, err)
		restauree, err := Restaurer(tb.ID(), evts, env.deps, env.cfg)
		require.NoError(t, err)
		assert.Equal(t, fin.Scores, restauree.partie.Scores, "le journal rejoue la partie à l'identique")
		assert.Equal(t, StatutTerminee, restauree.statut)

		patienter(env.cfg.DelaiFermeture)
		synctest.Wait()
		select {
		case <-tb.Arretee():
		default:
			t.Fatal("la table devrait être fermée après le délai de fermeture")
		}
	})
}

func TestLobby(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := nouvelEnvironnement()
		alice, bob, carole := joueur("Alice", 1200), joueur("Bob", 1200), joueur("Carole", 1200)
		tb, err := NouvelleTablePrivee(uuid.New(), alice, "ABCDEF", optionsRapides(), env.deps, env.cfg)
		require.NoError(t, err)
		lancerActeur(t, tb)

		tb.Envoyer(commande(CmdConnexion, alice, "a", nil))
		tb.Envoyer(commande(CmdConnexion, bob, "b", nil))
		synctest.Wait()

		var etat EtatTable
		require.True(t, env.diffuseur.dernier(t, alice.ID, MsgEtat, &etat))
		require.NotNil(t, etat.Sieges[1])
		assert.Equal(t, "Bob", etat.Sieges[1].Joueur.Pseudo, "Bob prend le premier siège libre")
		assert.Equal(t, StatutAttente, etat.Statut)
		assert.Equal(t, "ABCDEF", etat.Code)
		table, _ := env.annuaire.table(bob.ID)
		assert.Equal(t, tb.ID(), table)

		tb.Envoyer(commande(CmdSiege, bob, "b", DonneesSiege{Siege: 2}))
		tb.Envoyer(commande(CmdBot, bob, "b", DonneesSiege{Siege: 1}))
		tb.Envoyer(commande(CmdLancer, alice, "a", nil))
		tb.Envoyer(commande(CmdEnchere, alice, "a", DonneesEnchere{Action: coinche.Passe}))
		synctest.Wait()
		assert.Equal(t, []string{"interdit"}, env.diffuseur.codesErreur(bob.ID))
		assert.Equal(t, []string{"table_incomplete", "partie_non_commencee"}, env.diffuseur.codesErreur(alice.ID))

		require.True(t, env.diffuseur.dernier(t, alice.ID, MsgEtat, &etat))
		assert.Nil(t, etat.Sieges[1])
		assert.Equal(t, "Bob", etat.Sieges[2].Joueur.Pseudo)

		tb.Envoyer(commande(CmdBot, alice, "a", DonneesSiege{Siege: 1}))
		tb.Envoyer(commande(CmdConnexion, carole, "c", nil))
		tb.Envoyer(commande(CmdConnexion, joueur("Dan", 1200), "d", nil))
		synctest.Wait()
		require.True(t, env.diffuseur.dernier(t, alice.ID, MsgEtat, &etat))
		assert.True(t, etat.Sieges[1].Bot)
		assert.Equal(t, "Carole", etat.Sieges[3].Joueur.Pseudo)

		tb.Envoyer(commande(CmdLiberer, alice, "a", DonneesSiege{Siege: 3}))
		synctest.Wait()
		assert.NotEmpty(t, env.diffuseur.de(carole.ID, MsgExclu))
		_, associe := env.annuaire.table(carole.ID)
		assert.False(t, associe)

		tb.Envoyer(commande(CmdConnexion, carole, "c", nil))
		synctest.Wait()
		assert.Equal(t, []string{"exclu"}, env.diffuseur.codesErreur(carole.ID), "un joueur exclu ne revient pas")

		tb.Envoyer(commande(CmdQuitter, alice, "a", nil))
		synctest.Wait()
		require.True(t, env.diffuseur.dernier(t, bob.ID, MsgEtat, &etat))
		assert.Equal(t, bob.ID, *etat.Createur, "le créateur parti, Bob prend la main")

		tb.Envoyer(commande(CmdQuitter, bob, "b", nil))
		synctest.Wait()
		assert.NotEmpty(t, env.diffuseur.de(bob.ID, MsgFermee))
		<-tb.Arretee()
	})
}

func TestTableCompleteEtNonAssis(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := nouvelEnvironnement()
		alice := joueur("Alice", 1200)
		tb := tablePriveeAvecBots(t, env, alice)

		intrus := joueur("Intrus", 1200)
		tb.Envoyer(commande(CmdConnexion, intrus, "i", nil))
		tb.Envoyer(commande(CmdJouer, intrus, "i", DonneesJouer{Carte: coinche.Carte{Couleur: coinche.Coeur, Valeur: coinche.As}}))
		tb.Envoyer(commande(CmdEtat, intrus, "i", nil))
		synctest.Wait()

		assert.Equal(t, []string{"non_assis", "non_assis", "non_assis"}, env.diffuseur.codesErreur(intrus.ID))
	})
}

func TestLobbyFermeALArretDuServeur(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := nouvelEnvironnement()
		alice, bob := joueur("Alice", 1200), joueur("Bob", 1200)
		tb, err := NouvelleTablePrivee(uuid.New(), alice, "ABCDEF", optionsRapides(), env.deps, env.cfg)
		require.NoError(t, err)
		arreter := lancerActeur(t, tb)
		tb.Envoyer(commande(CmdConnexion, alice, "a", nil))
		tb.Envoyer(commande(CmdConnexion, bob, "b", nil))
		synctest.Wait()

		arreter()

		for _, j := range []Joueur{alice, bob} {
			var fermee MessageFermee
			require.True(t, env.diffuseur.dernier(t, j.ID, MsgFermee, &fermee))
			assert.Equal(t, "arret_serveur", fermee.Raison)
			_, associe := env.annuaire.table(j.ID)
			assert.False(t, associe, "un salon perdu ne bloque pas ses joueurs")
		}
	})
}

func TestLobbyExpire(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := nouvelEnvironnement()
		alice := joueur("Alice", 1200)
		tb, err := NouvelleTablePrivee(uuid.New(), alice, "ABCDEF", optionsRapides(), env.deps, env.cfg)
		require.NoError(t, err)
		lancerActeur(t, tb)

		patienter(env.cfg.DelaiLobby + time.Second)
		<-tb.Arretee()
		var fermee MessageFermee
		require.True(t, env.diffuseur.dernier(t, alice.ID, MsgFermee, &fermee))
		assert.Equal(t, "expiree", fermee.Raison)
	})
}

func TestCommandesRefusees(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := nouvelEnvironnement()
		tc := tableClasseeConnectee(t, env)
		patienter(env.cfg.DelaiDemarrage)
		synctest.Wait()

		var tour MessageTour
		require.True(t, env.diffuseur.dernier(t, tc.joueurs[0].ID, MsgTour, &tour))
		autre := tc.joueurs[tour.Siege.Suivant()]
		concerne := tc.joueurs[tour.Siege]

		tb := tc.table
		tb.Envoyer(commande(CmdEnchere, autre, "c-"+autre.Pseudo, DonneesEnchere{Action: coinche.Passe}))
		tb.Envoyer(commande(CmdJouer, concerne, "c-"+concerne.Pseudo, DonneesJouer{Carte: coinche.Carte{}}))
		tb.Envoyer(Commande{Type: CmdEnchere, Joueur: concerne, Connexion: "c-" + concerne.Pseudo, Donnees: json.RawMessage(`{"action":"tapis"}`)})
		tb.Envoyer(commande(CmdEnchere, concerne, "c-"+concerne.Pseudo, DonneesEnchere{Action: coinche.Annonce, Valeur: 85, Atout: new(coinche.AtoutPique)}))
		tb.Envoyer(commande("inconnue", concerne, "c-"+concerne.Pseudo, nil))
		tb.Envoyer(commande(CmdLancer, concerne, "c-"+concerne.Pseudo, nil))
		synctest.Wait()

		assert.Equal(t, []string{"pas_ton_tour"}, env.diffuseur.codesErreur(autre.ID))
		assert.Equal(t, []string{"phase", protocole.CodeRequeteInvalide, "enchere_invalide", protocole.CodeRequeteInvalide, "partie_deja_lancee"}, env.diffuseur.codesErreur(concerne.ID))
	})
}

func TestDelaiExpireEtAbsence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := nouvelEnvironnement()
		tc := tableClasseeConnectee(t, env)
		alice := tc.joueurs[0]

		var tour MessageTour
		patienter(env.cfg.DelaiDemarrage)
		synctest.Wait()
		require.True(t, env.diffuseur.dernier(t, alice.ID, MsgTour, &tour))
		require.NotNil(t, tour.Echeance, "en partie classée chaque action a une échéance")
		assert.Equal(t, 30*time.Second, time.Until(*tour.Echeance))

		// Personne ne joue : chacun voit ses délais expirer et devient absent.
		absents := map[coinche.Siege]bool{}
		for i := 0; len(absents) < 4; i++ {
			require.Less(t, i, 40, "tous les joueurs devraient finir absents")
			patienter(30 * time.Second)
			synctest.Wait()
			for _, data := range env.diffuseur.de(alice.ID, MsgStatut) {
				var st MessageStatut
				require.NoError(t, json.Unmarshal(data, &st))
				if st.Statut == SiegeAbsent {
					absents[st.Siege] = true
				}
			}
		}
		assert.NotEmpty(t, env.diffuseur.de(alice.ID, MsgCarte), "les bots jouent à la place des absents")

		tc.table.Envoyer(commande(CmdPresent, alice, "c-"+alice.Pseudo, nil))
		synctest.Wait()
		var st MessageStatut
		require.True(t, env.diffuseur.dernier(t, alice.ID, MsgStatut, &st))
		assert.Equal(t, MessageStatut{Entete: st.Entete, Siege: 0, Statut: SiegeConnecte}, st)
	})
}

type hasardNul struct{}

func (hasardNul) Intn(int) int {
	return 0
}

// paquetPour construit le paquet qui, distribué en 3-2-3 par ce donneur, donne exactement ces mains.
func paquetPour(donneur coinche.Siege, mains [coinche.NbSieges][]coinche.Carte) []coinche.Carte {
	var groupes [][]coinche.Carte
	var pos [coinche.NbSieges]int
	for _, nb := range []int{3, 2, 3} {
		s := donneur.Suivant()
		for range coinche.NbSieges {
			groupes = append(groupes, mains[s][pos[s]:pos[s]+nb])
			pos[s] += nb
			s = s.Suivant()
		}
	}
	var cartes []coinche.Carte
	for i := len(groupes) - 1; i >= 0; i-- {
		cartes = append(cartes, groupes[i]...)
	}
	return cartes
}

// Un joueur absent qui joue lui-même la carte qui lui fait remporter le pli garde la main :
// son tour suivant a une échéance et n'est pas confié au bot.
func TestAbsentQuiRejoueGardeLaMain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := nouvelEnvironnement()
		var mains [coinche.NbSieges][]coinche.Carte
		for _, c := range coinche.Couleurs {
			for _, v := range coinche.Valeurs {
				mains[c] = append(mains[c], coinche.Carte{Couleur: c, Valeur: v})
			}
		}
		env.deps.Hasard = hasardNul{}
		env.deps.Melangeur = coinche.FakeMelangeur{Cartes: paquetPour(0, mains)}
		env.deps.Coupeur = coinche.NoCoupeur{}

		a1, b1, a2, b2 := joueur("A1", 1500), joueur("B1", 1500), joueur("A2", 1500), joueur("B2", 1500)
		tb, err := NouvelleTableClassee(uuid.New(), [2]Joueur{a1, a2}, [2]Joueur{b1, b2}, OptionsClassees(), env.deps, env.cfg)
		require.NoError(t, err)
		tb.sieges[0].absent = true
		lancerActeur(t, tb)
		joueurs := [4]Joueur{a1, b1, a2, b2}
		for _, j := range joueurs {
			tb.Envoyer(commande(CmdConnexion, j, "c-"+j.Pseudo, nil))
		}

		// Le siège 0, absent, laisse le bot enchérir (il a tous les cœurs) ; les autres passent,
		// puis entament et fournissent. Le siège 0 coupe lui-même et remporte le pli.
		deja := map[coinche.Siege]int{}
		for i := 0; ; i++ {
			require.Less(t, i, 200)
			synctest.Wait()
			var tour MessageTour
			if env.diffuseur.dernier(t, a1.ID, MsgTour, &tour) && tour.Phase == coinche.PhaseJeu && tour.Siege == 0 {
				break
			}
			repondu := false
			for s := coinche.Siege(1); s < coinche.NbSieges; s++ {
				n := deja[s]
				repondu = repondreAuTour(t, env, tb, joueurs[s], s, &n) || repondu
				deja[s] = n
			}
			if !repondu {
				patienter(100 * time.Millisecond)
			}
		}

		var tour MessageTour
		require.True(t, env.diffuseur.dernier(t, a1.ID, MsgTour, &tour))
		require.NotEmpty(t, tour.CartesJouables)
		tb.Envoyer(commande(CmdJouer, a1, "c-A1", DonneesJouer{Carte: tour.CartesJouables[0]}))
		synctest.Wait()

		var pli map[string]any
		require.True(t, env.diffuseur.dernier(t, a1.ID, MsgPli, &pli))
		require.True(t, env.diffuseur.dernier(t, a1.ID, MsgTour, &tour))
		assert.Equal(t, coinche.Siege(0), tour.Siege, "le siège 0 a remporté le pli")
		assert.NotNil(t, tour.Echeance, "son tour suivant lui revient, avec une échéance")
	})
}

func TestForfaitApresDeconnexion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := nouvelEnvironnement()
		tc := tableClasseeConnectee(t, env)
		b1 := tc.joueurs[1]

		tc.table.Envoyer(commande(CmdDeconnexion, b1, "c-"+b1.Pseudo, nil))
		synctest.Wait()

		var st MessageStatut
		require.True(t, env.diffuseur.dernier(t, tc.joueurs[0].ID, MsgStatut, &st))
		assert.Equal(t, SiegeDeconnecte, st.Statut)
		require.NotNil(t, st.RetourAvant)
		assert.Equal(t, env.cfg.DelaiRetour, time.Until(*st.RetourAvant))

		patienter(env.cfg.DelaiRetour + time.Second)
		synctest.Wait()

		fin := env.archiviste.derniereFin(t)
		assert.Equal(t, CauseForfait, fin.Cause)
		assert.Equal(t, coinche.EquipeA, fin.Gagnante)
		assert.True(t, fin.Sieges[1].Abandon)

		var msg struct {
			FinPartie
		}
		require.True(t, env.diffuseur.dernier(t, tc.joueurs[0].ID, MsgTerminee, &msg))
		deltas := map[uuid.UUID]int{}
		for _, v := range msg.Elo {
			deltas[v.JoueurID] = v.Delta
		}
		assert.Equal(t, 20, deltas[tc.joueurs[0].ID])
		assert.Equal(t, -35, deltas[b1.ID], "défaite + pénalité d'abandon")
		assert.Equal(t, 0, deltas[tc.joueurs[3].ID], "le partenaire ne perd rien")
	})
}

func TestReconnexionAvantLeForfait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := nouvelEnvironnement()
		tc := tableClasseeConnectee(t, env)
		b1 := tc.joueurs[1]

		tc.table.Envoyer(commande(CmdDeconnexion, b1, "c-"+b1.Pseudo, nil))
		patienter(time.Minute)
		env.diffuseur.vider(b1.ID)
		tc.table.Envoyer(commande(CmdConnexion, b1, "nouvelle", nil))
		synctest.Wait()

		var etat EtatTable
		require.True(t, env.diffuseur.dernier(t, b1.ID, MsgEtat, &etat), "l'état complet est renvoyé à la reconnexion")
		require.NotNil(t, etat.Jeu)
		require.NotNil(t, etat.Moi)
		assert.Equal(t, coinche.Siege(1), *etat.Moi)
		assert.Equal(t, SiegeConnecte, etat.Sieges[1].Statut)

		patienter(env.cfg.DelaiRetour)
		synctest.Wait()
		assert.False(t, terminee(env, b1), "pas de forfait après une reconnexion")
	})
}

func TestAbandonVolontaire(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := nouvelEnvironnement()
		tc := tableClasseeConnectee(t, env)
		a2 := tc.joueurs[2]

		tc.table.Envoyer(commande(CmdAbandonner, a2, "c-"+a2.Pseudo, nil))
		synctest.Wait()

		fin := env.archiviste.derniereFin(t)
		assert.Equal(t, CauseForfait, fin.Cause)
		assert.Equal(t, coinche.EquipeB, fin.Gagnante)
		assert.Contains(t, env.journal.types(tc.table.ID()), EvtAbandon)
	})
}

func TestAbandonEnPartiePrivee(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := nouvelEnvironnement()
		alice, bob := joueur("Alice", 1200), joueur("Bob", 1200)
		tb, err := NouvelleTablePrivee(uuid.New(), alice, "ABCDEF", optionsRapides(), env.deps, env.cfg)
		require.NoError(t, err)
		lancerActeur(t, tb)
		tb.Envoyer(commande(CmdConnexion, alice, "a", nil))
		tb.Envoyer(commande(CmdConnexion, bob, "b", nil))
		tb.Envoyer(commande(CmdBot, alice, "a", DonneesSiege{Siege: 2}))
		tb.Envoyer(commande(CmdBot, alice, "a", DonneesSiege{Siege: 3}))
		tb.Envoyer(commande(CmdLancer, alice, "a", nil))
		synctest.Wait()

		tb.Envoyer(commande(CmdAbandonner, bob, "b", nil))
		synctest.Wait()
		assert.Empty(t, env.archiviste.fins, "en partie privée le bot reprend le siège")
		var st MessageStatut
		require.True(t, env.diffuseur.dernier(t, alice.ID, MsgStatut, &st))
		assert.Equal(t, SiegeAbandon, st.Statut)

		tb.Envoyer(commande(CmdAbandonner, alice, "a", nil))
		synctest.Wait()
		assert.Equal(t, CauseForfait, env.archiviste.derniereFin(t).Cause, "sans humain, la partie s'arrête")
	})
}

func TestRestaurationEnCoursDePartie(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := nouvelEnvironnement()
		tc := tableClasseeConnectee(t, env)
		patienter(5 * time.Minute)
		synctest.Wait()

		// Arrêt brutal de l'acteur, comme lors de la perte d'une instance.
		tc.arreter()
		original := tc.table

		evts, err := env.journal.Charger(context.Background(), original.ID())
		require.NoError(t, err)
		restauree, err := Restaurer(original.ID(), evts, env.deps, env.cfg)
		require.NoError(t, err)

		require.NotEmpty(t, original.partie.Menes)
		for _, s := range coinche.Sieges {
			assert.Equal(t, original.partie.Vue(s), restauree.partie.Vue(s), "siège %d", s)
		}
		assert.Equal(t, original.seq, restauree.seq)
	})
}

func TestRepriseDUneTable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := nouvelEnvironnement()
		tc := tableClasseeConnectee(t, env)
		patienter(2 * time.Minute)
		synctest.Wait()

		evts, err := env.journal.Charger(context.Background(), tc.table.ID())
		require.NoError(t, err)
		restauree, err := Restaurer(tc.table.ID(), evts, env.deps, env.cfg)
		require.NoError(t, err)
		for _, j := range tc.joueurs {
			env.diffuseur.vider(j.ID)
		}
		lancerActeur(t, restauree)
		synctest.Wait()

		for _, j := range tc.joueurs {
			assert.NotEmpty(t, env.diffuseur.de(j.ID, MsgCommencee), "chaque joueur est invité à se reconnecter")
			restauree.Envoyer(commande(CmdConnexion, j, "reprise-"+j.Pseudo, nil))
		}
		synctest.Wait()
		patienter(env.cfg.DelaiRetour + time.Minute)
		synctest.Wait()
		assert.False(t, terminee(env, tc.joueurs[0]), "les joueurs reconnectés ne sont pas forfaits")
	})
}

func TestRestaurerJournalIncoherent(t *testing.T) {
	env := nouvelEnvironnement()
	id := uuid.New()

	_, err := Restaurer(id, nil, env.deps, env.cfg)
	assert.ErrorAs(t, err, new(*ErrJournalIncoherent))

	creee, err := nouvelEvenement(1, EvtPartieCreee, PartieCreee{Type: Privee, Options: optionsRapides(), Sieges: [4]SiegeInitial{{Bot: true}, {Bot: true}, {Bot: true}, {Bot: true}}}, time.Now())
	require.NoError(t, err)
	carte, err := nouvelEvenement(2, EvtCarte, CarteJouee{Siege: 0, Carte: coinche.Carte{Couleur: coinche.Coeur, Valeur: coinche.As}}, time.Now())
	require.NoError(t, err)

	_, err = Restaurer(id, []Evenement{creee, carte}, env.deps, env.cfg)
	assert.ErrorAs(t, err, new(*ErrJournalIncoherent), "une carte jouée avant toute donne est incohérente")

	carte.Sequence = 3
	_, err = Restaurer(id, []Evenement{creee, carte}, env.deps, env.cfg)
	assert.ErrorAs(t, err, new(*ErrJournalIncoherent), "séquence discontinue")
}

func TestPanneDuJournal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := nouvelEnvironnement()
		alice := joueur("Alice", 1200)
		tb := tablePriveeAvecBots(t, env, alice)

		env.journal.casser(errPanne)
		patienter(time.Minute)
		<-tb.Arretee()
	})
}

func TestChat(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := nouvelEnvironnement()
		tc := tableClasseeConnectee(t, env)
		alice, bob := tc.joueurs[0], tc.joueurs[1]

		tc.table.Envoyer(commande(CmdChat, alice, "c-A1", DonneesChat{Texte: "j'ai le valet"}))
		tc.table.Envoyer(commande(CmdChat, alice, "c-A1", DonneesChat{Message: "inconnu"}))
		tc.table.Envoyer(commande(CmdChat, alice, "c-A1", DonneesChat{Message: "bonne_chance"}))
		synctest.Wait()

		assert.Equal(t, []string{"chat_restreint", "chat_invalide"}, env.diffuseur.codesErreur(alice.ID))
		var chat Chat
		require.True(t, env.diffuseur.dernier(t, bob.ID, MsgChat, &chat))
		assert.Equal(t, Chat{Siege: 0, Pseudo: "A1", Message: "bonne_chance"}, chat)

		for range 10 {
			tc.table.Envoyer(commande(CmdChat, bob, "c-B1", DonneesChat{Message: "gg"}))
		}
		synctest.Wait()
		assert.Contains(t, env.diffuseur.codesErreur(bob.ID), protocole.CodeTropDeRequetes)
		assert.Contains(t, env.journal.types(tc.table.ID()), EvtChat)
	})
}

func TestValiderChat(t *testing.T) {
	c, err := validerChat(Privee, DonneesChat{Texte: "  salut\x00 à tous \n"})
	require.NoError(t, err)
	assert.Equal(t, "salut à tous", c.Texte)

	_, err = validerChat(Privee, DonneesChat{Texte: "   "})
	assert.Error(t, err)

	long := make([]rune, 501)
	for i := range long {
		long[i] = 'é'
	}
	_, err = validerChat(Privee, DonneesChat{Texte: string(long)})
	assert.Error(t, err)
}

func TestOptions(t *testing.T) {
	assert.NoError(t, OptionsClassees().Valider())
	assert.NoError(t, OptionsParDefaut().Valider())

	o := OptionsParDefaut()
	o.DelaiAction = 45
	assert.Error(t, o.Valider())

	o = OptionsParDefaut()
	o.Regles.Objectif = 100
	assert.Error(t, o.Valider())
}

func TestGenererCode(t *testing.T) {
	code := GenererCode(coinche.HasardCrypto{})
	assert.Len(t, code, 6)
	for _, r := range code {
		assert.Contains(t, alphabetCode, string(r))
	}
}
