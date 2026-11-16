package bot

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
)

func vueEncheres(siege coinche.Siege, main []coinche.Carte, options coinche.OptionsEnchere, encheres ...coinche.Enchere) coinche.Vue {
	vm := &coinche.VueMene{
		Phase:          coinche.PhaseEncheres,
		Tour:           siege,
		Encheres:       encheres,
		Multiplicateur: 1,
		OptionsEnchere: &options,
	}
	for _, e := range encheres {
		if e.Type == coinche.Annonce {
			c := *e.Contrat
			s := e.Siege
			vm.Contrat, vm.Preneur = &c, &s
		}
	}
	return coinche.Vue{Siege: siege, Main: main, Mene: vm}
}

func annonce(s coinche.Siege, atout coinche.Atout, valeur coinche.ValeurAnnonce) coinche.Enchere {
	e := coinche.NouvelleAnnonce(atout, valeur)
	e.Siege = s
	return e
}

var ouverture = coinche.OptionsEnchere{Passe: true, AnnonceMin: coinche.A80}

func TestAnnonceSurUneMainForte(t *testing.T) {
	main := []coinche.Carte{carte("V-coeur"), carte("9-coeur"), carte("A-coeur"), carte("10-coeur"), carte("7-coeur"), carte("A-pique"), carte("A-trefle"), carte("8-carreau")}

	e := Bot{}.Encherir(vueEncheres(0, main, ouverture))

	require.Equal(t, coinche.Annonce, e.Type)
	assert.Equal(t, coinche.AtoutCoeur, e.Contrat.Atout)
	assert.GreaterOrEqual(t, e.Contrat.Valeur, coinche.A120)
	assert.LessOrEqual(t, e.Contrat.Valeur, coinche.A160)
}

func TestPasseSurUneMainFaible(t *testing.T) {
	main := []coinche.Carte{carte("7-coeur"), carte("8-carreau"), carte("9-trefle"), carte("D-pique"), carte("7-pique"), carte("8-trefle"), carte("R-carreau"), carte("10-trefle")}

	assert.Equal(t, coinche.NouvellePasse(), Bot{}.Encherir(vueEncheres(0, main, ouverture)))
}

func TestAnnonceMinimale(t *testing.T) {
	main := []coinche.Carte{carte("V-coeur"), carte("9-coeur"), carte("7-coeur"), carte("A-pique"), carte("7-pique"), carte("8-trefle"), carte("8-carreau"), carte("7-carreau")}

	assert.Equal(t, coinche.NouvelleAnnonce(coinche.AtoutCoeur, coinche.A80), Bot{}.Encherir(vueEncheres(0, main, ouverture)))
}

func TestSurencheritAuMinimumSurLAdversaire(t *testing.T) {
	main := []coinche.Carte{carte("V-coeur"), carte("9-coeur"), carte("A-coeur"), carte("10-coeur"), carte("7-coeur"), carte("A-pique"), carte("A-trefle"), carte("8-carreau")}
	options := coinche.OptionsEnchere{Passe: true, AnnonceMin: coinche.A90, Coinche: true}

	e := Bot{}.Encherir(vueEncheres(0, main, options, annonce(3, coinche.AtoutTrefle, coinche.A80)))

	assert.Equal(t, coinche.NouvelleAnnonce(coinche.AtoutCoeur, coinche.A90), e)
}

func TestNeCoincheJamais(t *testing.T) {
	main := []coinche.Carte{carte("7-coeur"), carte("8-carreau"), carte("8-trefle"), carte("D-pique"), carte("A-pique"), carte("7-trefle"), carte("A-carreau"), carte("10-trefle")}
	options := coinche.OptionsEnchere{Passe: true, AnnonceMin: coinche.A90, Coinche: true}

	assert.Equal(t, coinche.NouvellePasse(), Bot{}.Encherir(vueEncheres(0, main, options, annonce(3, coinche.AtoutCoeur, coinche.A80))))
}

func TestSurcoincheUnContratLargementAssure(t *testing.T) {
	main := []coinche.Carte{carte("V-coeur"), carte("9-coeur"), carte("A-coeur"), carte("10-coeur"), carte("7-coeur"), carte("A-pique"), carte("A-trefle"), carte("8-carreau")}
	options := coinche.OptionsEnchere{Passe: true, Surcoinche: true}
	vue := vueEncheres(0, main, options, annonce(0, coinche.AtoutCoeur, coinche.A100))

	assert.Equal(t, coinche.Surcoinche, Bot{}.Encherir(vue).Type)
	assert.Equal(t, coinche.NouvellePasse(), Bot{Profil: Bernard}.Encherir(vue), "le prudent ne surcoinche jamais")
}

func TestSoutientSonPartenaire(t *testing.T) {
	main := []coinche.Carte{carte("A-coeur"), carte("A-carreau"), carte("V-pique"), carte("7-trefle"), carte("8-trefle"), carte("9-carreau"), carte("7-coeur"), carte("8-coeur")}
	options := coinche.OptionsEnchere{Passe: true, AnnonceMin: coinche.A90}

	e := Bot{}.Encherir(vueEncheres(0, main, options, annonce(2, coinche.AtoutPique, coinche.A80)))

	assert.Equal(t, coinche.NouvelleAnnonce(coinche.AtoutPique, coinche.A110), e)
}

func TestNeChangePasLaCouleurDuPartenaire(t *testing.T) {
	main := []coinche.Carte{carte("V-coeur"), carte("9-coeur"), carte("10-coeur"), carte("R-coeur"), carte("7-coeur"), carte("8-trefle"), carte("7-carreau"), carte("8-carreau")}
	options := coinche.OptionsEnchere{Passe: true, AnnonceMin: coinche.A90}

	assert.Equal(t, coinche.NouvellePasse(), Bot{}.Encherir(vueEncheres(0, main, options, annonce(2, coinche.AtoutPique, coinche.A80))))
}

func TestNeSoutientQuUneFois(t *testing.T) {
	main := []coinche.Carte{carte("A-coeur"), carte("A-carreau"), carte("V-pique"), carte("7-trefle"), carte("8-trefle"), carte("9-carreau"), carte("7-coeur"), carte("8-coeur")}
	options := coinche.OptionsEnchere{Passe: true, AnnonceMin: coinche.A120}

	e := Bot{}.Encherir(vueEncheres(0, main, options, annonce(2, coinche.AtoutPique, coinche.A80), annonce(0, coinche.AtoutPique, coinche.A100), annonce(2, coinche.AtoutPique, coinche.A110)))

	assert.Equal(t, coinche.NouvellePasse(), e)
}

func TestCapotAvecTousLesAtouts(t *testing.T) {
	var main []coinche.Carte
	for _, v := range coinche.Valeurs {
		main = append(main, coinche.Carte{Couleur: coinche.Pique, Valeur: v})
	}

	assert.Equal(t, coinche.NouvelleAnnonce(coinche.AtoutPique, coinche.Capot), Bot{}.Encherir(vueEncheres(0, main, ouverture)))
}

func TestSansAtoutAvecLesAs(t *testing.T) {
	main := []coinche.Carte{carte("A-coeur"), carte("10-coeur"), carte("A-carreau"), carte("10-carreau"), carte("A-trefle"), carte("A-pique"), carte("R-pique"), carte("7-trefle")}

	e := Bot{}.Encherir(vueEncheres(0, main, ouverture))

	require.Equal(t, coinche.Annonce, e.Type)
	assert.Equal(t, coinche.SansAtout, e.Contrat.Atout)
}

func TestToutAtoutAvecLesValets(t *testing.T) {
	main := []coinche.Carte{carte("V-coeur"), carte("9-coeur"), carte("V-carreau"), carte("9-carreau"), carte("V-trefle"), carte("V-pique"), carte("7-coeur"), carte("8-carreau")}

	e := Bot{}.Encherir(vueEncheres(0, main, ouverture))

	require.Equal(t, coinche.Annonce, e.Type)
	assert.Equal(t, coinche.ToutAtout, e.Contrat.Atout)
}

func TestPasseApresLaGenerale(t *testing.T) {
	main := []coinche.Carte{carte("V-coeur"), carte("9-coeur"), carte("A-coeur"), carte("10-coeur"), carte("7-coeur"), carte("A-pique"), carte("A-trefle"), carte("8-carreau")}
	options := coinche.OptionsEnchere{Passe: true, Coinche: true}

	assert.Equal(t, coinche.NouvellePasse(), Bot{}.Encherir(vueEncheres(0, main, options, annonce(1, coinche.AtoutPique, coinche.Generale))))
}

func TestEncherirSansVue(t *testing.T) {
	assert.Equal(t, coinche.NouvellePasse(), Bot{}.Encherir(coinche.Vue{}))
}
