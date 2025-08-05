package src

import "fmt"

type Equipe struct {
	J1    *Joueur
	J2    *Joueur
	Score int
}

type Joueur struct {
	Nom  string
	Main CarteCollection
}

type Partie struct {
	Equipe1   Equipe
	Equipe2   Equipe
	Paquet    CarteCollection
	Menes     []Mene
	Melangeur Melangeur
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

type Mene struct {
	Contrat  Contrat
	Preneuse *Equipe

	Plis       map[*Equipe][]Pli
	pliEnCours Pli
}

func (m *Mene) Annonce(e *Equipe, c Contrat) error {
	if m.Contrat.ValeurAnnonce >= c.ValeurAnnonce {
		return fmt.Errorf("Annonce %v doit être supérieure à %v", c.ValeurAnnonce, m.Contrat.ValeurAnnonce)
	}

	m.Contrat = c
	m.Preneuse = e

	return nil
}

func NouvellePartie(j1 Joueur, j2 Joueur, j3 Joueur, j4 Joueur) Partie {
	p := Partie{
		Equipe{
			J1:    &j1,
			J2:    &j2,
			Score: 0,
		},
		Equipe{
			J1:    &j3,
			J2:    &j4,
			Score: 0,
		},
		NouveauPaquet32(),
		[]Mene{},
		&FischerYatesMelangeur{},
	}

	p.Paquet = p.Melangeur.Melanger(p.Paquet)

	return p
}

func (p *Partie) Score() map[*Equipe]int {
	s := make(map[*Equipe]int)

	s[&p.Equipe1] = 0
	s[&p.Equipe2] = 0

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
		return &p.Equipe1
	}
	if p.Equipe2.J1 == j || p.Equipe2.J2 == j {
		return &p.Equipe2
	}
	return nil
}

func (p *Partie) Joueurs() []*Joueur {
	return []*Joueur{p.Equipe1.J1, p.Equipe1.J2, p.Equipe2.J1, p.Equipe2.J2}
}

func (p *Partie) NouvelleDonne() {
	p.Paquet.Couper()
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
}

func (p *Partie) JoueCarte(joueur *Joueur, carte Carte) error {

	c, err := joueur.Main.Pop(carte)

	if err != nil {
		return fmt.Errorf("impossible de jouer la carte : %w", err)
	}

	m := &p.Menes[len(p.Menes)-1]
	m.pliEnCours.Ajouter(joueur, &c)

	if len(m.pliEnCours.Cartes) == 4 {
		gagnant := m.pliEnCours.Remporte(m.Contrat.Couleur)

		if m.Plis == nil {
			m.Plis = make(map[*Equipe][]Pli)
		}
		m.Plis[p.EquipeDe(gagnant)] = append(m.Plis[p.EquipeDe(gagnant)], m.pliEnCours)
	}

	return nil
}
