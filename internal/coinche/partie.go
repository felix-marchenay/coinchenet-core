package coinche

import "fmt"

// Partie est la machine à états d'une partie. Elle ne tire aucun hasard elle-même :
// le paquet de chaque donne lui est fourni, ce qui permet de la reconstruire à l'identique.
type Partie struct {
	Regles         Regles
	PremierDonneur Siege
	Scores         [2]int
	Menes          []*Mene
	Terminee       bool
	Gagnante       Equipe
	Forfait        bool
}

func NouvellePartie(r Regles, premierDonneur Siege) (*Partie, error) {
	if err := r.Valider(); err != nil {
		return nil, err
	}
	if !premierDonneur.Valide() {
		return nil, fmt.Errorf("donneur invalide %d", premierDonneur)
	}
	return &Partie{Regles: r, PremierDonneur: premierDonneur}, nil
}

// MeneCourante renvoie la dernière mène distribuée, terminée ou non.
func (p *Partie) MeneCourante() *Mene {
	if len(p.Menes) == 0 {
		return nil
	}
	return p.Menes[len(p.Menes)-1]
}

// MeneEnCours renvoie la mène en cours de jeu, ou nil entre deux donnes.
func (p *Partie) MeneEnCours() *Mene {
	m := p.MeneCourante()
	if m == nil || m.Terminee() {
		return nil
	}
	return m
}

// AttendDonne indique qu'il faut distribuer une nouvelle mène.
func (p *Partie) AttendDonne() bool {
	return !p.Terminee && p.MeneEnCours() == nil
}

func (p *Partie) ProchainDonneur() Siege {
	if m := p.MeneCourante(); m != nil {
		return m.Donneur.Suivant()
	}
	return p.PremierDonneur
}

// PaquetSuivant prépare les cartes de la prochaine donne, coupées et prêtes à distribuer.
func (p *Partie) PaquetSuivant(m Melangeur, c Coupeur) CarteCollection {
	precedente := p.MeneCourante()
	if precedente == nil || p.Regles.Distribution == DistributionAleatoire {
		return c.Couper(m.Melanger(NouveauPaquet32()))
	}
	return c.Couper(precedente.Ramasser())
}

func (p *Partie) Distribuer(paquet CarteCollection) (*Mene, error) {
	if p.Terminee {
		return nil, ErrPartieTerminee
	}
	if p.MeneEnCours() != nil {
		return nil, ErrDonneEnCours
	}
	if !paquet.EstPaquetComplet() {
		return nil, ErrPaquetInvalide
	}

	m := nouvelleMene(len(p.Menes)+1, p.ProchainDonneur(), paquet)
	p.Menes = append(p.Menes, m)
	return m, nil
}

func (p *Partie) Encherir(s Siege, e Enchere) (IssueEnchere, error) {
	m, err := p.meneJouable()
	if err != nil {
		return IssueEnchere{}, err
	}
	return m.Encherir(s, e)
}

func (p *Partie) Jouer(s Siege, c Carte) (IssueCarte, error) {
	m, err := p.meneJouable()
	if err != nil {
		return IssueCarte{}, err
	}

	issue, err := m.Jouer(s, c)
	if err != nil || !m.Terminee() {
		return issue, err
	}

	r := Decompter(p.Regles.Decompte, m)
	m.Resultat = &r
	issue.Mene = &r
	p.Scores[EquipeA] += r.Points[EquipeA]
	p.Scores[EquipeB] += r.Points[EquipeB]
	issue.PartieTerminee = p.verifierFin()
	return issue, nil
}

// Abandonner termine la partie par forfait de l'équipe donnée.
func (p *Partie) Abandonner(e Equipe) {
	if p.Terminee {
		return
	}
	p.Terminee = true
	p.Forfait = true
	p.Gagnante = e.Adverse()
}

// Tour indique qui doit agir et dans quelle phase.
func (p *Partie) Tour() (Siege, Phase, bool) {
	if p.Terminee {
		return 0, PhaseTerminee, false
	}
	m := p.MeneEnCours()
	if m == nil {
		return 0, PhaseTerminee, false
	}
	return m.Tour, m.Phase, true
}

func (p *Partie) CartesJouables(s Siege) []Carte {
	m := p.MeneEnCours()
	if p.Terminee || m == nil {
		return nil
	}
	return m.CartesJouables(s)
}

func (p *Partie) OptionsEnchere(s Siege) (OptionsEnchere, bool) {
	m := p.MeneEnCours()
	if p.Terminee || m == nil {
		return OptionsEnchere{}, false
	}
	return m.OptionsEnchere(s)
}

// Historique renvoie les résultats des mènes terminées.
func (p *Partie) Historique() []ResultatMene {
	h := make([]ResultatMene, 0, len(p.Menes))
	for _, m := range p.Menes {
		if m.Resultat != nil {
			h = append(h, *m.Resultat)
		}
	}
	return h
}

func (p *Partie) meneJouable() (*Mene, error) {
	if p.Terminee {
		return nil, ErrPartieTerminee
	}
	m := p.MeneEnCours()
	if m == nil {
		return nil, ErrAucuneMene
	}
	return m, nil
}

// verifierFin : la partie s'arrête à la fin de la mène où une équipe atteint l'objectif,
// l'équipe en tête l'emporte et une égalité impose une mène de plus.
func (p *Partie) verifierFin() bool {
	a, b := p.Scores[EquipeA], p.Scores[EquipeB]
	if max(a, b) < p.Regles.Objectif || a == b {
		return false
	}
	p.Terminee = true
	p.Gagnante = EquipeA
	if b > a {
		p.Gagnante = EquipeB
	}
	return true
}
