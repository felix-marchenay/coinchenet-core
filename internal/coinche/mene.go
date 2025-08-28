package coinche

import "fmt"

type Mene struct {
	Contrat  Contrat
	Preneuse *Equipe

	Plis       map[*Equipe][]Pli
	pliEnCours Pli
}

func (m Mene) Terminée() bool {
	nb := 0

	for _, p := range m.Plis {
		nb += len(p)
	}

	return nb >= 8
}

func (m Mene) Démarrée() bool {
	nb := 0

	for _, p := range m.Plis {
		nb += len(p)
	}

	return nb > 0
}

func (m *Mene) Annonce(e *Equipe, c Contrat) error {
	if m.Contrat.ValeurAnnonce >= c.ValeurAnnonce {
		return fmt.Errorf("Annonce %v doit être supérieure à %v", c.ValeurAnnonce, m.Contrat.ValeurAnnonce)
	}

	m.Contrat = c
	m.Preneuse = e

	return nil
}
