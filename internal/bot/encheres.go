package bot

import (
	"math"

	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
)

const (
	// apportPartenaire estime les points que le partenaire ramène en moyenne.
	apportPartenaire = 30
	pointsParLevee   = 20
)

// ordreAtoutDecroissant et ordreCouleurDecroissant listent les valeurs de la plus forte à la plus faible.
var (
	ordreAtoutDecroissant   = [...]coinche.Valeur{coinche.Valet, coinche.Neuf, coinche.As, coinche.Dix, coinche.Roi, coinche.Dame, coinche.Huit, coinche.Sept}
	ordreCouleurDecroissant = [...]coinche.Valeur{coinche.As, coinche.Dix, coinche.Roi, coinche.Dame, coinche.Valet, coinche.Neuf, coinche.Huit, coinche.Sept}
)

func meilleureAnnonce(main []coinche.Carte) (coinche.Atout, coinche.ValeurAnnonce) {
	var meilleur coinche.Atout
	var valeur coinche.ValeurAnnonce
	for _, a := range coinche.Atouts {
		if v := estimer(main, a); v > valeur {
			meilleur, valeur = a, v
		}
	}
	return meilleur, valeur
}

// estimer renvoie le contrat que la main permet d'espérer à cet atout, ou 0.
func estimer(main []coinche.Carte, a coinche.Atout) coinche.ValeurAnnonce {
	l, ok := levees(main, a)
	if !ok {
		return 0
	}
	if a.EstCouleur() && toutesSures(main, coinche.Couleur(a)) {
		return coinche.Capot
	}

	estimation := int(math.Floor(l*pointsParLevee+apportPartenaire)) / 10 * 10
	if estimation < int(coinche.A80) {
		return 0
	}
	return coinche.ValeurAnnonce(min(estimation, int(coinche.A160)))
}

func levees(main []coinche.Carte, a coinche.Atout) (float64, bool) {
	switch {
	case a.EstCouleur():
		return leveesCouleur(main, coinche.Couleur(a))
	case a == coinche.SansAtout:
		return leveesSansAtout(main)
	default:
		return leveesToutAtout(main)
	}
}

func leveesCouleur(main []coinche.Carte, atout coinche.Couleur) (float64, bool) {
	atouts := deCouleur(main, atout)
	n := len(atouts)
	valet := possede(atouts, coinche.Valet)
	neuf := possede(atouts, coinche.Neuf)
	if !valet && (!neuf || n < 3) {
		return 0, false
	}

	l := 0.0
	if valet {
		l++
	}
	switch {
	case neuf && (valet || n >= 3):
		l++
	case neuf:
		l += 0.5
	}
	switch {
	case possede(atouts, coinche.As) && (valet || neuf):
		l += 0.75
	case possede(atouts, coinche.As):
		l += 0.5
	}
	if possede(atouts, coinche.Dix) {
		l += 0.25
	}
	if n > 3 {
		l += 0.5 * float64(n-3)
	}

	for _, co := range coinche.Couleurs {
		if co == atout {
			continue
		}
		cote := deCouleur(main, co)
		as := possede(cote, coinche.As)
		if as {
			l += 0.9
		}
		if as && possede(cote, coinche.Dix) {
			l += 0.6
		}
		if len(cote) == 0 && n >= 3 {
			l += 0.3
		}
	}
	return l, true
}

func leveesSansAtout(main []coinche.Carte) (float64, bool) {
	l, as := 0.0, 0
	for _, co := range coinche.Couleurs {
		cote := deCouleur(main, co)
		if !possede(cote, coinche.As) {
			continue
		}
		as++
		l++
		if possede(cote, coinche.Dix) {
			l += 0.8
			if possede(cote, coinche.Roi) {
				l += 0.4
			}
		}
		if len(cote) >= 5 {
			l += 0.5 * float64(len(cote)-4)
		}
	}
	return l, as >= 3
}

func leveesToutAtout(main []coinche.Carte) (float64, bool) {
	l, valets := 0.0, 0
	for _, co := range coinche.Couleurs {
		cote := deCouleur(main, co)
		if !possede(cote, coinche.Valet) {
			continue
		}
		valets++
		l++
		if possede(cote, coinche.Neuf) {
			l += 0.8
			if possede(cote, coinche.As) {
				l += 0.4
			}
		}
	}
	return l, valets >= 3
}

// toutesSures : les atouts maîtres suffisent à faire tomber ceux des adversaires
// et chaque couleur annexe n'est faite que de cartes maîtresses.
func toutesSures(main []coinche.Carte, atout coinche.Couleur) bool {
	atouts := deCouleur(main, atout)
	adverses := len(coinche.Valeurs) - len(atouts)
	if adverses > 0 && sequenceDeTete(atouts, ordreAtoutDecroissant[:]) < adverses {
		return false
	}
	for _, co := range coinche.Couleurs {
		if co == atout {
			continue
		}
		cote := deCouleur(main, co)
		if sequenceDeTete(cote, ordreCouleurDecroissant[:]) != len(cote) {
			return false
		}
	}
	return true
}

func sequenceDeTete(cartes []coinche.Carte, ordre []coinche.Valeur) int {
	n := 0
	for _, v := range ordre {
		if !possede(cartes, v) {
			break
		}
		n++
	}
	return n
}

// soutenir surenchérit dans la couleur du partenaire si la main lui apporte au moins 10 points.
func soutenir(v coinche.Vue, o coinche.OptionsEnchere) coinche.Enchere {
	if aDejaAnnonce(v) {
		return coinche.NouvellePasse()
	}
	contrat := *v.Mene.Contrat
	apport := soutien(v.Main, contrat.Atout)
	valeur := coinche.ValeurAnnonce(min(int(contrat.Valeur)+apport, int(coinche.A160)))
	if apport < 10 || valeur < o.AnnonceMin {
		return coinche.NouvellePasse()
	}
	return coinche.NouvelleAnnonce(contrat.Atout, valeur)
}

func soutien(main []coinche.Carte, a coinche.Atout) int {
	s := 0
	for _, c := range main {
		switch {
		case a == coinche.SansAtout && c.Valeur == coinche.As:
			s += 10
		case a == coinche.ToutAtout && c.Valeur == coinche.Valet:
			s += 10
		case a.EstCouleur() && c.Couleur == coinche.Couleur(a) && (c.Valeur == coinche.Valet || c.Valeur == coinche.Neuf):
			s += 10
		case a.EstCouleur() && c.Couleur != coinche.Couleur(a) && c.Valeur == coinche.As:
			s += 10
		}
	}
	return s
}

func aDejaAnnonce(v coinche.Vue) bool {
	for _, e := range v.Mene.Encheres {
		if e.Siege == v.Siege && e.Type == coinche.Annonce {
			return true
		}
	}
	return false
}

func deCouleur(cartes []coinche.Carte, co coinche.Couleur) []coinche.Carte {
	var res []coinche.Carte
	for _, c := range cartes {
		if c.Couleur == co {
			res = append(res, c)
		}
	}
	return res
}

func possede(cartes []coinche.Carte, v coinche.Valeur) bool {
	for _, c := range cartes {
		if c.Valeur == v {
			return true
		}
	}
	return false
}
