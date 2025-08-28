package coinche

import (
	"fmt"
)

type Equipe struct {
	J1    *Joueur
	J2    *Joueur
	Score int
}

type Joueur struct {
	Nom  string
	Main CarteCollection
}

func (j *Joueur) PeutJouer(carte Carte, p Pli, atout Atout) bool {
	trouvee := false
	aAtout := false
	aCouleurDemandee := false
	var plusHauteCarteAtoutDuPli *Carte = nil
	var plusHauteCarteAtoutDeLaMain *Carte = nil

	_, pliDemarre := p.Cartes[p.PremierJoueur]

	// si plusHauteValeurAtout reste à -1 c'est qu'aucun atout n'a été joué dans ce pli
	for _, cj := range p.Cartes {
		if cj.Couleur == Couleur(atout) || atout == ToutAtout {
			if plusHauteCarteAtoutDuPli == nil {
				plusHauteCarteAtoutDuPli = &cj
				continue
			}

			if cj.Bat(plusHauteCarteAtoutDuPli, atout, Couleur(atout)) {
				plusHauteCarteAtoutDuPli = &cj
			}
		}
	}

	for _, c := range j.Main.Cartes {

		if c == carte {
			trouvee = true
		}
		if c.Couleur == Couleur(atout) || atout == ToutAtout {
			if plusHauteCarteAtoutDeLaMain == nil || c.Bat(plusHauteCarteAtoutDeLaMain, atout, Couleur(atout)) {
				plusHauteCarteAtoutDeLaMain = &c
			}
		}
		if !pliDemarre {
			continue
		}
		if c.Couleur == p.Cartes[p.PremierJoueur].Couleur {
			aCouleurDemandee = true
		}
	}

	aAtout = plusHauteCarteAtoutDeLaMain != nil

	if !trouvee {
		return false
	}

	if !pliDemarre {
		return true
	}

	couleurDemandee := p.Cartes[p.PremierJoueur].Couleur

	// Si c'est la couleur demandée on peut la jouer
	if carte.Couleur == couleurDemandee {

		// Si on a un atout supérieur et que c'est de l'atout demandé, il faut monter si on peut
		if (couleurDemandee == Couleur(atout) || atout == ToutAtout) && aAtout && plusHauteCarteAtoutDuPli.Bat(&carte, atout, Couleur(atout)) && plusHauteCarteAtoutDeLaMain.Bat(plusHauteCarteAtoutDuPli, atout, couleurDemandee) {
			return false
		}

		return true
	}

	// on a la couleur demandée mais cette carte n'en est pas
	if aCouleurDemandee {
		return false
	}

	if aAtout && carte.Couleur != Couleur(atout) {
		return false
	}

	return true
}

type Partie struct {
	Equipe1   *Equipe
	Equipe2   *Equipe
	Paquet    CarteCollection
	Menes     []Mene
	Melangeur Melangeur
	Coupeur   Coupeur
}

type Contrat struct {
	Couleur       Atout
	ValeurAnnonce ValeurAnnonce
}

type Atout int

const (
	AtoutCoeur Atout = iota
	AtoutCarreau
	AtoutTrefle
	AtoutPique
	SansAtout
	ToutAtout
)

type ValeurAnnonce int

const (
	A80      ValeurAnnonce = 80
	A90      ValeurAnnonce = 90
	A100     ValeurAnnonce = 100
	A110     ValeurAnnonce = 110
	A120     ValeurAnnonce = 120
	A130     ValeurAnnonce = 130
	A140     ValeurAnnonce = 140
	A150     ValeurAnnonce = 150
	A160     ValeurAnnonce = 160
	Capot    ValeurAnnonce = 250
	Generale ValeurAnnonce = 500
)

func NouvellePartie(j1 Joueur, j2 Joueur, j3 Joueur, j4 Joueur) Partie {
	p := Partie{
		&Equipe{
			J1:    &j1,
			J2:    &j2,
			Score: 0,
		},
		&Equipe{
			J1:    &j3,
			J2:    &j4,
			Score: 0,
		},
		NouveauPaquet32(),
		[]Mene{},
		&FischerYatesMelangeur{},
		RandomCoupeur{},
	}

	p.Paquet = p.Melangeur.Melanger(p.Paquet)

	return p
}

func (p *Partie) Score() map[*Equipe]int {
	s := make(map[*Equipe]int)

	s[p.Equipe1] = 0
	s[p.Equipe2] = 0

	for _, mene := range p.Menes {
		for equipe, plis := range mene.Plis {
			for _, pli := range plis {
				s[equipe] += pli.Score(mene.Contrat.Couleur)
			}
		}
	}

	return s
}

func (p *Partie) EquipeDe(j *Joueur) *Equipe {
	if p.Equipe1.J1 == j || p.Equipe1.J2 == j {
		return p.Equipe1
	}
	if p.Equipe2.J1 == j || p.Equipe2.J2 == j {
		return p.Equipe2
	}

	panic("équipe introuvable")
}

func (p *Partie) Joueurs() []*Joueur {
	return []*Joueur{p.Equipe1.J1, p.Equipe1.J2, p.Equipe2.J1, p.Equipe2.J2}
}

func (p *Partie) NouvelleMene() error {

	if len(p.Menes) > 0 {
		m := p.Menes[len(p.Menes)-1]

		if !m.Terminée() {
			return fmt.Errorf("impossible de lancer une nouvelle mène si la dernière n'est pas terminée")
		}

		cc := CarteCollection{}

		for _, c := range m.Plis[p.Equipe1] {
			cc.Cartes = append(cc.Cartes, c.SCartes().Cartes...)
		}
		for _, c := range m.Plis[p.Equipe2] {
			cc.Cartes = append(cc.Cartes, c.SCartes().Cartes...)
		}

		p.Paquet = cc
	}

	p.Menes = append(p.Menes, Mene{})

	p.Paquet = p.Coupeur.Couper(p.Paquet)

	js := p.Joueurs()

	js[0].Main.Cartes = append(js[0].Main.Cartes, p.Paquet.Tirer(3)...)
	js[1].Main.Cartes = append(js[1].Main.Cartes, p.Paquet.Tirer(3)...)
	js[2].Main.Cartes = append(js[2].Main.Cartes, p.Paquet.Tirer(3)...)
	js[3].Main.Cartes = append(js[3].Main.Cartes, p.Paquet.Tirer(3)...)

	js[0].Main.Cartes = append(js[0].Main.Cartes, p.Paquet.Tirer(2)...)
	js[1].Main.Cartes = append(js[1].Main.Cartes, p.Paquet.Tirer(2)...)
	js[2].Main.Cartes = append(js[2].Main.Cartes, p.Paquet.Tirer(2)...)
	js[3].Main.Cartes = append(js[3].Main.Cartes, p.Paquet.Tirer(2)...)

	js[0].Main.Cartes = append(js[0].Main.Cartes, p.Paquet.Tirer(3)...)
	js[1].Main.Cartes = append(js[1].Main.Cartes, p.Paquet.Tirer(3)...)
	js[2].Main.Cartes = append(js[2].Main.Cartes, p.Paquet.Tirer(3)...)
	js[3].Main.Cartes = append(js[3].Main.Cartes, p.Paquet.Tirer(3)...)

	return nil
}

func (p *Partie) JoueCarte(joueur *Joueur, carte Carte) error {

	m := &p.Menes[len(p.Menes)-1]

	if !joueur.PeutJouer(carte, m.pliEnCours, m.Contrat.Couleur) {
		return fmt.Errorf("carte interdite")
	}

	c, err := joueur.Main.Pop(carte)

	if err != nil {
		return fmt.Errorf("impossible de jouer la carte : %w", err)
	}

	if m.Terminée() {
		return fmt.Errorf("mène terminée")
	}

	m.pliEnCours.Ajouter(joueur, &c)

	if len(m.pliEnCours.Cartes) == 4 {
		gagnant := m.pliEnCours.Remporte(m.Contrat.Couleur)

		if m.Plis == nil {
			m.Plis = make(map[*Equipe][]Pli)
		}
		m.Plis[p.EquipeDe(gagnant)] = append(m.Plis[p.EquipeDe(gagnant)], m.pliEnCours)
		m.pliEnCours = Pli{
			Cartes: make(map[*Joueur]Carte),
		}
	}

	return nil
}

func (p *Partie) Annonce(j *Joueur, c Contrat) error {
	if len(p.Menes) == 0 {
		return fmt.Errorf("impossible de miser sans mène")
	}

	m := &p.Menes[len(p.Menes)-1]

	if m.Démarrée() {
		return fmt.Errorf("impossible de miser sur une mène démarée")
	}

	m.Annonce(p.EquipeDe(j), c)

	return nil
}
