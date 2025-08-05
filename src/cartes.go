package src

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

type Carte struct {
	Couleur Couleur
	Valeur  Valeur
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

func (p *CarteCollection) Couper() {
	var c CarteCollection
	r := rand.Intn(len(p.Cartes)-1) + 1
	c.Cartes = append(c.Cartes, p.Cartes[r:]...)
	c.Cartes = append(c.Cartes, p.Cartes[:r]...)

	p.Cartes = c.Cartes
}

func (p *CarteCollection) Melanger() {
	for i := len(p.Cartes) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		p.Cartes[i], p.Cartes[j] = p.Cartes[j], p.Cartes[i]
	}
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
