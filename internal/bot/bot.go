// Package bot fournit un joueur artificiel simple, à base de règles, utilisé pour
// remplacer un joueur absent ou compléter une table.
package bot

import "github.com/felix-marchenay/coinchenet-core/internal/coinche"

// Bot est sans état : ses décisions ne dépendent que de la vue qui lui est donnée.
type Bot struct{}

func (Bot) Encherir(v coinche.Vue) coinche.Enchere {
	vm := v.Mene
	if vm == nil || vm.OptionsEnchere == nil {
		return coinche.NouvellePasse()
	}
	o := *vm.OptionsEnchere
	if o.AnnonceMin == 0 {
		return coinche.NouvellePasse()
	}

	if vm.Contrat != nil && vm.Preneur != nil && *vm.Preneur == v.Siege.Partenaire() {
		return soutenir(v, o)
	}

	atout, valeur := meilleureAnnonce(v.Main)
	if valeur == 0 || valeur < o.AnnonceMin {
		return coinche.NouvellePasse()
	}
	if vm.Contrat != nil {
		valeur = o.AnnonceMin
	}
	return coinche.NouvelleAnnonce(atout, valeur)
}

func (Bot) Jouer(v coinche.Vue) coinche.Carte {
	vm := v.Mene
	if vm == nil || len(vm.CartesJouables) == 0 || vm.Contrat == nil {
		return coinche.Carte{}
	}
	if len(vm.CartesJouables) == 1 {
		return vm.CartesJouables[0]
	}

	j := nouveauJeu(v)
	if len(vm.PliEnCours.Cartes) == 0 {
		return j.entamer()
	}
	return j.suivre()
}
