package coinche

// CartesJouables applique les règles de pose : fournir, monter à l'atout, couper,
// surcouper (ou sous-couper faute de mieux), sauf si le partenaire est maître.
func CartesJouables(main []Carte, pli Pli, atout Atout, siege Siege) []Carte {
	if len(main) == 0 {
		return nil
	}

	demandee, demarre := pli.CouleurDemandee()
	if !demarre {
		return copier(main)
	}

	deLaCouleur := filtrer(main, func(c Carte) bool { return c.Couleur == demandee })

	switch atout {
	case SansAtout:
		if len(deLaCouleur) > 0 {
			return deLaCouleur
		}
		return copier(main)

	case ToutAtout:
		if len(deLaCouleur) > 0 {
			reference, _ := pli.plusHauteDeCouleur(demandee, atout)
			return monter(deLaCouleur, reference, atout)
		}
		return copier(main)
	}

	couleurAtout := Couleur(atout)

	if len(deLaCouleur) > 0 {
		if demandee == couleurAtout {
			reference, _ := pli.plusHautAtout(atout)
			return monter(deLaCouleur, reference, atout)
		}
		return deLaCouleur
	}

	atouts := filtrer(main, func(c Carte) bool { return c.Couleur == couleurAtout })
	if len(atouts) == 0 {
		return copier(main)
	}

	if maitre, _ := pli.Maitre(atout); maitre.Siege == siege.Partenaire() {
		return copier(main)
	}

	reference, coupe := pli.plusHautAtout(atout)
	if !coupe {
		return atouts
	}
	return monter(atouts, reference, atout)
}

// PeutJouer vérifie qu'une carte est dans la main et respecte les règles de pose.
func PeutJouer(main []Carte, carte Carte, pli Pli, atout Atout, siege Siege) bool {
	for _, c := range CartesJouables(main, pli, atout, siege) {
		if c == carte {
			return true
		}
	}
	return false
}

// monter garde les cartes qui battent la référence ; s'il n'y en a aucune, toutes restent jouables.
func monter(cartes []Carte, reference Carte, atout Atout) []Carte {
	superieures := filtrer(cartes, func(c Carte) bool { return c.Rang(atout) > reference.Rang(atout) })
	if len(superieures) > 0 {
		return superieures
	}
	return cartes
}

func filtrer(cartes []Carte, garder func(Carte) bool) []Carte {
	var res []Carte
	for _, c := range cartes {
		if garder(c) {
			res = append(res, c)
		}
	}
	return res
}

func copier(cartes []Carte) []Carte {
	return append([]Carte(nil), cartes...)
}
