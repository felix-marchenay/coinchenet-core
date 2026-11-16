package bot

import "github.com/felix-marchenay/coinchenet-core/internal/coinche"

type jeu struct {
	profil    Profil
	siege     coinche.Siege
	atout     coinche.Atout
	preneur   bool
	pli       coinche.Pli
	jouables  []coinche.Carte
	inconnues []coinche.Carte
}

func nouveauJeu(v coinche.Vue, p Profil) jeu {
	vm := v.Mene
	connues := make(map[coinche.Carte]bool, 32)
	for _, c := range vm.CartesJouees() {
		connues[c] = true
	}
	for _, c := range v.Main {
		connues[c] = true
	}
	var inconnues []coinche.Carte
	for _, c := range coinche.NouveauPaquet32().Cartes {
		if !connues[c] {
			inconnues = append(inconnues, c)
		}
	}

	return jeu{
		profil:    p,
		siege:     v.Siege,
		atout:     vm.Contrat.Atout,
		preneur:   vm.Preneur != nil && vm.Preneur.Equipe() == v.Siege.Equipe(),
		pli:       vm.PliEnCours,
		jouables:  vm.CartesJouables,
		inconnues: inconnues,
	}
}

func (j jeu) entamer() coinche.Carte {
	if j.atout.EstCouleur() && j.preneur && j.resteAtoutsAdverses() {
		if plusFort, ok := j.plusForte(j.atouts(j.jouables)); ok && j.maitresse(plusFort) {
			return plusFort
		}
	}

	var maitresses []coinche.Carte
	for _, c := range j.jouables {
		if !j.estAtoutCouleur(c) && j.maitresse(c) {
			maitresses = append(maitresses, c)
		}
	}
	if len(maitresses) > 0 {
		return meilleure(maitresses, func(a, b coinche.Carte) bool {
			if a.Points(j.atout) != b.Points(j.atout) {
				return a.Points(j.atout) > b.Points(j.atout)
			}
			return a.Rang(j.atout) > b.Rang(j.atout)
		})
	}

	return j.moinsChere(j.jouables)
}

func (j jeu) suivre() coinche.Carte {
	maitre, _ := j.pli.Maitre(j.atout)
	demandee, _ := j.pli.CouleurDemandee()
	dernier := len(j.pli.Cartes) == coinche.NbSieges-1

	if maitre.Siege == j.siege.Partenaire() {
		if dernier || j.imbattable(maitre.Carte, demandee) || j.profil.ChargeRisquee && j.maitresse(maitre.Carte) {
			return j.charger()
		}
		return j.moinsChere(j.jouables)
	}

	var gagnantes []coinche.Carte
	for _, c := range j.jouables {
		if c.Bat(&maitre.Carte, j.atout, demandee) {
			gagnantes = append(gagnantes, c)
		}
	}
	if len(gagnantes) == 0 {
		return j.moinsChere(j.jouables)
	}

	plusPetite := meilleure(gagnantes, func(a, b coinche.Carte) bool { return a.Rang(j.atout) < b.Rang(j.atout) })
	switch {
	case dernier:
		return plusPetite
	case !j.estAtoutCouleur(gagnantes[0]):
		// On fournit : la plus forte carte a le plus de chances de tenir.
		return meilleure(gagnantes, func(a, b coinche.Carte) bool { return a.Rang(j.atout) > b.Rang(j.atout) })
	case j.atout.EstCouleur() && demandee == coinche.Couleur(j.atout):
		var maitresses []coinche.Carte
		for _, c := range gagnantes {
			if j.maitresse(c) {
				maitresses = append(maitresses, c)
			}
		}
		if len(maitresses) > 0 {
			return meilleure(maitresses, func(a, b coinche.Carte) bool { return a.Rang(j.atout) < b.Rang(j.atout) })
		}
		return plusPetite
	default:
		// On coupe avec le plus petit atout suffisant.
		return plusPetite
	}
}

// charger donne un maximum de points au partenaire maître sans sacrifier d'atout.
func (j jeu) charger() coinche.Carte {
	horsAtout := j.horsAtout(j.jouables)
	if len(horsAtout) == 0 {
		return j.moinsChere(j.jouables)
	}
	return meilleure(horsAtout, func(a, b coinche.Carte) bool {
		if a.Points(j.atout) != b.Points(j.atout) {
			return a.Points(j.atout) > b.Points(j.atout)
		}
		return a.Rang(j.atout) < b.Rang(j.atout)
	})
}

// moinsChere choisit la carte qui coûte le moins : hors atout d'abord, puis la moins riche en points.
func (j jeu) moinsChere(cartes []coinche.Carte) coinche.Carte {
	return meilleure(cartes, func(a, b coinche.Carte) bool {
		aa, ab := j.estAtoutCouleur(a), j.estAtoutCouleur(b)
		if aa != ab {
			return !aa
		}
		if a.Points(j.atout) != b.Points(j.atout) {
			return a.Points(j.atout) < b.Points(j.atout)
		}
		return a.Rang(j.atout) < b.Rang(j.atout)
	})
}

// maitresse : aucune carte encore en jeu chez les autres n'est plus forte dans la couleur.
func (j jeu) maitresse(c coinche.Carte) bool {
	for _, x := range j.inconnues {
		if x.Couleur == c.Couleur && x.Rang(j.atout) > c.Rang(j.atout) {
			return false
		}
	}
	return true
}

// imbattable : aucune carte encore en jeu ne peut prendre le pli, coupe comprise.
func (j jeu) imbattable(c coinche.Carte, demandee coinche.Couleur) bool {
	for _, x := range j.inconnues {
		if x.Bat(&c, j.atout, demandee) {
			return false
		}
	}
	return true
}

func (j jeu) resteAtoutsAdverses() bool {
	for _, x := range j.inconnues {
		if j.estAtoutCouleur(x) {
			return true
		}
	}
	return false
}

func (j jeu) estAtoutCouleur(c coinche.Carte) bool {
	return j.atout.EstCouleur() && c.Couleur == coinche.Couleur(j.atout)
}

func (j jeu) atouts(cartes []coinche.Carte) []coinche.Carte {
	var res []coinche.Carte
	for _, c := range cartes {
		if j.estAtoutCouleur(c) {
			res = append(res, c)
		}
	}
	return res
}

func (j jeu) horsAtout(cartes []coinche.Carte) []coinche.Carte {
	var res []coinche.Carte
	for _, c := range cartes {
		if !j.estAtoutCouleur(c) {
			res = append(res, c)
		}
	}
	return res
}

func (j jeu) plusForte(cartes []coinche.Carte) (coinche.Carte, bool) {
	if len(cartes) == 0 {
		return coinche.Carte{}, false
	}
	return meilleure(cartes, func(a, b coinche.Carte) bool { return a.Rang(j.atout) > b.Rang(j.atout) }), true
}

// meilleure renvoie la première carte qu'aucune autre ne précède selon avant.
func meilleure(cartes []coinche.Carte, avant func(a, b coinche.Carte) bool) coinche.Carte {
	choix := cartes[0]
	for _, c := range cartes[1:] {
		if avant(c, choix) {
			choix = c
		}
	}
	return choix
}
