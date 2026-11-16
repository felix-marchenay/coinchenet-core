package coinche

import "fmt"

// Siege numérote les places autour de la table, dans l'ordre de jeu.
// Les partenaires sont face à face : l'équipe A occupe 0 et 2, l'équipe B 1 et 3.
type Siege int

const NbSieges = 4

var Sieges = [...]Siege{0, 1, 2, 3}

func (s Siege) Valide() bool {
	return s >= 0 && s < NbSieges
}

func (s Siege) Suivant() Siege {
	return (s + 1) % NbSieges
}

func (s Siege) Partenaire() Siege {
	return (s + 2) % NbSieges
}

func (s Siege) Equipe() Equipe {
	return Equipe(s % 2)
}

func (s Siege) String() string {
	return fmt.Sprintf("siège %d", int(s))
}

type Equipe int

const (
	EquipeA Equipe = iota
	EquipeB
)

func (e Equipe) Adverse() Equipe {
	return 1 - e
}

func (e Equipe) Sieges() [2]Siege {
	return [2]Siege{Siege(e), Siege(e) + 2}
}

func (e Equipe) String() string {
	if e == EquipeA {
		return "A"
	}
	return "B"
}
