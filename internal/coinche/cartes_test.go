package coinche

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNouveauPaquet32(t *testing.T) {
	paquet := NouveauPaquet32()

	assert.Len(t, paquet.Cartes, 32)
	assert.True(t, paquet.EstPaquetComplet())

	m := FischerYatesMelangeur{}
	for range 10 {
		avant := SerializePaquet(paquet)
		paquet = m.Melanger(paquet)

		assert.NotEqual(t, avant, SerializePaquet(paquet), "le mélange doit changer l'ordre")
		assert.True(t, paquet.EstPaquetComplet(), "le mélange doit conserver les 32 cartes")
	}
}

func TestMelangeurNeModifiePasLEntree(t *testing.T) {
	paquet := NouveauPaquet32()
	original := append([]Carte(nil), paquet.Cartes...)

	FischerYatesMelangeur{Hasard: NouveauHasardDeterministe(1)}.Melanger(paquet)

	assert.Equal(t, original, paquet.Cartes)
}

func TestMelangeDeterministe(t *testing.T) {
	a := FischerYatesMelangeur{Hasard: NouveauHasardDeterministe(42)}.Melanger(NouveauPaquet32())
	b := FischerYatesMelangeur{Hasard: NouveauHasardDeterministe(42)}.Melanger(NouveauPaquet32())

	assert.Equal(t, a, b)
}

func TestEstPaquetComplet(t *testing.T) {
	paquet := NouveauPaquet32()
	assert.True(t, paquet.EstPaquetComplet())

	doublon := NouveauPaquet32()
	doublon.Cartes[0] = doublon.Cartes[1]
	assert.False(t, doublon.EstPaquetComplet())

	incomplet := CarteCollection{Cartes: paquet.Cartes[:31]}
	assert.False(t, incomplet.EstPaquetComplet())
}

func TestTirer(t *testing.T) {
	paquet := NouveauPaquet32()

	cartesTirees := paquet.Tirer(5)
	assert.Len(t, cartesTirees, 5)
	assert.Len(t, paquet.Cartes, 27)

	assert.Nil(t, paquet.Tirer(50), "tirer plus de cartes qu'il n'en reste renvoie nil")
}

func TestPop(t *testing.T) {
	main := CarteCollection{Cartes: []Carte{{Coeur, As}, {Pique, Sept}}}

	c, err := main.Pop(Carte{Pique, Sept})
	require.NoError(t, err)
	assert.Equal(t, Carte{Pique, Sept}, c)
	assert.Equal(t, []Carte{{Coeur, As}}, main.Cartes)

	_, err = main.Pop(Carte{Trefle, Roi})
	assert.Error(t, err)
}

func TestPliRemporte(t *testing.T) {
	tests := []struct {
		nom     string
		entame  Siege
		cartes  []Carte
		atout   Atout
		gagnant Siege
	}{
		{"sans atout, la couleur demandée gagne", 0, []Carte{{Coeur, Dix}, {Coeur, As}, {Trefle, Roi}, {Pique, Valet}}, SansAtout, 1},
		{"hors atout le dix bat le roi", 0, []Carte{{Coeur, Roi}, {Coeur, Dix}, {Coeur, Valet}, {Coeur, Neuf}}, SansAtout, 1},
		{"l'atout gagne sur la couleur demandée", 0, []Carte{{Coeur, Dix}, {Carreau, As}, {Coeur, Valet}, {Pique, Valet}}, AtoutPique, 3},
		{"l'entame gagne si personne ne fournit plus fort", 0, []Carte{{Trefle, Roi}, {Trefle, Dame}, {Trefle, Valet}, {Trefle, Sept}}, SansAtout, 0},
		{"le valet d'atout est la plus forte carte", 1, []Carte{{Trefle, Dame}, {Trefle, Valet}, {Trefle, Sept}, {Trefle, Roi}}, AtoutTrefle, 2},
		{"tout atout : la plus forte de la couleur demandée", 1, []Carte{{Coeur, Neuf}, {Pique, Valet}, {Trefle, Valet}, {Coeur, Valet}}, ToutAtout, 0},
		{"on fait tomber les atouts", 0, []Carte{{Coeur, Valet}, {Coeur, Neuf}, {Coeur, Sept}, {Coeur, Huit}}, AtoutCoeur, 0},
		{"personne ne fournit ni ne coupe", 3, []Carte{{Trefle, Sept}, {Carreau, As}, {Carreau, Dix}, {Coeur, As}}, AtoutPique, 3},
		{"la coupe l'emporte même jouée en dernier", 3, []Carte{{Trefle, Sept}, {Carreau, As}, {Carreau, Dix}, {Pique, Sept}}, AtoutPique, 2},
		{"la plus forte coupe l'emporte", 0, []Carte{{Trefle, As}, {Pique, Huit}, {Pique, Neuf}, {Pique, As}}, AtoutPique, 2},
	}

	for _, tt := range tests {
		t.Run(tt.nom, func(t *testing.T) {
			p := nouveauPli(tt.entame, tt.cartes...)
			assert.Equal(t, tt.gagnant, p.Remporte(tt.atout))
		})
	}
}

func TestPoints152ParAtout(t *testing.T) {
	paquet := NouveauPaquet32()

	for _, atout := range Atouts {
		total := 0
		for _, carte := range paquet.Cartes {
			total += carte.Points(atout)
		}
		assert.Equal(t, 152, total, "total des cartes pour %v", atout)
	}
}

func TestCouper(t *testing.T) {
	paquet := NouveauPaquet32()
	coupe := RandomCoupeur{}.Couper(paquet)

	require.Len(t, coupe.Cartes, 32)
	assert.NotEqual(t, paquet.Cartes, coupe.Cartes)

	// La coupe conserve l'ordre circulaire du paquet.
	debut := 0
	for i, c := range coupe.Cartes {
		if c == paquet.Cartes[0] {
			debut = i
		}
	}
	assert.Positive(t, debut)
	for i := range paquet.Cartes {
		assert.Equal(t, paquet.Cartes[i], coupe.Cartes[(debut+i)%32])
	}
}

func TestCarteTexte(t *testing.T) {
	for _, c := range NouveauPaquet32().Cartes {
		texte, err := c.MarshalText()
		require.NoError(t, err)

		var relue Carte
		require.NoError(t, relue.UnmarshalText(texte))
		assert.Equal(t, c, relue)
	}

	assert.Equal(t, "V-coeur", Carte{Coeur, Valet}.String())
	assert.Equal(t, "10-pique", Carte{Pique, Dix}.String())

	for _, invalide := range []string{"", "V", "V-", "X-coeur", "V-rouge", "valet-coeur"} {
		_, err := ParseCarte(invalide)
		assert.Error(t, err, invalide)
	}
}

func TestCarteJSON(t *testing.T) {
	b, err := json.Marshal([]Carte{{Trefle, As}, {Carreau, Sept}})
	require.NoError(t, err)
	assert.JSONEq(t, `["A-trefle","7-carreau"]`, string(b))

	var cartes []Carte
	require.NoError(t, json.Unmarshal([]byte(`["D-pique"]`), &cartes))
	assert.Equal(t, []Carte{{Pique, Dame}}, cartes)
}

func TestSiege(t *testing.T) {
	assert.Equal(t, Siege(1), Siege(0).Suivant())
	assert.Equal(t, Siege(0), Siege(3).Suivant())
	assert.Equal(t, Siege(2), Siege(0).Partenaire())
	assert.Equal(t, Siege(1), Siege(3).Partenaire())
	assert.Equal(t, EquipeA, Siege(2).Equipe())
	assert.Equal(t, EquipeB, Siege(3).Equipe())
	assert.Equal(t, [2]Siege{1, 3}, EquipeB.Sieges())
	assert.Equal(t, EquipeA, EquipeB.Adverse())
}

func nouveauPli(entame Siege, cartes ...Carte) Pli {
	p := Pli{Entame: entame}
	s := entame
	for _, c := range cartes {
		p.Cartes = append(p.Cartes, CarteJouee{Siege: s, Carte: c})
		s = s.Suivant()
	}
	return p
}
