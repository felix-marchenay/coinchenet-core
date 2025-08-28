package coinche

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPeutJouerCarte(t *testing.T) {
	J1 := Joueur{Nom: "Alice"}
	J2 := Joueur{Nom: "Bob"}
	J3 := Joueur{Nom: "Charles"}
	J4 := Joueur{Nom: "Dicky", Main: CarteCollection{
		Cartes: []Carte{
			{Pique, Dix},
			{Carreau, Dix},
			{Coeur, Dame},
			{Coeur, Valet},
			{Trefle, Valet},
		},
	}}

	pli := Pli{
		Cartes: map[*Joueur]Carte{
			&J1: {Carreau, Valet},
			&J2: {Coeur, Neuf},
			&J3: {Carreau, Dame},
		},
		PremierJoueur: &J3,
	}

	assert.False(t, J4.PeutJouer(Carte{Carreau, As}, pli, AtoutCarreau))

	assert.False(t, J4.PeutJouer(Carte{Pique, Dix}, pli, AtoutCarreau))
	assert.True(t, J4.PeutJouer(Carte{Carreau, Dix}, pli, AtoutCarreau))
	assert.False(t, J4.PeutJouer(Carte{Coeur, Dame}, pli, AtoutCarreau))
	assert.False(t, J4.PeutJouer(Carte{Coeur, Valet}, pli, AtoutCarreau))
	assert.False(t, J4.PeutJouer(Carte{Trefle, Valet}, pli, AtoutCarreau))

	assert.False(t, J4.PeutJouer(Carte{Pique, Dix}, pli, AtoutPique))
	assert.True(t, J4.PeutJouer(Carte{Carreau, Dix}, pli, AtoutPique))
	assert.False(t, J4.PeutJouer(Carte{Coeur, Dame}, pli, AtoutPique))
	assert.False(t, J4.PeutJouer(Carte{Coeur, Valet}, pli, AtoutPique))
	assert.False(t, J4.PeutJouer(Carte{Trefle, Valet}, pli, AtoutPique))

	assert.False(t, J4.PeutJouer(Carte{Pique, Dix}, pli, SansAtout))
	assert.True(t, J4.PeutJouer(Carte{Carreau, Dix}, pli, SansAtout))
	assert.False(t, J4.PeutJouer(Carte{Coeur, Dame}, pli, SansAtout))
	assert.False(t, J4.PeutJouer(Carte{Coeur, Valet}, pli, SansAtout))
	assert.False(t, J4.PeutJouer(Carte{Trefle, Valet}, pli, SansAtout))

	J4 = Joueur{Nom: "Dicky", Main: CarteCollection{
		Cartes: []Carte{
			{Coeur, Dame},
			{Coeur, Valet},
			{Trefle, Valet},
		},
	}}
	pli = Pli{
		Cartes: map[*Joueur]Carte{
			&J2: {Coeur, Neuf},
			&J3: {Trefle, Dame},
		},
		PremierJoueur: &J3,
	}

	assert.False(t, J4.PeutJouer(Carte{Coeur, Dame}, pli, AtoutCoeur))
	assert.False(t, J4.PeutJouer(Carte{Coeur, Valet}, pli, AtoutCoeur))
	assert.True(t, J4.PeutJouer(Carte{Trefle, Valet}, pli, AtoutCoeur))

	assert.False(t, J4.PeutJouer(Carte{Coeur, Dame}, pli, SansAtout))
	assert.False(t, J4.PeutJouer(Carte{Coeur, Valet}, pli, SansAtout))
	assert.True(t, J4.PeutJouer(Carte{Trefle, Valet}, pli, SansAtout))

	pli = Pli{
		Cartes: map[*Joueur]Carte{
			&J2: {Coeur, Neuf},
			&J3: {Pique, Dame},
		},
		PremierJoueur: &J3,
	}

	assert.True(t, J4.PeutJouer(Carte{Coeur, Dame}, pli, AtoutCoeur))
	assert.True(t, J4.PeutJouer(Carte{Coeur, Valet}, pli, AtoutCoeur))
	assert.False(t, J4.PeutJouer(Carte{Trefle, Valet}, pli, AtoutCoeur))

	assert.True(t, J4.PeutJouer(Carte{Coeur, Dame}, pli, SansAtout))
	assert.True(t, J4.PeutJouer(Carte{Coeur, Valet}, pli, SansAtout))
	assert.True(t, J4.PeutJouer(Carte{Trefle, Valet}, pli, SansAtout))

	assert.False(t, J4.PeutJouer(Carte{Coeur, Dame}, pli, AtoutTrefle))
	assert.False(t, J4.PeutJouer(Carte{Coeur, Valet}, pli, AtoutTrefle))
	assert.True(t, J4.PeutJouer(Carte{Trefle, Valet}, pli, AtoutTrefle))

	J4 = Joueur{Nom: "Dicky", Main: CarteCollection{
		Cartes: []Carte{
			{Coeur, Dame},
			{Coeur, Valet},
			{Trefle, Valet},
		},
	}}
	pli = Pli{
		Cartes: map[*Joueur]Carte{
			&J2: {Coeur, Neuf},
			&J3: {Trefle, Dame},
		},
		PremierJoueur: &J2,
	}

	assert.False(t, J4.PeutJouer(Carte{Coeur, Dame}, pli, AtoutCoeur))
	assert.True(t, J4.PeutJouer(Carte{Coeur, Valet}, pli, AtoutCoeur))
	assert.False(t, J4.PeutJouer(Carte{Trefle, Valet}, pli, AtoutCoeur))

	assert.False(t, J4.PeutJouer(Carte{Coeur, Dame}, pli, ToutAtout))
	assert.True(t, J4.PeutJouer(Carte{Coeur, Valet}, pli, ToutAtout))
	assert.False(t, J4.PeutJouer(Carte{Trefle, Valet}, pli, ToutAtout))

	assert.True(t, J4.PeutJouer(Carte{Coeur, Dame}, pli, AtoutPique))
	assert.True(t, J4.PeutJouer(Carte{Coeur, Valet}, pli, AtoutPique))
	assert.False(t, J4.PeutJouer(Carte{Trefle, Valet}, pli, AtoutPique))
}

func TestNouvellePartie(t *testing.T) {
	e1 := Equipe{
		J1: &Joueur{Nom: "Alice"},
		J2: &Joueur{Nom: "Bob"},
	}

	e2 := Equipe{
		J1: &Joueur{Nom: "Charles"},
		J2: &Joueur{Nom: "Dicky"},
	}

	partie := Partie{
		Equipe1:   &e1,
		Equipe2:   &e2,
		Paquet:    NouveauPaquet32(),
		Menes:     []Mene{},
		Melangeur: FakeMelangeur{},
		Coupeur:   NoCoupeur{},
	}

	if len(partie.Paquet.Cartes) != 32 {
		t.Errorf("Le paquet doit contenir 32 cartes, obtenu %d", len(partie.Paquet.Cartes))
	}

	assert.Len(t, partie.Paquet.Cartes, 32, "Le paquet doit contenir 32 cartes au début")

	partie.NouvelleMene()

	assert.Len(t, partie.Paquet.Cartes, 0, "Le paquet doit être vide après distribution")
	assert.Len(t, partie.Equipe1.J1.Main.Cartes, 8)
	assert.Len(t, partie.Equipe1.J2.Main.Cartes, 8)
	assert.Len(t, partie.Equipe2.J1.Main.Cartes, 8)
	assert.Len(t, partie.Equipe2.J2.Main.Cartes, 8)

	partie.Annonce(partie.Equipe1.J1, Contrat{AtoutCarreau, A100})
	partie.Annonce(partie.Equipe1.J2, Contrat{AtoutCarreau, A110})
	partie.Annonce(partie.Equipe1.J1, Contrat{AtoutCarreau, Capot})

	fmt.Println("ff", partie.Menes[0].Contrat)
	fmt.Println(SerializePaquet(partie.Equipe1.J1.Main))
	fmt.Println(SerializePaquet(partie.Equipe1.J2.Main))
	fmt.Println(SerializePaquet(partie.Equipe2.J1.Main))
	fmt.Println(SerializePaquet(partie.Equipe2.J2.Main))

	partie.JoueCarte(partie.Equipe1.J1, Carte{Pique, Dame})
	partie.JoueCarte(partie.Equipe1.J2, Carte{Pique, Neuf})
	partie.JoueCarte(partie.Equipe2.J1, Carte{Pique, Sept})
	partie.JoueCarte(partie.Equipe2.J2, Carte{Carreau, Valet})

	err := partie.JoueCarte(partie.Equipe1.J1, Carte{Coeur, Valet})

	assert.Equal(t, "carte interdite", err.Error())

	assert.Len(t, partie.Menes, 1)

	assert.Len(t, partie.Menes[0].Plis[&e1], 0)
	assert.Len(t, partie.Menes[0].Plis[&e2], 1)

	assert.Equal(t, 0, partie.Score()[&e1])
	assert.Equal(t, 23, partie.Score()[&e2])

	partie.NouvelleMene()

	assert.Len(t, partie.Menes, 1)
}
