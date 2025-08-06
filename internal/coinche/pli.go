package coinche

type Pli struct {
	Cartes        map[*Joueur]*Carte
	PremierJoueur *Joueur
}

func (p Pli) SCartes() (cc CarteCollection) {

	for _, c := range p.Cartes {
		cc.Cartes = append(cc.Cartes, *c)
	}

	return
}

func (p *Pli) Ajouter(j *Joueur, c *Carte) {
	if p.PremierJoueur == nil {
		p.PremierJoueur = j
	}

	if _, existe := p.Cartes[j]; existe {
		return
	}

	if p.Cartes == nil {
		p.Cartes = make(map[*Joueur]*Carte)
	}

	p.Cartes[j] = c
}

func (p Pli) Remporte(a Atout) (g *Joueur) {
	if len(p.Cartes) == 0 {
		return nil
	}

	var b *Carte

	couleurDemandee := p.Cartes[p.PremierJoueur].Couleur

	for j, c := range p.Cartes {
		if b == nil || c.Bat(b, a, couleurDemandee) {
			b = c
			g = j
		}
	}

	return g
}

func (p Pli) Score(a Atout) int {
	if len(p.Cartes) == 0 {
		return 0
	}

	s := 0
	for _, c := range p.Cartes {
		s += c.Points(a)
	}

	return s
}
