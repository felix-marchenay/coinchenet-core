package coinche

import (
	"fmt"
	"math/rand"
)

type Couleur int

const (
	Coeur Couleur = iota
	Carreau
	Trefle
	Pique
)

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
		case Sept, Huit:
			return 0
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
		case Neuf, Sept, Huit:
			return 0
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
		case Sept, Huit:
			return 0
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
	case Neuf, Sept, Huit:
		return 0
	default:
		return 0
	}
}

type Carte struct {
	Couleur Couleur
	Valeur  Valeur
}

type Melangeur interface {
	Melanger(CarteCollection) CarteCollection
}

type FischerYatesMelangeur struct{}

func (f *FischerYatesMelangeur) Melanger(cc CarteCollection) CarteCollection {
	for i := len(cc.Cartes) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		cc.Cartes[i], cc.Cartes[j] = cc.Cartes[j], cc.Cartes[i]
	}
	return cc
}

type Coupeur interface {
	Couper(CarteCollection) CarteCollection
}

type RandomCoupeur struct{}

func (RandomCoupeur) Couper(in CarteCollection) (out CarteCollection) {

	r := rand.Intn(len(in.Cartes)-1) + 1
	out.Cartes = append(out.Cartes, in.Cartes[r:]...)
	out.Cartes = append(out.Cartes, in.Cartes[:r]...)

	in.Cartes = out.Cartes

	return
}

func (c *Carte) Bat(c2 *Carte, atout Atout, couleurDemandee Couleur) bool {
	ordreSansAtout := map[Valeur]int{Sept: 1, Huit: 2, Neuf: 3, Dix: 4, Valet: 5, Dame: 6, Roi: 7, As: 8}
	ordreAtout := map[Valeur]int{Sept: 100, Huit: 200, Dame: 300, Roi: 400, Dix: 500, As: 600, Neuf: 700, Valet: 800}

	var p1, p2 int

	if atout == ToutAtout {
		if c.Couleur == couleurDemandee {
			p1 = ordreAtout[c.Valeur] * 10
		} else {
			p1 = ordreAtout[c.Valeur]
		}

		if c2.Couleur == couleurDemandee {
			p2 = ordreAtout[c2.Valeur] * 10
		} else {
			p2 = ordreAtout[c2.Valeur]
		}

		return p1 > p2
	}

	if atout == SansAtout {
		if c.Couleur == couleurDemandee {
			p1 = ordreSansAtout[c.Valeur] * 10
		} else {
			p1 = ordreSansAtout[c.Valeur]
		}

		if c2.Couleur == couleurDemandee {
			p2 = ordreSansAtout[c2.Valeur] * 10
		} else {
			p2 = ordreSansAtout[c2.Valeur]
		}

		return p1 > p2
	}

	if atout == Atout(c.Couleur) {
		p1 = ordreAtout[c.Valeur]
	} else if c.Couleur == couleurDemandee {
		p1 = ordreSansAtout[c.Valeur] * 10
	} else {
		p1 = ordreSansAtout[c.Valeur]
	}

	if atout == Atout(c2.Couleur) {
		p2 = ordreAtout[c2.Valeur]
	} else if c2.Couleur == couleurDemandee {
		p2 = ordreSansAtout[c2.Valeur] * 10
	} else {
		p2 = ordreSansAtout[c2.Valeur]
	}

	return p1 > p2
}

type CarteCollection struct {
	Cartes []Carte
}

func NouveauPaquet32() CarteCollection {
	var cartes []Carte
	for c := Coeur; c <= Pique; c++ {
		for v := Sept; v <= As; v++ {
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

func (p *CarteCollection) Tirer(nb int) []Carte {
	d := len(p.Cartes)

	if d == 0 || nb > d {
		return nil
	}

	cartesTirees := p.Cartes[d-nb:]
	p.Cartes = p.Cartes[:d-nb]
	return cartesTirees
}
