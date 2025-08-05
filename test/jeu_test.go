package test

import (
	"coinchenetcore/src"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNouvellePartie(t *testing.T) {
	partie := src.Partie{
		Equipe1: src.Equipe{
			J1:    &src.Joueur{Nom: "Alice"},
			J2:    &src.Joueur{Nom: "Bob"},
			Score: 0,
		},
		Equipe2: src.Equipe{
			J1:    &src.Joueur{Nom: "Charles"},
			J2:    &src.Joueur{Nom: "Dicky"},
			Score: 0,
		},
		Paquet:    src.NouveauPaquet32(),
		Menes:     []src.Mene{},
		Melangeur: &FakeMelangeur{},
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

	m := src.Mene{}

	partie.Menes = append(partie.Menes, m)

	annonces := []src.Contrat{
		{ValeurAnnonce: src.A100, Couleur: src.AtoutCarreau},
		{ValeurAnnonce: src.A110, Couleur: src.AtoutCarreau},
		{ValeurAnnonce: src.Capot, Couleur: src.AtoutTrefle},
	}

	for _, annonce := range annonces {
		err := m.Annonce(&partie.Equipe1, annonce)
		if err != nil {
			t.Errorf("Erreur lors de l'annonce: %v", err)
		}
	}

	fmt.Printf("%s", SerializePaquet(partie.Equipe1.J1.Main))
	fmt.Printf("%s", SerializePaquet(partie.Equipe1.J2.Main))

	partie.JoueCarte(partie.Equipe1.J1, partie.Equipe1.J1.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe1.J2, partie.Equipe1.J2.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe2.J1, partie.Equipe2.J1.Main.Cartes[0])
	partie.JoueCarte(partie.Equipe2.J2, partie.Equipe2.J2.Main.Cartes[0])

	assert.Len(t, partie.Equipe1.J1.Main.Cartes, 7)
	assert.Len(t, partie.Equipe1.J2.Main.Cartes, 7)
	assert.Len(t, partie.Equipe2.J1.Main.Cartes, 7)
	assert.Len(t, partie.Equipe2.J2.Main.Cartes, 7)
}
