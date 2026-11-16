package coinche

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCartesJouables(t *testing.T) {
	tests := []struct {
		nom      string
		atout    Atout
		pli      Pli
		siege    Siege
		main     []Carte
		attendus []Carte
	}{
		{
			nom:      "à l'entame tout est jouable",
			atout:    AtoutCoeur,
			pli:      Pli{Entame: 0},
			siege:    0,
			main:     []Carte{{Coeur, Sept}, {Pique, As}},
			attendus: []Carte{{Coeur, Sept}, {Pique, As}},
		},
		{
			nom:      "il faut fournir, sans obligation de monter hors atout",
			atout:    AtoutCoeur,
			pli:      nouveauPli(0, Carte{Pique, Roi}),
			siege:    1,
			main:     []Carte{{Pique, As}, {Pique, Sept}, {Coeur, Valet}},
			attendus: []Carte{{Pique, As}, {Pique, Sept}},
		},
		{
			nom:      "atout demandé : il faut monter",
			atout:    AtoutCoeur,
			pli:      nouveauPli(0, Carte{Coeur, Neuf}),
			siege:    1,
			main:     []Carte{{Coeur, Valet}, {Coeur, Dame}, {Pique, As}},
			attendus: []Carte{{Coeur, Valet}},
		},
		{
			nom:      "atout demandé : faute de pouvoir monter, n'importe quel atout",
			atout:    AtoutCoeur,
			pli:      nouveauPli(0, Carte{Coeur, Neuf}),
			siege:    1,
			main:     []Carte{{Coeur, Dame}, {Coeur, Huit}, {Pique, As}},
			attendus: []Carte{{Coeur, Dame}, {Coeur, Huit}},
		},
		{
			nom:      "atout demandé : il faut monter même si le partenaire est maître",
			atout:    AtoutCoeur,
			pli:      nouveauPli(0, Carte{Coeur, Neuf}, Carte{Coeur, Sept}),
			siege:    2,
			main:     []Carte{{Coeur, Valet}, {Coeur, Huit}},
			attendus: []Carte{{Coeur, Valet}},
		},
		{
			nom:      "sans la couleur demandée il faut couper",
			atout:    AtoutCoeur,
			pli:      nouveauPli(0, Carte{Pique, As}),
			siege:    1,
			main:     []Carte{{Coeur, Sept}, {Coeur, Valet}, {Carreau, As}},
			attendus: []Carte{{Coeur, Sept}, {Coeur, Valet}},
		},
		{
			nom:      "partenaire maître : pas d'obligation de couper",
			atout:    AtoutCoeur,
			pli:      nouveauPli(0, Carte{Pique, As}, Carte{Pique, Sept}),
			siege:    2,
			main:     []Carte{{Coeur, Sept}, {Carreau, As}},
			attendus: []Carte{{Coeur, Sept}, {Carreau, As}},
		},
		{
			nom:      "un adversaire a coupé : il faut surcouper",
			atout:    AtoutCoeur,
			pli:      nouveauPli(0, Carte{Pique, As}, Carte{Coeur, Neuf}),
			siege:    2,
			main:     []Carte{{Coeur, Valet}, {Coeur, Dame}, {Carreau, As}},
			attendus: []Carte{{Coeur, Valet}},
		},
		{
			nom:      "impossible de surcouper : il faut sous-couper",
			atout:    AtoutCoeur,
			pli:      nouveauPli(0, Carte{Pique, As}, Carte{Coeur, Valet}),
			siege:    2,
			main:     []Carte{{Coeur, Dame}, {Coeur, Sept}, {Carreau, As}},
			attendus: []Carte{{Coeur, Dame}, {Coeur, Sept}},
		},
		{
			nom:      "partenaire maître grâce à une coupe : défausse libre",
			atout:    AtoutCoeur,
			pli:      nouveauPli(0, Carte{Pique, As}, Carte{Coeur, Neuf}, Carte{Pique, Sept}),
			siege:    3,
			main:     []Carte{{Coeur, Valet}, {Carreau, As}},
			attendus: []Carte{{Coeur, Valet}, {Carreau, As}},
		},
		{
			nom:      "partenaire surcoupé : il faut couper à nouveau",
			atout:    AtoutCoeur,
			pli:      nouveauPli(0, Carte{Pique, As}, Carte{Coeur, Neuf}, Carte{Coeur, Valet}),
			siege:    3,
			main:     []Carte{{Coeur, Dame}, {Carreau, As}},
			attendus: []Carte{{Coeur, Dame}},
		},
		{
			nom:      "ni la couleur ni atout : défausse libre",
			atout:    AtoutCoeur,
			pli:      nouveauPli(0, Carte{Pique, As}),
			siege:    1,
			main:     []Carte{{Carreau, As}, {Trefle, Sept}},
			attendus: []Carte{{Carreau, As}, {Trefle, Sept}},
		},
		{
			nom:      "sans atout : il faut fournir sans monter",
			atout:    SansAtout,
			pli:      nouveauPli(0, Carte{Pique, Roi}),
			siege:    1,
			main:     []Carte{{Pique, Sept}, {Pique, As}, {Coeur, Valet}},
			attendus: []Carte{{Pique, Sept}, {Pique, As}},
		},
		{
			nom:      "sans atout : défausse libre sans la couleur",
			atout:    SansAtout,
			pli:      nouveauPli(0, Carte{Pique, Roi}),
			siege:    1,
			main:     []Carte{{Coeur, Valet}, {Trefle, Sept}},
			attendus: []Carte{{Coeur, Valet}, {Trefle, Sept}},
		},
		{
			nom:      "tout atout : il faut monter dans la couleur demandée",
			atout:    ToutAtout,
			pli:      nouveauPli(0, Carte{Pique, Neuf}),
			siege:    1,
			main:     []Carte{{Pique, Valet}, {Pique, Sept}, {Coeur, Neuf}},
			attendus: []Carte{{Pique, Valet}},
		},
		{
			nom:      "tout atout : faute de pouvoir monter, toute la couleur",
			atout:    ToutAtout,
			pli:      nouveauPli(0, Carte{Pique, Valet}),
			siege:    1,
			main:     []Carte{{Pique, Neuf}, {Pique, Sept}, {Coeur, Valet}},
			attendus: []Carte{{Pique, Neuf}, {Pique, Sept}},
		},
		{
			nom:      "tout atout : on ne coupe pas",
			atout:    ToutAtout,
			pli:      nouveauPli(0, Carte{Pique, Valet}),
			siege:    1,
			main:     []Carte{{Coeur, Valet}, {Trefle, Sept}},
			attendus: []Carte{{Coeur, Valet}, {Trefle, Sept}},
		},
		{
			nom:      "tout atout : la référence est la plus forte carte de la couleur demandée",
			atout:    ToutAtout,
			pli:      nouveauPli(1, Carte{Coeur, Neuf}, Carte{Trefle, Dame}),
			siege:    3,
			main:     []Carte{{Coeur, Dame}, {Coeur, Valet}, {Trefle, Valet}},
			attendus: []Carte{{Coeur, Valet}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.nom, func(t *testing.T) {
			jouables := CartesJouables(tt.main, tt.pli, tt.atout, tt.siege)
			assert.ElementsMatch(t, tt.attendus, jouables)

			for _, c := range tt.main {
				attendu := false
				for _, a := range tt.attendus {
					attendu = attendu || a == c
				}
				assert.Equal(t, attendu, PeutJouer(tt.main, c, tt.pli, tt.atout, tt.siege), "PeutJouer(%v)", c)
			}
		})
	}
}

func TestPeutJouerCarteAbsente(t *testing.T) {
	main := []Carte{{Coeur, Sept}}
	assert.False(t, PeutJouer(main, Carte{Pique, As}, Pli{}, AtoutCoeur, 0))
}
