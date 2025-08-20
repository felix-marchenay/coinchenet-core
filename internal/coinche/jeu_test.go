package coinche

import (
	"fmt"
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

	partie.NouvelleMene()

	assert.Len(t, partie.Paquet.Cartes, 0, "Le paquet doit être vide après distribution")
	assert.Len(t, partie.Equipe1.J1.Main.Cartes, 8)
	assert.Len(t, partie.Equipe1.J2.Main.Cartes, 8)
	assert.Len(t, partie.Equipe2.J1.Main.Cartes, 8)
	assert.Len(t, partie.Equipe2.J2.Main.Cartes, 8)

	partie.Annonce(partie.Equipe1.J1, Contrat{AtoutCarreau, A100})
	partie.Annonce(partie.Equipe1.J2, Contrat{AtoutCarreau, A110})
	partie.Annonce(partie.Equipe1.J1, Contrat{AtoutCarreau, Capot})

	for range 8 {
		partie.JoueCarte(partie.Equipe1.J1, partie.Equipe1.J1.Main.Cartes[0])
		partie.JoueCarte(partie.Equipe1.J2, partie.Equipe1.J2.Main.Cartes[0])
		partie.JoueCarte(partie.Equipe2.J1, partie.Equipe2.J1.Main.Cartes[0])
		partie.JoueCarte(partie.Equipe2.J2, partie.Equipe2.J2.Main.Cartes[0])
	}

	err := partie.JoueCarte(partie.Equipe1.J1, Carte{Coeur, Valet})

	assert.Equal(t, "impossible de jouer la carte : carte {0 4} non trouvée", err.Error())

	assert.Len(t, partie.Menes, 1)

	assert.Len(t, partie.Menes[0].Plis[&e1], 5)
	assert.Len(t, partie.Menes[0].Plis[&e2], 3)

	assert.Equal(t, 80, partie.Score()[&e1])
	assert.Equal(t, 72, partie.Score()[&e2])

	partie.NouvelleMene()

	assert.Len(t, partie.Menes, 2)

	partie.Annonce(e1.J1, Contrat{AtoutPique, A80})
	partie.Annonce(e2.J1, Contrat{AtoutCoeur, A90})
	partie.Annonce(e1.J2, Contrat{AtoutPique, A120})

	fmt.Println(SerializeCarte(partie.Equipe1.J1.Main.Cartes[0]))
	fmt.Println(SerializeCarte(partie.Equipe1.J2.Main.Cartes[0]))
	for range 8 {
		partie.JoueCarte(partie.Equipe1.J1, partie.Equipe1.J1.Main.Cartes[0])
		partie.JoueCarte(partie.Equipe1.J2, partie.Equipe1.J2.Main.Cartes[0])
		partie.JoueCarte(partie.Equipe2.J1, partie.Equipe2.J1.Main.Cartes[0])
		partie.JoueCarte(partie.Equipe2.J2, partie.Equipe2.J2.Main.Cartes[0])
	}
}
