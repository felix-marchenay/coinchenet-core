package coinche

import "fmt"

type Atout int

const (
	AtoutCoeur Atout = iota
	AtoutCarreau
	AtoutTrefle
	AtoutPique
	SansAtout
	ToutAtout
)

var Atouts = [...]Atout{AtoutCoeur, AtoutCarreau, AtoutTrefle, AtoutPique, SansAtout, ToutAtout}

var nomsAtouts = [...]string{
	AtoutCoeur:   "coeur",
	AtoutCarreau: "carreau",
	AtoutTrefle:  "trefle",
	AtoutPique:   "pique",
	SansAtout:    "sans-atout",
	ToutAtout:    "tout-atout",
}

func (a Atout) Valide() bool {
	return a >= AtoutCoeur && a <= ToutAtout
}

// EstCouleur indique un contrat à la couleur (par opposition à sans-atout / tout-atout).
func (a Atout) EstCouleur() bool {
	return a >= AtoutCoeur && a <= AtoutPique
}

func (a Atout) String() string {
	if !a.Valide() {
		return fmt.Sprintf("atout(%d)", int(a))
	}
	return nomsAtouts[a]
}

func ParseAtout(s string) (Atout, error) {
	for a, nom := range nomsAtouts {
		if nom == s {
			return Atout(a), nil
		}
	}
	return 0, fmt.Errorf("atout inconnu %q", s)
}

func (a Atout) MarshalText() ([]byte, error) {
	if !a.Valide() {
		return nil, fmt.Errorf("atout invalide %d", int(a))
	}
	return []byte(a.String()), nil
}

func (a *Atout) UnmarshalText(b []byte) error {
	v, err := ParseAtout(string(b))
	if err != nil {
		return err
	}
	*a = v
	return nil
}

type ValeurAnnonce int

const (
	A80      ValeurAnnonce = 80
	A90      ValeurAnnonce = 90
	A100     ValeurAnnonce = 100
	A110     ValeurAnnonce = 110
	A120     ValeurAnnonce = 120
	A130     ValeurAnnonce = 130
	A140     ValeurAnnonce = 140
	A150     ValeurAnnonce = 150
	A160     ValeurAnnonce = 160
	Capot    ValeurAnnonce = 250
	Generale ValeurAnnonce = 500
)

var ValeursAnnonce = [...]ValeurAnnonce{A80, A90, A100, A110, A120, A130, A140, A150, A160, Capot, Generale}

func (v ValeurAnnonce) Valide() bool {
	for _, va := range ValeursAnnonce {
		if va == v {
			return true
		}
	}
	return false
}

// Suivante renvoie la plus petite annonce strictement supérieure, ou 0 s'il n'y en a pas.
func (v ValeurAnnonce) Suivante() ValeurAnnonce {
	for _, va := range ValeursAnnonce {
		if va > v {
			return va
		}
	}
	return 0
}

type Contrat struct {
	Atout  Atout         `json:"atout"`
	Valeur ValeurAnnonce `json:"valeur"`
}

func (c Contrat) Valide() bool {
	return c.Atout.Valide() && c.Valeur.Valide()
}

func (c Contrat) String() string {
	switch c.Valeur {
	case Capot:
		return "capot " + c.Atout.String()
	case Generale:
		return "générale " + c.Atout.String()
	default:
		return fmt.Sprintf("%d %s", c.Valeur, c.Atout)
	}
}
