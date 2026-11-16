package coinche

// Vue est ce qu'un joueur a le droit de connaître de la partie : sa main et les informations publiques.
type Vue struct {
	Siege      Siege          `json:"siege"`
	Regles     Regles         `json:"regles"`
	Scores     [2]int         `json:"scores"`
	Historique []ResultatMene `json:"historique"`
	Mene       *VueMene       `json:"mene,omitempty"`
	Main       []Carte        `json:"main"`
	Terminee   bool           `json:"terminee"`
	Gagnante   *Equipe        `json:"gagnante,omitempty"`
	Forfait    bool           `json:"forfait"`
}

type VueMene struct {
	Numero         int             `json:"numero"`
	Donneur        Siege           `json:"donneur"`
	Phase          Phase           `json:"phase"`
	Tour           Siege           `json:"tour"`
	Encheres       []Enchere       `json:"encheres"`
	Contrat        *Contrat        `json:"contrat,omitempty"`
	Preneur        *Siege          `json:"preneur,omitempty"`
	Multiplicateur int             `json:"multiplicateur"`
	PliEnCours     Pli             `json:"pli_en_cours"`
	Plis           []Pli           `json:"plis"`
	NbCartes       [NbSieges]int   `json:"nb_cartes"`
	Belote         *Siege          `json:"belote,omitempty"`
	BeloteJouees   int             `json:"belote_jouees"`
	Resultat       *ResultatMene   `json:"resultat,omitempty"`
	CartesJouables []Carte         `json:"cartes_jouables,omitempty"`
	OptionsEnchere *OptionsEnchere `json:"options_enchere,omitempty"`
}

func (p *Partie) Vue(s Siege) Vue {
	v := Vue{
		Siege:      s,
		Regles:     p.Regles,
		Scores:     p.Scores,
		Historique: p.Historique(),
		Main:       []Carte{},
		Terminee:   p.Terminee,
		Forfait:    p.Forfait,
	}
	if p.Terminee {
		g := p.Gagnante
		v.Gagnante = &g
	}

	m := p.MeneCourante()
	if m == nil {
		return v
	}

	v.Main = append(v.Main, m.Mains[s].Cartes...)
	vm := &VueMene{
		Numero:         m.Numero,
		Donneur:        m.Donneur,
		Phase:          m.Phase,
		Tour:           m.Tour,
		Encheres:       append([]Enchere{}, m.Encheres...),
		Multiplicateur: m.Multiplicateur,
		PliEnCours:     copierPli(m.PliEnCours),
		Plis:           make([]Pli, 0, len(m.Plis)),
		BeloteJouees:   m.BeloteJouees,
		Resultat:       m.Resultat,
	}
	for _, pli := range m.Plis {
		vm.Plis = append(vm.Plis, copierPli(pli))
	}
	for _, siege := range Sieges {
		vm.NbCartes[siege] = len(m.Mains[siege].Cartes)
	}
	if m.Contrat != nil {
		c := *m.Contrat
		vm.Contrat = &c
		preneur := m.Preneur
		vm.Preneur = &preneur
	}
	// Le détenteur de la belote n'est révélé qu'une fois annoncée, sauf à lui-même.
	if m.Belote != nil && (m.BeloteJouees > 0 || *m.Belote == s) {
		b := *m.Belote
		vm.Belote = &b
	}
	if !p.Terminee {
		vm.CartesJouables = m.CartesJouables(s)
		if o, ok := m.OptionsEnchere(s); ok {
			vm.OptionsEnchere = &o
		}
	}
	v.Mene = vm
	return v
}

// CartesJouees renvoie toutes les cartes déjà posées dans la mène, pli en cours compris.
func (vm *VueMene) CartesJouees() []Carte {
	var cartes []Carte
	for _, p := range vm.Plis {
		cartes = append(cartes, p.SCartes().Cartes...)
	}
	return append(cartes, vm.PliEnCours.SCartes().Cartes...)
}

func copierPli(p Pli) Pli {
	p.Cartes = append([]CarteJouee{}, p.Cartes...)
	return p
}
