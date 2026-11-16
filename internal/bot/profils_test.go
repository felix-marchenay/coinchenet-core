package bot

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
)

func TestProfils(t *testing.T) {
	// Les identifiants sont enregistrés dans le journal des parties : les changer casserait la reprise.
	assert.Equal(t, []string{"bernard", "martine", "nadia", "leo"}, Bots{}.Profils())

	noms := map[string]bool{}
	for _, p := range Profils {
		assert.False(t, noms[p.Nom], "prénom en double : %s", p.Nom)
		noms[p.Nom] = true
		assert.NotEmpty(t, p.Style)
		assert.NotEmpty(t, p.Description)
		nom, ok := Bots{}.Nom(p.ID)
		assert.True(t, ok)
		assert.Equal(t, p.Nom, nom)
	}

	_, ok := Bots{}.Nom("zorro")
	assert.False(t, ok)
	assert.Equal(t, Defaut, Pour("zorro"))
	assert.Equal(t, Defaut, Pour(""), "un joueur absent est remplacé par le profil par défaut")
}

func TestLePrudentCoincheUnContratTropGourmand(t *testing.T) {
	// Valet et as d'atout, as-dix à pique : de quoi faire chuter 100 à cœur, pas de quoi annoncer 110.
	main := []coinche.Carte{carte("V-coeur"), carte("A-coeur"), carte("8-coeur"), carte("A-pique"), carte("10-pique"), carte("7-trefle"), carte("8-trefle"), carte("7-carreau")}
	options := coinche.OptionsEnchere{Passe: true, AnnonceMin: coinche.A110, Coinche: true}
	vue := vueEncheres(0, main, options, annonce(1, coinche.AtoutCoeur, coinche.A100))

	assert.Equal(t, coinche.Coinche, Bot{Profil: Bernard}.Encherir(vue).Type)
	assert.Equal(t, coinche.NouvellePasse(), Bot{Profil: Nadia}.Encherir(vue), "l'agressive préfère annoncer que coincher")
}

func TestLeCasseCouAnnonceLeCapotALIntuition(t *testing.T) {
	// Atouts maîtres et deux as, mais le 7 de pique peut se perdre : pas un capot sûr.
	main := []coinche.Carte{carte("V-coeur"), carte("9-coeur"), carte("A-coeur"), carte("10-coeur"), carte("A-pique"), carte("7-pique"), carte("A-trefle"), carte("10-trefle")}
	vue := vueEncheres(0, main, ouverture)

	assert.Equal(t, coinche.NouvelleAnnonce(coinche.AtoutCoeur, coinche.Capot), Bot{Profil: Leo}.Encherir(vue))
	assert.Equal(t, coinche.NouvelleAnnonce(coinche.AtoutCoeur, coinche.A150), Bot{Profil: Martine}.Encherir(vue))
}

func TestLAgressiveChargeLePliDeSonPartenaireSansAttendre(t *testing.T) {
	// Le partenaire mène avec l'as de pique ; le dernier joueur peut encore couper.
	main := []coinche.Carte{carte("10-pique"), carte("7-pique"), carte("8-carreau"), carte("9-trefle")}
	vue := vueJeu(2, main, coeur80, 1, nouveauPli(0, carte("A-pique"), carte("8-pique")))

	assert.Equal(t, carte("10-pique"), Bot{Profil: Nadia}.Jouer(vue))
	assert.Equal(t, carte("7-pique"), Bot{Profil: Martine}.Jouer(vue), "l'équilibrée attend que le pli soit à l'abri")
}
