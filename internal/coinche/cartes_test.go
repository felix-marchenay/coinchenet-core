package coinche

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNouveauPaquet32(t *testing.T) {

	paquet := NouveauPaquet32()
	if len(paquet.Cartes) != 32 {
		t.Errorf("Le paquet doit contenir 32 cartes, obtenu %d", len(paquet.Cartes))
	}
	m := FischerYatesMelangeur{}

	vu := make(map[Carte]bool)
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
	paquet := NouveauPaquet32()

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
		cartes        map[*Joueur]Carte
		premierJoueur *Joueur
		atout         Atout
	}
	j1 := &Joueur{Nom: "Alice"}
	j2 := &Joueur{Nom: "Bob"}
	j3 := &Joueur{Nom: "Charlie"}
	j4 := &Joueur{Nom: "David"}

	tests := []struct {
		name     string
		args     args
		expected *Joueur
	}{
		{
			name: "Sans atout, couleur demandée gagne",
			args: args{
				cartes: map[*Joueur]Carte{
					j1: {Couleur: Coeur, Valeur: Dix},
					j2: {Couleur: Coeur, Valeur: As},
					j3: {Couleur: Trefle, Valeur: Roi},
					j4: {Couleur: Pique, Valeur: Valet},
				},
				premierJoueur: j1,
				atout:         SansAtout,
			},
			expected: j2,
		},
		{
			name: "Atout gagne sur couleur demandée",
			args: args{
				cartes: map[*Joueur]Carte{
					j1: {Couleur: Coeur, Valeur: Dix},
					j2: {Couleur: Carreau, Valeur: As},
					j3: {Couleur: Coeur, Valeur: Valet},
					j4: {Couleur: Pique, Valeur: Valet},
				},
				premierJoueur: j1,
				atout:         AtoutPique,
			},
			expected: j4,
		},
		{
			name: "Premier joueur gagne si tous jouent même couleur",
			args: args{
				cartes: map[*Joueur]Carte{
					j1: {Couleur: Trefle, Valeur: Roi},
					j2: {Couleur: Trefle, Valeur: Dame},
					j3: {Couleur: Trefle, Valeur: Valet},
					j4: {Couleur: Trefle, Valeur: Sept},
				},
				premierJoueur: j1,
				atout:         SansAtout,
			},
			expected: j1,
		},
		{
			name: "Valet d'atout coupe",
			args: args{
				cartes: map[*Joueur]Carte{
					j1: {Couleur: Trefle, Valeur: Roi},
					j2: {Couleur: Trefle, Valeur: Dame},
					j3: {Couleur: Trefle, Valeur: Valet},
					j4: {Couleur: Trefle, Valeur: Sept},
				},
				premierJoueur: j2,
				atout:         AtoutTrefle,
			},
			expected: j3,
		},
		{
			name: "ToutAtout, plus forte valeur gagne",
			args: args{
				cartes: map[*Joueur]Carte{
					j1: {Couleur: Coeur, Valeur: Valet},
					j2: {Couleur: Coeur, Valeur: Neuf},
					j3: {Couleur: Pique, Valeur: Valet},
					j4: {Couleur: Trefle, Valeur: Valet},
				},
				premierJoueur: j2,
				atout:         ToutAtout,
			},
			expected: j1,
		},
		{
			name: "Premier tour classique, on fait tomber les atouts",
			args: args{
				cartes: map[*Joueur]Carte{
					j1: {Couleur: Coeur, Valeur: Valet},
					j2: {Couleur: Coeur, Valeur: Neuf},
					j3: {Couleur: Coeur, Valeur: Sept},
					j4: {Couleur: Coeur, Valeur: Huit},
				},
				premierJoueur: j1,
				atout:         AtoutCoeur,
			},
			expected: j1,
		},
		{
			name: "Personne ne fournit, personne ne coupe",
			args: args{
				cartes: map[*Joueur]Carte{
					j1: {Couleur: Carreau, Valeur: As},
					j2: {Couleur: Carreau, Valeur: Dix},
					j3: {Couleur: Coeur, Valeur: As},
					j4: {Couleur: Trefle, Valeur: Sept},
				},
				premierJoueur: j4,
				atout:         AtoutPique,
			},
			expected: j4,
		},
		{
			name: "Coupé par un atout, mais pas le premier joueur",
			args: args{
				cartes: map[*Joueur]Carte{
					j1: {Couleur: Carreau, Valeur: As},
					j2: {Couleur: Carreau, Valeur: Dix},
					j3: {Couleur: Pique, Valeur: As},
					j4: {Couleur: Trefle, Valeur: Sept},
				},
				premierJoueur: j4,
				atout:         AtoutPique,
			},
			expected: j3,
		},
		{
			name: "Aucun pli, retourne nil",
			args: args{
				cartes:        map[*Joueur]Carte{},
				premierJoueur: j1,
				atout:         SansAtout,
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pli := Pli{
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
	paquet := NouveauPaquet32()

	total := make(map[Atout]int)
	for _, carte := range paquet.Cartes {
		total[AtoutCarreau] += carte.Points(SansAtout)
		total[AtoutCoeur] += carte.Points(SansAtout)
		total[AtoutPique] += carte.Points(SansAtout)
		total[AtoutTrefle] += carte.Points(SansAtout)
		total[SansAtout] += carte.Points(SansAtout)
		total[ToutAtout] += carte.Points(SansAtout)
	}

	for atout, points := range total {
		assert.Equal(t, 152, points, "La somme des points pour %v doit être 152, obtenu %d", atout, points)
	}
}

func TestCouper(t *testing.T) {

	cp := RandomCoupeur{}
	paquet := NouveauPaquet32()

	p1 := paquet.Cartes

	paquet = cp.Couper(paquet)

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
