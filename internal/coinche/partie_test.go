package coinche

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mainsParCouleur donne tous les cœurs au siège 0, les carreaux au 1, les trèfles au 2 et les piques au 3.
func mainsParCouleur() [NbSieges][]Carte {
	var mains [NbSieges][]Carte
	for _, c := range Couleurs {
		for _, v := range Valeurs {
			mains[c] = append(mains[c], Carte{c, v})
		}
	}
	return mains
}

// paquetPour construit le paquet qui, distribué en 3-2-3 par ce donneur, donne exactement ces mains.
func paquetPour(donneur Siege, mains [NbSieges][]Carte) CarteCollection {
	var groupes [][]Carte
	var pos [NbSieges]int
	for _, nb := range []int{3, 2, 3} {
		s := donneur.Suivant()
		for range NbSieges {
			groupes = append(groupes, mains[s][pos[s]:pos[s]+nb])
			pos[s] += nb
			s = s.Suivant()
		}
	}
	var cartes []Carte
	for i := len(groupes) - 1; i >= 0; i-- {
		cartes = append(cartes, groupes[i]...)
	}
	return CarteCollection{Cartes: cartes}
}

func nouvellePartieDeTest(t *testing.T, r Regles, donneur Siege) *Partie {
	t.Helper()
	p, err := NouvellePartie(r, donneur)
	require.NoError(t, err)
	return p
}

// jouerMene fait jouer à chacun sa première carte jouable jusqu'à la fin de la mène.
func jouerMene(t *testing.T, p *Partie) []IssueCarte {
	t.Helper()
	var issues []IssueCarte
	for {
		s, phase, ok := p.Tour()
		if !ok || phase != PhaseJeu {
			return issues
		}
		issue, err := p.Jouer(s, p.CartesJouables(s)[0])
		require.NoError(t, err)
		issues = append(issues, issue)
	}
}

func TestNouvellePartieInvalide(t *testing.T) {
	_, err := NouvellePartie(Regles{Objectif: 750}, 0)
	assert.Error(t, err)

	_, err = NouvellePartie(ReglesParDefaut(), 4)
	assert.Error(t, err)
}

func TestDistribution323(t *testing.T) {
	p := nouvellePartieDeTest(t, ReglesParDefaut(), 2)
	mains := mainsParCouleur()

	m, err := p.Distribuer(paquetPour(2, mains))
	require.NoError(t, err)

	assert.Equal(t, 1, m.Numero)
	assert.Equal(t, Siege(2), m.Donneur)
	assert.Equal(t, Siege(3), m.Tour)
	for _, s := range Sieges {
		assert.Equal(t, mains[s], m.Mains[s].Cartes)
		assert.Equal(t, mains[s], m.MainsInitiales[s])
	}
}

func TestDistributionOrdreDuPaquet(t *testing.T) {
	paquet := NouveauPaquet32()
	p := nouvellePartieDeTest(t, ReglesParDefaut(), 0)

	m, err := p.Distribuer(paquet)
	require.NoError(t, err)

	// Le siège 1 reçoit les 3 cartes du dessus, puis 2 au deuxième tour, puis 3 au troisième.
	c := paquet.Cartes
	attendu := append(append(append([]Carte{}, c[29:32]...), c[18:20]...), c[9:12]...)
	assert.Equal(t, attendu, m.Mains[1].Cartes)
}

func TestDistribuerErreurs(t *testing.T) {
	p := nouvellePartieDeTest(t, ReglesParDefaut(), 0)

	_, err := p.Distribuer(CarteCollection{Cartes: NouveauPaquet32().Cartes[:31]})
	require.ErrorIs(t, err, ErrPaquetInvalide)

	_, err = p.Distribuer(NouveauPaquet32())
	require.NoError(t, err)

	_, err = p.Distribuer(NouveauPaquet32())
	assert.ErrorIs(t, err, ErrDonneEnCours)
}

func TestJouerHorsMene(t *testing.T) {
	p := nouvellePartieDeTest(t, ReglesParDefaut(), 0)

	_, err := p.Jouer(1, Carte{Coeur, As})
	assert.ErrorIs(t, err, ErrAucuneMene)

	_, err = p.Encherir(1, NouvellePasse())
	assert.ErrorIs(t, err, ErrAucuneMene)
}

func preneur0A80Coeur(t *testing.T, p *Partie) {
	t.Helper()
	m := p.MeneEnCours()
	s := m.Tour
	for range NbSieges {
		e := NouvellePasse()
		if s == 0 {
			e = NouvelleAnnonce(AtoutCoeur, A80)
		}
		_, err := p.Encherir(s, e)
		require.NoError(t, err)
		s = s.Suivant()
	}
	for m.Phase == PhaseEncheres {
		_, err := p.Encherir(m.Tour, NouvellePasse())
		require.NoError(t, err)
	}
}

func TestMeneComplete(t *testing.T) {
	p := nouvellePartieDeTest(t, ReglesParDefaut(), 3)
	_, err := p.Distribuer(paquetPour(3, mainsParCouleur()))
	require.NoError(t, err)
	preneur0A80Coeur(t, p)

	issues := jouerMene(t, p)

	require.Len(t, issues, 32)
	var plis int
	var annonces []AnnonceBelote
	for _, i := range issues {
		if i.Pli != nil {
			plis++
			assert.Equal(t, Siege(0), i.Pli.Gagnant, "le siège 0 a tous les atouts")
		}
		if i.Belote != PasDeBelote {
			annonces = append(annonces, i.Belote)
		}
	}
	assert.Equal(t, NbPlis, plis)
	assert.Equal(t, []AnnonceBelote{BeloteAnnoncee, RebeloteAnnoncee}, annonces)

	r := issues[len(issues)-1].Mene
	require.NotNil(t, r)
	assert.True(t, r.Reussi)
	assert.Equal(t, equipe(EquipeA), r.Capot)
	assert.Equal(t, equipe(EquipeA), r.Belote)
	assert.Equal(t, [2]int{350, 0}, r.Points, "capot 250 + contrat 80 + belote 20")
	assert.Equal(t, [2]int{350, 0}, p.Scores)
	assert.True(t, p.AttendDonne())
	assert.False(t, p.Terminee)
}

func TestPartieJusquALObjectif(t *testing.T) {
	r := ReglesParDefaut()
	r.Objectif = 500
	p := nouvellePartieDeTest(t, r, 3)

	for numero := 1; !p.Terminee; numero++ {
		require.LessOrEqual(t, numero, 2)
		donneur := p.ProchainDonneur()
		m, err := p.Distribuer(paquetPour(donneur, mainsParCouleur()))
		require.NoError(t, err)
		assert.Equal(t, numero, m.Numero)
		preneur0A80Coeur(t, p)
		jouerMene(t, p)
	}

	assert.Equal(t, [2]int{700, 0}, p.Scores)
	assert.Equal(t, EquipeA, p.Gagnante)
	assert.False(t, p.Forfait)
	assert.Equal(t, Siege(0), p.Menes[1].Donneur, "le donneur tourne")

	_, err := p.Distribuer(NouveauPaquet32())
	assert.ErrorIs(t, err, ErrPartieTerminee)
	_, _, ok := p.Tour()
	assert.False(t, ok)
}

func TestEgaliteAuDessusDeLObjectif(t *testing.T) {
	r := ReglesParDefaut()
	r.Objectif = 500
	p := nouvellePartieDeTest(t, r, 0)
	p.Scores = [2]int{600, 600}

	assert.False(t, p.verifierFin(), "une égalité impose une mène de plus")

	p.Scores = [2]int{600, 610}
	assert.True(t, p.verifierFin())
	assert.Equal(t, EquipeB, p.Gagnante)
}

func TestRedistributionQuandToutLeMondePasse(t *testing.T) {
	p := nouvellePartieDeTest(t, ReglesParDefaut(), 1)
	_, err := p.Distribuer(NouveauPaquet32())
	require.NoError(t, err)

	for _, s := range []Siege{2, 3, 0, 1} {
		_, err := p.Encherir(s, NouvellePasse())
		require.NoError(t, err)
	}

	assert.True(t, p.AttendDonne())
	assert.Equal(t, Siege(2), p.ProchainDonneur())
	assert.Equal(t, [2]int{0, 0}, p.Scores)
	assert.Empty(t, p.Historique()[0].Points[EquipeA])

	paquet := p.PaquetSuivant(FakeMelangeur{}, NoCoupeur{})
	assert.True(t, paquet.EstPaquetComplet())
	m := p.MeneCourante()
	assert.Equal(t, m.MainsInitiales[2], paquet.Cartes[:8], "on ramasse les mains en commençant après le donneur")
}

func TestPaquetSuivantTraditionnel(t *testing.T) {
	p := nouvellePartieDeTest(t, ReglesParDefaut(), 3)
	_, err := p.Distribuer(paquetPour(3, mainsParCouleur()))
	require.NoError(t, err)
	preneur0A80Coeur(t, p)
	jouerMene(t, p)

	paquet := p.PaquetSuivant(FakeMelangeur{}, NoCoupeur{})

	var attendu []Carte
	for _, pli := range p.MeneCourante().Plis {
		attendu = append(attendu, pli.SCartes().Cartes...)
	}
	assert.Equal(t, attendu, paquet.Cartes)
}

func TestPaquetSuivantAleatoire(t *testing.T) {
	p := nouvellePartieDeTest(t, ReglesClassees(), 3)
	_, err := p.Distribuer(paquetPour(3, mainsParCouleur()))
	require.NoError(t, err)
	preneur0A80Coeur(t, p)
	jouerMene(t, p)

	fixe := FischerYatesMelangeur{Hasard: NouveauHasardDeterministe(3)}.Melanger(NouveauPaquet32())
	paquet := p.PaquetSuivant(FakeMelangeur(fixe), NoCoupeur{})

	assert.Equal(t, fixe.Cartes, paquet.Cartes, "le paquet est entièrement remélangé")
}

func TestJouerErreurs(t *testing.T) {
	p := nouvellePartieDeTest(t, ReglesParDefaut(), 3)
	_, err := p.Distribuer(paquetPour(3, mainsParCouleur()))
	require.NoError(t, err)

	_, err = p.Jouer(0, Carte{Coeur, Valet})
	assert.ErrorIs(t, err, ErrPhase, "on ne joue pas pendant les enchères")

	preneur0A80Coeur(t, p)

	_, err = p.Jouer(1, Carte{Carreau, As})
	assert.ErrorIs(t, err, ErrPasSonTour)

	_, err = p.Jouer(0, Carte{Pique, As})
	assert.ErrorIs(t, err, ErrCarteAbsente)

	_, err = p.Jouer(0, Carte{Coeur, Valet})
	require.NoError(t, err)

	_, err = p.Jouer(1, Carte{Pique, As})
	assert.ErrorIs(t, err, ErrCarteAbsente)
}

func TestCarteInterdite(t *testing.T) {
	mains := mainsParCouleur()
	// Le siège 1 récupère un cœur : il devra fournir.
	mains[0][0], mains[1][0] = mains[1][0], mains[0][0]
	p := nouvellePartieDeTest(t, ReglesParDefaut(), 3)
	_, err := p.Distribuer(paquetPour(3, mains))
	require.NoError(t, err)
	preneur0A80Coeur(t, p)

	_, err = p.Jouer(0, Carte{Coeur, As})
	require.NoError(t, err)

	_, err = p.Jouer(1, Carte{Carreau, As})
	assert.ErrorIs(t, err, ErrCarteInterdite)
	_, err = p.Jouer(1, Carte{Coeur, Sept})
	assert.NoError(t, err)
}

func TestAbandon(t *testing.T) {
	p := nouvellePartieDeTest(t, ReglesParDefaut(), 0)
	_, err := p.Distribuer(NouveauPaquet32())
	require.NoError(t, err)

	p.Abandonner(EquipeB)

	assert.True(t, p.Terminee)
	assert.True(t, p.Forfait)
	assert.Equal(t, EquipeA, p.Gagnante)
	_, err = p.Encherir(1, NouvellePasse())
	assert.ErrorIs(t, err, ErrPartieTerminee)

	p.Abandonner(EquipeA)
	assert.Equal(t, EquipeA, p.Gagnante, "un second abandon ne change rien")
}

func TestVue(t *testing.T) {
	p := nouvellePartieDeTest(t, ReglesParDefaut(), 3)
	mains := mainsParCouleur()
	_, err := p.Distribuer(paquetPour(3, mains))
	require.NoError(t, err)

	v := p.Vue(1)
	assert.Equal(t, mains[1], v.Main, "chacun ne voit que sa main")
	require.NotNil(t, v.Mene)
	assert.Equal(t, [NbSieges]int{8, 8, 8, 8}, v.Mene.NbCartes)
	assert.Nil(t, v.Mene.OptionsEnchere, "ce n'est pas son tour")
	assert.NotNil(t, p.Vue(0).Mene.OptionsEnchere)

	preneur0A80Coeur(t, p)

	assert.Nil(t, p.Vue(1).Mene.Belote, "la belote reste secrète tant qu'elle n'est pas annoncée")
	assert.Equal(t, siege(0), p.Vue(0).Mene.Belote, "son détenteur la connaît")
	assert.ElementsMatch(t, mains[0], p.Vue(0).Mene.CartesJouables)

	for _, c := range []Carte{{Coeur, Dame}, {Carreau, Sept}, {Trefle, Sept}, {Pique, Sept}} {
		s, _, _ := p.Tour()
		_, err := p.Jouer(s, c)
		require.NoError(t, err)
	}

	v = p.Vue(1)
	assert.Equal(t, siege(0), v.Mene.Belote)
	assert.Len(t, v.Mene.Plis, 1)
	assert.Len(t, v.Mene.CartesJouees(), 4)
	assert.Equal(t, [NbSieges]int{7, 7, 7, 7}, v.Mene.NbCartes)
}

func TestSerializePartie(t *testing.T) {
	p := nouvellePartieDeTest(t, ReglesParDefaut(), 3)
	_, err := p.Distribuer(paquetPour(3, mainsParCouleur()))
	require.NoError(t, err)
	preneur0A80Coeur(t, p)
	jouerMene(t, p)

	s := SerializePartie(p)
	assert.Contains(t, s, "80 coeur")
	assert.Contains(t, s, "A 350 - B 0")
}
