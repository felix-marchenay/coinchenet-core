package bot

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
)

func carte(s string) coinche.Carte {
	c, err := coinche.ParseCarte(s)
	if err != nil {
		panic(err)
	}
	return c
}

func nouveauPli(entame coinche.Siege, cartes ...coinche.Carte) coinche.Pli {
	p := coinche.Pli{Entame: entame}
	s := entame
	for _, c := range cartes {
		p.Cartes = append(p.Cartes, coinche.CarteJouee{Siege: s, Carte: c})
		s = s.Suivant()
	}
	return p
}

func vueJeu(siege coinche.Siege, main []coinche.Carte, contrat coinche.Contrat, preneur coinche.Siege, pli coinche.Pli, plis ...coinche.Pli) coinche.Vue {
	if len(pli.Cartes) == 0 {
		pli.Entame = siege
	}
	vm := &coinche.VueMene{
		Phase:          coinche.PhaseJeu,
		Tour:           siege,
		Contrat:        &contrat,
		Preneur:        &preneur,
		Multiplicateur: 1,
		PliEnCours:     pli,
		Plis:           plis,
		CartesJouables: coinche.CartesJouables(main, pli, contrat.Atout, siege),
	}
	return coinche.Vue{Siege: siege, Main: main, Mene: vm}
}

var coeur80 = coinche.Contrat{Atout: coinche.AtoutCoeur, Valeur: coinche.A80}

func TestUneSeuleCarteJouable(t *testing.T) {
	v := vueJeu(1, []coinche.Carte{carte("7-pique"), carte("A-carreau")}, coeur80, 0, nouveauPli(0, carte("A-pique")))

	assert.Equal(t, carte("7-pique"), Bot{}.Jouer(v))
}

func TestEntamePreneurFaitTomberLesAtouts(t *testing.T) {
	v := vueJeu(0, []coinche.Carte{carte("7-coeur"), carte("V-coeur"), carte("A-pique"), carte("8-trefle")}, coeur80, 0, coinche.Pli{})

	assert.Equal(t, carte("V-coeur"), Bot{}.Jouer(v))
}

func TestEntamePreneurSansAtoutMaitre(t *testing.T) {
	v := vueJeu(0, []coinche.Carte{carte("9-coeur"), carte("7-coeur"), carte("A-pique"), carte("8-trefle")}, coeur80, 0, coinche.Pli{})

	assert.Equal(t, carte("A-pique"), Bot{}.Jouer(v), "le valet d'atout est dehors : on joue l'as maître")
}

func TestEntameDefenseNeJouePasAtout(t *testing.T) {
	v := vueJeu(0, []coinche.Carte{carte("V-coeur"), carte("A-pique"), carte("7-carreau"), carte("8-trefle")}, coeur80, 1, coinche.Pli{})

	assert.Equal(t, carte("A-pique"), Bot{}.Jouer(v))
}

func TestEntameSansCarteMaitresse(t *testing.T) {
	v := vueJeu(0, []coinche.Carte{carte("R-pique"), carte("8-trefle"), carte("9-carreau"), carte("D-coeur")}, coeur80, 1, coinche.Pli{})

	assert.Equal(t, carte("8-trefle"), Bot{}.Jouer(v), "la plus petite carte hors atout")
}

func TestLeDixDevientMaitreApresLAs(t *testing.T) {
	joue := nouveauPli(1, carte("A-pique"), carte("7-pique"), carte("8-pique"), carte("9-pique"))
	joue.Gagnant = 1
	v := vueJeu(0, []coinche.Carte{carte("10-pique"), carte("8-trefle"), carte("9-carreau")}, coeur80, 1, coinche.Pli{}, joue)

	assert.Equal(t, carte("10-pique"), Bot{}.Jouer(v))
}

func TestChargeLePartenaireMaitre(t *testing.T) {
	pli := nouveauPli(0, carte("R-pique"), carte("A-pique"), carte("7-pique"))
	v := vueJeu(3, []coinche.Carte{carte("10-carreau"), carte("7-carreau"), carte("V-coeur"), carte("8-trefle")}, coeur80, 0, pli)

	assert.Equal(t, carte("10-carreau"), Bot{}.Jouer(v), "dernier à jouer, partenaire maître : on charge sans couper")
}

func TestPartenaireMaitreIncertain(t *testing.T) {
	pli := nouveauPli(0, carte("R-pique"), carte("7-pique"))
	v := vueJeu(2, []coinche.Carte{carte("A-carreau"), carte("7-trefle"), carte("D-coeur")}, coeur80, 0, pli)

	assert.Equal(t, carte("7-trefle"), Bot{}.Jouer(v), "l'as de pique est dehors : on ne charge pas")
}

func TestCoupeMinimale(t *testing.T) {
	v := vueJeu(1, []coinche.Carte{carte("V-coeur"), carte("9-coeur"), carte("7-coeur"), carte("8-carreau")}, coeur80, 0, nouveauPli(0, carte("A-pique")))

	assert.Equal(t, carte("7-coeur"), Bot{}.Jouer(v))
}

func TestDernierPrendAvecLaPlusPetiteGagnante(t *testing.T) {
	pli := nouveauPli(0, carte("R-pique"), carte("7-pique"), carte("8-pique"))
	v := vueJeu(3, []coinche.Carte{carte("A-pique"), carte("10-pique"), carte("7-carreau")}, coeur80, 0, pli)

	assert.Equal(t, carte("10-pique"), Bot{}.Jouer(v))
}

func TestSecondFournitLaPlusForte(t *testing.T) {
	v := vueJeu(1, []coinche.Carte{carte("A-pique"), carte("10-pique"), carte("7-carreau")}, coeur80, 0, nouveauPli(0, carte("R-pique")))

	assert.Equal(t, carte("A-pique"), Bot{}.Jouer(v))
}

func TestDefausseLaMoinsChere(t *testing.T) {
	v := vueJeu(1, []coinche.Carte{carte("10-carreau"), carte("7-trefle"), carte("R-carreau")}, coeur80, 0, nouveauPli(0, carte("A-pique")))

	assert.Equal(t, carte("7-trefle"), Bot{}.Jouer(v))
}

func TestAtoutDemandeMonteAvecLaPlusPetiteMaitresse(t *testing.T) {
	v := vueJeu(1, []coinche.Carte{carte("V-coeur"), carte("9-coeur"), carte("7-carreau")}, coeur80, 0, nouveauPli(0, carte("A-coeur")))

	assert.Equal(t, carte("9-coeur"), Bot{}.Jouer(v), "le 9 est maître puisque le bot tient le valet")
}

func TestJouerSansVue(t *testing.T) {
	assert.Equal(t, coinche.Carte{}, Bot{}.Jouer(coinche.Vue{}))
}
