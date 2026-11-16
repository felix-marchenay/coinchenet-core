package coinche

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func meneDeTest(t *testing.T, donneur Siege) *Mene {
	t.Helper()
	return nouvelleMene(1, donneur, FischerYatesMelangeur{Hasard: NouveauHasardDeterministe(7)}.Melanger(NouveauPaquet32()))
}

func encherir(t *testing.T, m *Mene, s Siege, e Enchere) IssueEnchere {
	t.Helper()
	issue, err := m.Encherir(s, e)
	require.NoError(t, err)
	return issue
}

func TestOrdreDeParole(t *testing.T) {
	m := meneDeTest(t, 3)

	assert.Equal(t, Siege(0), m.Tour, "le joueur après le donneur parle en premier")

	_, err := m.Encherir(1, NouvellePasse())
	assert.ErrorIs(t, err, ErrPasSonTour)

	encherir(t, m, 0, NouvellePasse())
	assert.Equal(t, Siege(1), m.Tour)
}

func TestToutLeMondePasse(t *testing.T) {
	m := meneDeTest(t, 0)

	for _, s := range []Siege{1, 2, 3} {
		issue := encherir(t, m, s, NouvellePasse())
		assert.False(t, issue.Redistribution)
	}
	issue := encherir(t, m, 0, NouvellePasse())

	assert.True(t, issue.Redistribution)
	assert.True(t, m.Terminee())
	require.NotNil(t, m.Resultat)
	assert.True(t, m.Resultat.Redistribuee)
}

func TestAnnonceSuivieDeTroisPasses(t *testing.T) {
	m := meneDeTest(t, 0)

	encherir(t, m, 1, NouvellePasse())
	encherir(t, m, 2, NouvelleAnnonce(AtoutPique, A80))
	encherir(t, m, 3, NouvellePasse())
	encherir(t, m, 0, NouvellePasse())
	issue := encherir(t, m, 1, NouvellePasse())

	assert.True(t, issue.EncheresTerminees)
	assert.Equal(t, PhaseJeu, m.Phase)
	assert.Equal(t, &Contrat{AtoutPique, A80}, m.Contrat)
	assert.Equal(t, Siege(2), m.Preneur)
	assert.Equal(t, 1, m.Multiplicateur)
	assert.Equal(t, Siege(1), m.Tour, "le joueur après le donneur entame")
	assert.Equal(t, Siege(1), m.PliEnCours.Entame)
}

func TestAnnonceDoitEtreSuperieure(t *testing.T) {
	m := meneDeTest(t, 0)
	encherir(t, m, 1, NouvelleAnnonce(AtoutCoeur, A100))

	_, err := m.Encherir(2, NouvelleAnnonce(AtoutPique, A100))
	assert.ErrorIs(t, err, ErrEnchereInvalide)

	_, err = m.Encherir(2, NouvelleAnnonce(AtoutPique, A90))
	assert.ErrorIs(t, err, ErrEnchereInvalide)

	encherir(t, m, 2, NouvelleAnnonce(SansAtout, A110))
	assert.Equal(t, Siege(2), m.Preneur)
}

func TestValeurAnnonceInvalide(t *testing.T) {
	m := meneDeTest(t, 0)

	for _, v := range []ValeurAnnonce{0, 70, 85, 170, 300} {
		_, err := m.Encherir(1, NouvelleAnnonce(AtoutCoeur, v))
		assert.ErrorIs(t, err, ErrEnchereInvalide, "valeur %d", v)
	}
	_, err := m.Encherir(1, Enchere{Type: Annonce})
	assert.ErrorIs(t, err, ErrEnchereInvalide, "une annonce sans contrat est refusée")
}

func TestUnJoueurQuiAPasseReparle(t *testing.T) {
	m := meneDeTest(t, 0)

	encherir(t, m, 1, NouvellePasse())
	encherir(t, m, 2, NouvelleAnnonce(AtoutCoeur, A80))
	encherir(t, m, 3, NouvellePasse())
	encherir(t, m, 0, NouvellePasse())
	encherir(t, m, 1, NouvelleAnnonce(AtoutPique, A90))

	assert.Equal(t, PhaseEncheres, m.Phase)
	assert.Equal(t, Siege(1), m.Preneur)
}

func TestCapotEtGenerale(t *testing.T) {
	m := meneDeTest(t, 0)

	encherir(t, m, 1, NouvelleAnnonce(AtoutCoeur, A160))
	o, _ := m.OptionsEnchere(2)
	assert.Equal(t, Capot, o.AnnonceMin)

	encherir(t, m, 2, NouvelleAnnonce(AtoutCoeur, Capot))
	encherir(t, m, 3, NouvelleAnnonce(ToutAtout, Generale))

	o, _ = m.OptionsEnchere(0)
	assert.Zero(t, o.AnnonceMin, "plus rien au-dessus de la générale")
	_, err := m.Encherir(0, NouvelleAnnonce(ToutAtout, Generale))
	assert.ErrorIs(t, err, ErrEnchereInvalide)
}

func TestCoincheParUnAdversaireSeulement(t *testing.T) {
	m := meneDeTest(t, 0)
	encherir(t, m, 1, NouvelleAnnonce(AtoutCoeur, A80))

	o, _ := m.OptionsEnchere(2)
	assert.True(t, o.Coinche)

	encherir(t, m, 2, NouvellePasse())
	o, _ = m.OptionsEnchere(3)
	assert.False(t, o.Coinche, "le partenaire du preneur ne peut pas coincher")
	_, err := m.Encherir(3, Enchere{Type: Coinche})
	assert.ErrorIs(t, err, ErrEnchereInvalide)
}

func TestCoincheSansContrat(t *testing.T) {
	m := meneDeTest(t, 0)
	_, err := m.Encherir(1, Enchere{Type: Coinche})
	assert.ErrorIs(t, err, ErrEnchereInvalide)
}

func TestCoincheSansSurcoinche(t *testing.T) {
	m := meneDeTest(t, 0)
	encherir(t, m, 1, NouvelleAnnonce(AtoutCoeur, A80))
	encherir(t, m, 2, Enchere{Type: Coinche})

	assert.Equal(t, 2, m.Multiplicateur)
	assert.Equal(t, Siege(3), m.Tour, "la parole passe à l'équipe preneuse")

	o, _ := m.OptionsEnchere(3)
	assert.Equal(t, OptionsEnchere{Passe: true, Surcoinche: true}, o)

	_, err := m.Encherir(3, NouvelleAnnonce(AtoutCoeur, A90))
	assert.ErrorIs(t, err, ErrEnchereInvalide, "plus de surenchère après une coinche")

	encherir(t, m, 3, NouvellePasse())
	assert.Equal(t, Siege(1), m.Tour, "puis au preneur")
	issue := encherir(t, m, 1, NouvellePasse())

	assert.True(t, issue.EncheresTerminees)
	assert.Equal(t, 2, m.Multiplicateur)
	assert.Equal(t, Siege(1), m.Preneur)
}

func TestSurcoinche(t *testing.T) {
	m := meneDeTest(t, 0)
	encherir(t, m, 1, NouvelleAnnonce(AtoutCoeur, A80))
	encherir(t, m, 2, Enchere{Type: Coinche})
	issue := encherir(t, m, 3, Enchere{Type: Surcoinche})

	assert.True(t, issue.EncheresTerminees)
	assert.Equal(t, 4, m.Multiplicateur)
	assert.Equal(t, PhaseJeu, m.Phase)
}

func TestSurcoincheSansCoinche(t *testing.T) {
	m := meneDeTest(t, 0)
	encherir(t, m, 1, NouvelleAnnonce(AtoutCoeur, A80))
	_, err := m.Encherir(2, Enchere{Type: Surcoinche})
	assert.ErrorIs(t, err, ErrEnchereInvalide)
}

func TestEnchereHorsPhase(t *testing.T) {
	m := meneDeTest(t, 0)
	encherir(t, m, 1, NouvelleAnnonce(AtoutCoeur, A80))
	encherir(t, m, 2, NouvellePasse())
	encherir(t, m, 3, NouvellePasse())
	encherir(t, m, 0, NouvellePasse())

	_, err := m.Encherir(1, NouvellePasse())
	assert.ErrorIs(t, err, ErrPhase)
}

func TestLEnchereEnregistreeEstUneCopie(t *testing.T) {
	m := meneDeTest(t, 0)
	e := NouvelleAnnonce(AtoutCoeur, A80)
	encherir(t, m, 1, e)

	e.Contrat.Valeur = A160
	assert.Equal(t, A80, m.Contrat.Valeur)
	assert.Equal(t, A80, m.Encheres[0].Contrat.Valeur)
}

func TestDetenteurBelote(t *testing.T) {
	mains := mainsParCouleur()
	m := nouvelleMene(1, 3, paquetPour(3, mains))
	encherir(t, m, 0, NouvelleAnnonce(AtoutTrefle, A80))
	encherir(t, m, 1, NouvellePasse())
	encherir(t, m, 2, NouvellePasse())
	encherir(t, m, 3, NouvellePasse())

	require.NotNil(t, m.Belote)
	assert.Equal(t, Siege(2), *m.Belote, "le siège 2 a tous les trèfles")

	m = nouvelleMene(1, 3, paquetPour(3, mains))
	encherir(t, m, 0, NouvelleAnnonce(SansAtout, A80))
	encherir(t, m, 1, NouvellePasse())
	encherir(t, m, 2, NouvellePasse())
	encherir(t, m, 3, NouvellePasse())
	assert.Nil(t, m.Belote, "pas de belote à sans atout")
}
