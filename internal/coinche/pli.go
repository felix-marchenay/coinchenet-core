package coinche

type CarteJouee struct {
	Siege Siege `json:"siege"`
	Carte Carte `json:"carte"`
}

type Pli struct {
	Entame  Siege        `json:"entame"`
	Cartes  []CarteJouee `json:"cartes"`
	Gagnant Siege        `json:"gagnant"`
}

func (p Pli) Complet() bool {
	return len(p.Cartes) == NbSieges
}

func (p Pli) CouleurDemandee() (Couleur, bool) {
	if len(p.Cartes) == 0 {
		return 0, false
	}
	return p.Cartes[0].Carte.Couleur, true
}

// Maitre renvoie la carte qui remporte le pli en l'état.
func (p Pli) Maitre(a Atout) (CarteJouee, bool) {
	if len(p.Cartes) == 0 {
		return CarteJouee{}, false
	}

	demandee := p.Cartes[0].Carte.Couleur
	meilleure := p.Cartes[0]
	for _, cj := range p.Cartes[1:] {
		if cj.Carte.Bat(&meilleure.Carte, a, demandee) {
			meilleure = cj
		}
	}
	return meilleure, true
}

func (p Pli) Remporte(a Atout) Siege {
	m, _ := p.Maitre(a)
	return m.Siege
}

func (p Pli) Score(a Atout) int {
	s := 0
	for _, cj := range p.Cartes {
		s += cj.Carte.Points(a)
	}
	return s
}

func (p Pli) SCartes() (cc CarteCollection) {
	for _, cj := range p.Cartes {
		cc.Cartes = append(cc.Cartes, cj.Carte)
	}
	return
}

// plusHautAtout renvoie le plus fort atout posé dans le pli.
func (p Pli) plusHautAtout(a Atout) (Carte, bool) {
	var meilleure Carte
	trouve := false
	for _, cj := range p.Cartes {
		if !a.EstCouleur() || cj.Carte.Couleur != Couleur(a) {
			continue
		}
		if !trouve || cj.Carte.Rang(a) > meilleure.Rang(a) {
			meilleure, trouve = cj.Carte, true
		}
	}
	return meilleure, trouve
}

// plusHauteDeCouleur renvoie la plus forte carte posée dans une couleur donnée.
func (p Pli) plusHauteDeCouleur(c Couleur, a Atout) (Carte, bool) {
	var meilleure Carte
	trouve := false
	for _, cj := range p.Cartes {
		if cj.Carte.Couleur != c {
			continue
		}
		if !trouve || cj.Carte.Rang(a) > meilleure.Rang(a) {
			meilleure, trouve = cj.Carte, true
		}
	}
	return meilleure, trouve
}
