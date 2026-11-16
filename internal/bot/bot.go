// Package bot fournit un joueur artificiel à base de règles, utilisé pour remplacer un joueur
// absent ou compléter une table. Son profil (profils.go) règle son goût du risque.
package bot

import "github.com/felix-marchenay/coinchenet-core/internal/coinche"

// Bot est sans état : ses décisions ne dépendent que de son profil et de la vue qui lui est donnée.
// Le profil zéro joue comme Defaut.
type Bot struct {
	Profil Profil
}

func (b Bot) profil() Profil {
	if b.Profil.ID == "" {
		return Defaut
	}
	return b.Profil
}

func (b Bot) Encherir(v coinche.Vue) coinche.Enchere {
	p := b.profil()
	vm := v.Mene
	if vm == nil || vm.OptionsEnchere == nil {
		return coinche.NouvellePasse()
	}
	o := *vm.OptionsEnchere
	partenairePreneur := vm.Contrat != nil && vm.Preneur != nil && *vm.Preneur == v.Siege.Partenaire()
	adversePreneur := vm.Contrat != nil && vm.Preneur != nil && vm.Preneur.Equipe() != v.Siege.Equipe()

	if o.Surcoinche && p.surcoincher(v) {
		return coinche.Enchere{Type: coinche.Surcoinche}
	}
	if o.AnnonceMin != 0 {
		if partenairePreneur {
			if e, ok := p.soutenir(v, o); ok {
				return e
			}
			return coinche.NouvellePasse()
		}
		atout, valeur := p.meilleureAnnonce(v.Main)
		if valeur != 0 && valeur >= o.AnnonceMin {
			if vm.Contrat != nil && valeur < coinche.Capot {
				valeur = o.AnnonceMin
			}
			return coinche.NouvelleAnnonce(atout, valeur)
		}
	}
	if o.Coinche && adversePreneur && p.coincher(v) {
		return coinche.Enchere{Type: coinche.Coinche}
	}
	return coinche.NouvellePasse()
}

func (b Bot) Jouer(v coinche.Vue) coinche.Carte {
	vm := v.Mene
	if vm == nil || len(vm.CartesJouables) == 0 || vm.Contrat == nil {
		return coinche.Carte{}
	}
	if len(vm.CartesJouables) == 1 {
		return vm.CartesJouables[0]
	}

	j := nouveauJeu(v, b.profil())
	if len(vm.PliEnCours.Cartes) == 0 {
		return j.entamer()
	}
	return j.suivre()
}
