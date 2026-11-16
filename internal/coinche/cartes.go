package coinche

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

type Couleur int

const (
	Coeur Couleur = iota
	Carreau
	Trefle
	Pique
)

var Couleurs = [...]Couleur{Coeur, Carreau, Trefle, Pique}

var nomsCouleurs = [...]string{Coeur: "coeur", Carreau: "carreau", Trefle: "trefle", Pique: "pique"}

func (c Couleur) Valide() bool {
	return c >= Coeur && c <= Pique
}

func (c Couleur) String() string {
	if !c.Valide() {
		return fmt.Sprintf("couleur(%d)", int(c))
	}
	return nomsCouleurs[c]
}

func ParseCouleur(s string) (Couleur, error) {
	for c, nom := range nomsCouleurs {
		if nom == s {
			return Couleur(c), nil
		}
	}
	return 0, fmt.Errorf("couleur inconnue %q", s)
}

type Valeur int

const (
	Sept Valeur = iota
	Huit
	Neuf
	Dix
	Valet
	Dame
	Roi
	As
)

var Valeurs = [...]Valeur{Sept, Huit, Neuf, Dix, Valet, Dame, Roi, As}

var nomsValeurs = [...]string{Sept: "7", Huit: "8", Neuf: "9", Dix: "10", Valet: "V", Dame: "D", Roi: "R", As: "A"}

func (v Valeur) Valide() bool {
	return v >= Sept && v <= As
}

func (v Valeur) String() string {
	if !v.Valide() {
		return fmt.Sprintf("valeur(%d)", int(v))
	}
	return nomsValeurs[v]
}

func ParseValeur(s string) (Valeur, error) {
	for v, nom := range nomsValeurs {
		if nom == s {
			return Valeur(v), nil
		}
	}
	return 0, fmt.Errorf("valeur inconnue %q", s)
}

// Rangs croissants : à l'atout (7 8 D R 10 A 9 V) et hors atout (7 8 9 V D R 10 A).
var (
	rangAtout   = [...]int{Sept: 0, Huit: 1, Dame: 2, Roi: 3, Dix: 4, As: 5, Neuf: 6, Valet: 7}
	rangCouleur = [...]int{Sept: 0, Huit: 1, Neuf: 2, Valet: 3, Dame: 4, Roi: 5, Dix: 6, As: 7}
)

type Carte struct {
	Couleur Couleur
	Valeur  Valeur
}

func (c Carte) Valide() bool {
	return c.Couleur.Valide() && c.Valeur.Valide()
}

// String renvoie la notation utilisée par l'API : "V-coeur", "10-pique"…
func (c Carte) String() string {
	return c.Valeur.String() + "-" + c.Couleur.String()
}

func ParseCarte(s string) (Carte, error) {
	valeur, couleur, ok := strings.Cut(s, "-")
	if !ok {
		return Carte{}, fmt.Errorf("carte invalide %q", s)
	}
	v, err := ParseValeur(valeur)
	if err != nil {
		return Carte{}, fmt.Errorf("carte invalide %q : %w", s, err)
	}
	co, err := ParseCouleur(couleur)
	if err != nil {
		return Carte{}, fmt.Errorf("carte invalide %q : %w", s, err)
	}
	return Carte{Couleur: co, Valeur: v}, nil
}

func (c Carte) MarshalText() ([]byte, error) {
	if !c.Valide() {
		return nil, fmt.Errorf("carte invalide %+v", struct{ Couleur, Valeur int }{int(c.Couleur), int(c.Valeur)})
	}
	return []byte(c.String()), nil
}

func (c *Carte) UnmarshalText(b []byte) error {
	carte, err := ParseCarte(string(b))
	if err != nil {
		return err
	}
	*c = carte
	return nil
}

// EstAtout indique si la carte se joue comme un atout (ordre et obligation de monter).
func (c Carte) EstAtout(a Atout) bool {
	return a == ToutAtout || (a.EstCouleur() && Couleur(a) == c.Couleur)
}

// Rang donne la force de la carte dans sa couleur, selon l'atout.
func (c Carte) Rang(a Atout) int {
	if c.EstAtout(a) {
		return rangAtout[c.Valeur]
	}
	return rangCouleur[c.Valeur]
}

func (c Carte) Points(a Atout) int {
	if a == ToutAtout {
		switch c.Valeur {
		case Valet:
			return 14
		case Neuf:
			return 9
		case As:
			return 6
		case Dix:
			return 5
		case Roi:
			return 3
		case Dame:
			return 1
		default:
			return 0
		}
	}

	if a == SansAtout {
		switch c.Valeur {
		case As:
			return 19
		case Dix:
			return 10
		case Roi:
			return 4
		case Dame:
			return 3
		case Valet:
			return 2
		default:
			return 0
		}
	}

	if Atout(c.Couleur) == a {
		switch c.Valeur {
		case Valet:
			return 20
		case Neuf:
			return 14
		case As:
			return 11
		case Dix:
			return 10
		case Roi:
			return 4
		case Dame:
			return 3
		default:
			return 0
		}
	}

	switch c.Valeur {
	case As:
		return 11
	case Dix:
		return 10
	case Roi:
		return 4
	case Dame:
		return 3
	case Valet:
		return 2
	default:
		return 0
	}
}

// Bat indique si c l'emporte sur c2 dans un pli dont la couleur demandée est donnée.
func (c *Carte) Bat(c2 *Carte, atout Atout, couleurDemandee Couleur) bool {
	return c.force(atout, couleurDemandee) > c2.force(atout, couleurDemandee)
}

func (c Carte) force(a Atout, couleurDemandee Couleur) int {
	switch {
	case a.EstCouleur() && Couleur(a) == c.Couleur:
		return 200 + c.Rang(a)
	case c.Couleur == couleurDemandee:
		return 100 + c.Rang(a)
	default:
		return c.Rang(a)
	}
}

type CarteCollection struct {
	Cartes []Carte
}

func NouveauPaquet32() CarteCollection {
	cartes := make([]Carte, 0, 32)
	for _, c := range Couleurs {
		for _, v := range Valeurs {
			cartes = append(cartes, Carte{Couleur: c, Valeur: v})
		}
	}
	return CarteCollection{Cartes: cartes}
}

func (cc *CarteCollection) Pop(carte Carte) (Carte, error) {
	for i, c := range cc.Cartes {
		if c == carte {
			cc.Cartes = append(cc.Cartes[:i], cc.Cartes[i+1:]...)
			return c, nil
		}
	}
	return Carte{}, fmt.Errorf("carte %v non trouvée", carte)
}

func (cc CarteCollection) Contient(carte Carte) bool {
	for _, c := range cc.Cartes {
		if c == carte {
			return true
		}
	}
	return false
}

// Tirer retire nb cartes du dessus du paquet (la fin de la slice).
func (cc *CarteCollection) Tirer(nb int) []Carte {
	d := len(cc.Cartes)

	if d == 0 || nb > d {
		return nil
	}

	cartesTirees := cc.Cartes[d-nb:]
	cc.Cartes = cc.Cartes[:d-nb]
	return cartesTirees
}

// EstPaquetComplet vérifie que la collection contient exactement les 32 cartes du jeu.
func (cc CarteCollection) EstPaquetComplet() bool {
	if len(cc.Cartes) != 32 {
		return false
	}
	vues := make(map[Carte]bool, 32)
	for _, c := range cc.Cartes {
		if !c.Valide() || vues[c] {
			return false
		}
		vues[c] = true
	}
	return true
}

// Hasard fournit des entiers aléatoires dans [0, n).
type Hasard interface {
	Intn(n int) int
}

// HasardCrypto est la source utilisée en production : imprévisible côté joueurs.
type HasardCrypto struct{}

func (HasardCrypto) Intn(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		panic(fmt.Errorf("lecture de l'aléa système impossible : %w", err))
	}
	return int(v.Int64())
}

type Melangeur interface {
	Melanger(CarteCollection) CarteCollection
}

type FischerYatesMelangeur struct {
	Hasard Hasard
}

func (f FischerYatesMelangeur) Melanger(cc CarteCollection) CarteCollection {
	h := f.Hasard
	if h == nil {
		h = HasardCrypto{}
	}
	cartes := append([]Carte(nil), cc.Cartes...)
	for i := len(cartes) - 1; i > 0; i-- {
		j := h.Intn(i + 1)
		cartes[i], cartes[j] = cartes[j], cartes[i]
	}
	return CarteCollection{Cartes: cartes}
}

type Coupeur interface {
	Couper(CarteCollection) CarteCollection
}

type RandomCoupeur struct {
	Hasard Hasard
}

func (rc RandomCoupeur) Couper(in CarteCollection) (out CarteCollection) {
	if len(in.Cartes) < 2 {
		return CarteCollection{Cartes: append([]Carte(nil), in.Cartes...)}
	}
	h := rc.Hasard
	if h == nil {
		h = HasardCrypto{}
	}

	r := h.Intn(len(in.Cartes)-1) + 1
	out.Cartes = make([]Carte, 0, len(in.Cartes))
	out.Cartes = append(out.Cartes, in.Cartes[r:]...)
	out.Cartes = append(out.Cartes, in.Cartes[:r]...)

	return out
}
