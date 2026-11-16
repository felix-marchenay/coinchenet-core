// Package classement calcule l'évolution de l'ELO individuel après une partie classée.
package classement

import (
	"math"

	"github.com/google/uuid"

	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
)

const (
	EloInitial  = 1200
	EloPlancher = 100
	// KDebutant s'applique pendant les SeuilConfirme premières parties classées.
	KDebutant     = 40
	KConfirme     = 20
	SeuilConfirme = 30
	// SeuilClassement est le nombre de parties à jouer pour apparaître au classement public.
	SeuilClassement = 10
	PenaliteAbandon = 15
)

type Participant struct {
	JoueurID uuid.UUID
	Equipe   coinche.Equipe
	Elo      int
	// NbParties compte les parties classées jouées avant celle-ci.
	NbParties int
	Abandon   bool
}

type Variation struct {
	JoueurID uuid.UUID `json:"joueur"`
	Avant    int       `json:"avant"`
	Apres    int       `json:"apres"`
	Delta    int       `json:"delta"`
}

func K(nbParties int) int {
	if nbParties < SeuilConfirme {
		return KDebutant
	}
	return KConfirme
}

// Esperance est la probabilité de victoire attendue d'une équipe face à une autre.
func Esperance(eloEquipe, eloAdverse float64) float64 {
	return 1 / (1 + math.Pow(10, (eloAdverse-eloEquipe)/400))
}

// Calculer renvoie la variation de chaque participant, dans le même ordre.
// En cas d'abandon, l'auteur subit une pénalité et son partenaire ne perd rien.
func Calculer(participants []Participant, gagnante coinche.Equipe) []Variation {
	var somme, nb [2]float64
	var abandon [2]bool
	for _, p := range participants {
		somme[p.Equipe] += float64(p.Elo)
		nb[p.Equipe]++
		abandon[p.Equipe] = abandon[p.Equipe] || p.Abandon
	}
	moyenne := func(e coinche.Equipe) float64 {
		if nb[e] == 0 {
			return EloInitial
		}
		return somme[e] / nb[e]
	}

	variations := make([]Variation, len(participants))
	for i, p := range participants {
		score := 0.0
		if p.Equipe == gagnante {
			score = 1
		}
		esperance := Esperance(moyenne(p.Equipe), moyenne(p.Equipe.Adverse()))
		delta := int(math.Round(float64(K(p.NbParties)) * (score - esperance)))

		switch {
		case p.Abandon:
			delta -= PenaliteAbandon
		case abandon[p.Equipe] && p.Equipe != gagnante:
			delta = 0
		}

		apres := max(p.Elo+delta, EloPlancher)
		variations[i] = Variation{JoueurID: p.JoueurID, Avant: p.Elo, Apres: apres, Delta: apres - p.Elo}
	}
	return variations
}
