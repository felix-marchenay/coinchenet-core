package test

import (
	"coinchenetcore/src"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func SerializePaquet(p src.CarteCollection) string {
	var result string
	for _, carte := range p.Cartes {
		result += SerializeCarte(carte) + ", "
	}
	return "Paquet{" + result + "}\n"
}

func SerializeCarte(c src.Carte) string {
	cstr := ""
	switch c.Couleur {
	case src.Coeur:
		cstr = "Coeur"
	case src.Carreau:
		cstr = "Carreau"
	case src.Trefle:
		cstr = "Trefle"
	case src.Pique:
		cstr = "Pique"
	default:
		cstr = "-"
	}

	vstr := ""
	switch c.Valeur {
	case src.Sept:
		vstr = "Sept"
	case src.Huit:
		vstr = "Huit"
	case src.Neuf:
		vstr = "Neuf"
	case src.Dix:
		vstr = "Dix"
	case src.Valet:
		vstr = "Valet"
	case src.Dame:
		vstr = "Dame"
	case src.Roi:
		vstr = "Roi"
	case src.As:
		vstr = "As"
	default:
		vstr = "Inconnu"
	}

	return fmt.Sprintf("%s %s", vstr, cstr)
}

func TestNouveauPaquet32(t *testing.T) {

	paquet := src.NouveauPaquet32()
	if len(paquet.Cartes) != 32 {
		t.Errorf("Le paquet doit contenir 32 cartes, obtenu %d", len(paquet.Cartes))
	}
	m := src.FischerYatesMelangeur{}

	vu := make(map[src.Carte]bool)
	for _, c := range paquet.Cartes {
		if vu[c] {
			t.Errorf("Carte dupliquée: %+v", c)
		}
		vu[c] = true
	}

	for range 10 {
		in := SerializePaquet(paquet)

		paquet = m.Melanger(paquet)

		assert.NotEqual(t, in, SerializePaquet(paquet), "Le paquet mélangé doit contenir les mêmes cartes")
	}

	if len(paquet.Cartes) != 32 {
		t.Errorf("Le paquet doit contenir 32 cartes après mélange, obtenu %d", len(paquet.Cartes))
	}
}

func TestTirer(t *testing.T) {
	paquet := src.NouveauPaquet32()

	cartesTirees := paquet.Tirer(5)
	if len(cartesTirees) != 5 {
		t.Errorf("Doit tirer 5 cartes, obtenu %d", len(cartesTirees))
	}

	if len(paquet.Cartes) != 27 {
		t.Errorf("Le paquet doit contenir 27 cartes après tirage, obtenu %d", len(paquet.Cartes))
	}

	cartesTirees = paquet.Tirer(50)
	if cartesTirees != nil {
		t.Error("Doit retourner nil si le nombre de cartes à tirer est supérieur au nombre de cartes restantes")
	}
}

func TestPliRemporte_Provider(t *testing.T) {
	type args struct {
		cartes        map[*src.Joueur]*src.Carte
		premierJoueur *src.Joueur
		atout         src.Atout
	}
	j1 := &src.Joueur{Nom: "Alice"}
	j2 := &src.Joueur{Nom: "Bob"}
	j3 := &src.Joueur{Nom: "Charlie"}
	j4 := &src.Joueur{Nom: "David"}

	tests := []struct {
		name     string
		args     args
		expected *src.Joueur
	}{
		{
			name: "Sans atout, couleur demandée gagne",
			args: args{
				cartes: map[*src.Joueur]*src.Carte{
					j1: {Couleur: src.Coeur, Valeur: src.Dix},
					j2: {Couleur: src.Coeur, Valeur: src.As},
					j3: {Couleur: src.Trefle, Valeur: src.Roi},
					j4: {Couleur: src.Pique, Valeur: src.Valet},
				},
				premierJoueur: j1,
				atout:         src.SansAtout,
			},
			expected: j2,
		},
		{
			name: "Atout gagne sur couleur demandée",
			args: args{
				cartes: map[*src.Joueur]*src.Carte{
					j1: {Couleur: src.Coeur, Valeur: src.Dix},
					j2: {Couleur: src.Carreau, Valeur: src.As},
					j3: {Couleur: src.Coeur, Valeur: src.Valet},
					j4: {Couleur: src.Pique, Valeur: src.Valet},
				},
				premierJoueur: j1,
				atout:         src.AtoutPique,
			},
			expected: j4,
		},
		{
			name: "Premier joueur gagne si tous jouent même couleur",
			args: args{
				cartes: map[*src.Joueur]*src.Carte{
					j1: {Couleur: src.Trefle, Valeur: src.Roi},
					j2: {Couleur: src.Trefle, Valeur: src.Dame},
					j3: {Couleur: src.Trefle, Valeur: src.Valet},
					j4: {Couleur: src.Trefle, Valeur: src.Sept},
				},
				premierJoueur: j1,
				atout:         src.SansAtout,
			},
			expected: j1,
		},
		{
			name: "Valet d'atout coupe",
			args: args{
				cartes: map[*src.Joueur]*src.Carte{
					j1: {Couleur: src.Trefle, Valeur: src.Roi},
					j2: {Couleur: src.Trefle, Valeur: src.Dame},
					j3: {Couleur: src.Trefle, Valeur: src.Valet},
					j4: {Couleur: src.Trefle, Valeur: src.Sept},
				},
				premierJoueur: j2,
				atout:         src.AtoutTrefle,
			},
			expected: j3,
		},
		{
			name: "ToutAtout, plus forte valeur gagne",
			args: args{
				cartes: map[*src.Joueur]*src.Carte{
					j1: {Couleur: src.Coeur, Valeur: src.Valet},
					j2: {Couleur: src.Coeur, Valeur: src.Neuf},
					j3: {Couleur: src.Pique, Valeur: src.Valet},
					j4: {Couleur: src.Trefle, Valeur: src.Valet},
				},
				premierJoueur: j2,
				atout:         src.ToutAtout,
			},
			expected: j1,
		},
		{
			name: "Premier tour classique, on fait tomber les atouts",
			args: args{
				cartes: map[*src.Joueur]*src.Carte{
					j1: {Couleur: src.Coeur, Valeur: src.Valet},
					j2: {Couleur: src.Coeur, Valeur: src.Neuf},
					j3: {Couleur: src.Coeur, Valeur: src.Sept},
					j4: {Couleur: src.Coeur, Valeur: src.Huit},
				},
				premierJoueur: j1,
				atout:         src.AtoutCoeur,
			},
			expected: j1,
		},
		{
			name: "Personne ne fournit, personne ne coupe",
			args: args{
				cartes: map[*src.Joueur]*src.Carte{
					j1: {Couleur: src.Carreau, Valeur: src.As},
					j2: {Couleur: src.Carreau, Valeur: src.Dix},
					j3: {Couleur: src.Coeur, Valeur: src.As},
					j4: {Couleur: src.Trefle, Valeur: src.Sept},
				},
				premierJoueur: j4,
				atout:         src.AtoutPique,
			},
			expected: j4,
		},
		{
			name: "Coupé par un atout, mais pas le premier joueur",
			args: args{
				cartes: map[*src.Joueur]*src.Carte{
					j1: {Couleur: src.Carreau, Valeur: src.As},
					j2: {Couleur: src.Carreau, Valeur: src.Dix},
					j3: {Couleur: src.Pique, Valeur: src.As},
					j4: {Couleur: src.Trefle, Valeur: src.Sept},
				},
				premierJoueur: j4,
				atout:         src.AtoutPique,
			},
			expected: j3,
		},
		{
			name: "Aucun pli, retourne nil",
			args: args{
				cartes:        map[*src.Joueur]*src.Carte{},
				premierJoueur: j1,
				atout:         src.SansAtout,
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pli := src.Pli{
				Cartes:        tt.args.cartes,
				PremierJoueur: tt.args.premierJoueur,
			}
			res := pli.Remporte(tt.args.atout)
			if tt.expected == nil {
				assert.Nil(t, res)
			} else {
				assert.Equal(t, tt.expected.Nom, res.Nom)
			}
		})
	}
}

func TestPoints162(t *testing.T) {
	paquet := src.NouveauPaquet32()

	total := make(map[src.Atout]int)
	for _, carte := range paquet.Cartes {
		total[src.AtoutCarreau] += carte.Points(src.SansAtout)
		total[src.AtoutCoeur] += carte.Points(src.SansAtout)
		total[src.AtoutPique] += carte.Points(src.SansAtout)
		total[src.AtoutTrefle] += carte.Points(src.SansAtout)
		total[src.SansAtout] += carte.Points(src.SansAtout)
		total[src.ToutAtout] += carte.Points(src.SansAtout)
	}

	for atout, points := range total {
		assert.Equal(t, 152, points, "La somme des points pour %v doit être 152, obtenu %d", atout, points)
	}
}

func TestCouper(t *testing.T) {

	paquet := src.NouveauPaquet32()

	p1 := paquet.Cartes

	paquet.Couper()

	p2 := paquet.Cartes

	assert.NotEqual(t, p1, p2)
	assert.Len(t, p2, len(p1))

	var ci int
	for i := range p1 {
		if p2[i] == p1[0] {
			ci = i
			break
		}
	}

	assert.Greater(t, ci, 0)
	assert.LessOrEqual(t, ci, len(p1)-1)

	assert.Equal(t, p1[0], p2[ci])
	assert.Equal(t, p1[1], p2[ci+1])
	assert.Equal(t, p2[ci], p1[0])
	assert.Equal(t, p2[ci+1], p1[1])
}
