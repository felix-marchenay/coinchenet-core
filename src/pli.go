package src

type Pli struct {
	Cartes        map[*Joueur]*Carte
	PremierJoueur *Joueur
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
