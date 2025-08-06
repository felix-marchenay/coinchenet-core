package coinche

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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

	partie.NouvelleDonne()

	assert.Len(t, partie.Paquet.Cartes, 0, "Le paquet doit être vide après distribution")
	assert.Len(t, partie.Equipe1.J1.Main.Cartes, 8)
	assert.Len(t, partie.Equipe1.J2.Main.Cartes, 8)
	assert.Len(t, partie.Equipe2.J1.Main.Cartes, 8)
	assert.Len(t, partie.Equipe2.J2.Main.Cartes, 8)

	m := Mene{}

	annonces := []Contrat{
		{ValeurAnnonce: A100, Couleur: AtoutCarreau},
		{ValeurAnnonce: A110, Couleur: AtoutCarreau},
		{ValeurAnnonce: Capot, Couleur: AtoutTrefle},
	}

	for _, annonce := range annonces {
		err := m.Annonce(partie.Equipe1, annonce)
		if err != nil {
			t.Errorf("Erreur lors de l'annonce: %v", err)
		}
	}

	partie.Menes = append(partie.Menes, m)

	partie.JoueCarte(partie.Equipe1.J1, partie.Equipe1.J1.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe1.J2, partie.Equipe1.J2.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe2.J1, partie.Equipe2.J1.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe2.J2, partie.Equipe2.J2.Main.Cartes[0])

	partie.JoueCarte(partie.Equipe1.J1, partie.Equipe1.J1.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe1.J2, partie.Equipe1.J2.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe2.J1, partie.Equipe2.J1.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe2.J2, partie.Equipe2.J2.Main.Cartes[0])

	partie.JoueCarte(partie.Equipe1.J1, partie.Equipe1.J1.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe1.J2, partie.Equipe1.J2.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe2.J1, partie.Equipe2.J1.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe2.J2, partie.Equipe2.J2.Main.Cartes[0])

	partie.JoueCarte(partie.Equipe1.J1, partie.Equipe1.J1.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe1.J2, partie.Equipe1.J2.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe2.J1, partie.Equipe2.J1.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe2.J2, partie.Equipe2.J2.Main.Cartes[0])

	partie.JoueCarte(partie.Equipe1.J1, partie.Equipe1.J1.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe1.J2, partie.Equipe1.J2.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe2.J1, partie.Equipe2.J1.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe2.J2, partie.Equipe2.J2.Main.Cartes[0])

	partie.JoueCarte(partie.Equipe1.J1, partie.Equipe1.J1.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe1.J2, partie.Equipe1.J2.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe2.J1, partie.Equipe2.J1.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe2.J2, partie.Equipe2.J2.Main.Cartes[0])

	partie.JoueCarte(partie.Equipe1.J1, partie.Equipe1.J1.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe1.J2, partie.Equipe1.J2.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe2.J1, partie.Equipe2.J1.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe2.J2, partie.Equipe2.J2.Main.Cartes[0])

	partie.JoueCarte(partie.Equipe1.J1, partie.Equipe1.J1.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe1.J2, partie.Equipe1.J2.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe2.J1, partie.Equipe2.J1.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe2.J2, partie.Equipe2.J2.Main.Cartes[0])

	assert.Len(t, partie.Menes[0].Plis[&e1], 5)
	assert.Len(t, partie.Menes[0].Plis[&e2], 3)

	assert.Equal(t, 84, partie.Score()[&e1])
	assert.Equal(t, 68, partie.Score()[&e2])
}
