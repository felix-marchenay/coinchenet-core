package coinche

import (
	"errors"
	"fmt"
)

var (
	ErrPartieTerminee  = errors.New("partie terminée")
	ErrPhase           = errors.New("action impossible à ce moment de la mène")
	ErrPasSonTour      = errors.New("ce n'est pas votre tour")
	ErrCarteAbsente    = errors.New("carte absente de la main")
	ErrCarteInterdite  = errors.New("carte interdite")
	ErrEnchereInvalide = errors.New("enchère invalide")
	ErrPaquetInvalide  = errors.New("paquet invalide")
	ErrDonneEnCours    = errors.New("une mène est déjà en cours")
	ErrAucuneMene      = errors.New("aucune mène en cours")
)

type TypeEnchere int

const (
	Passe TypeEnchere = iota
	Annonce
	Coinche
	Surcoinche
)

var nomsTypesEnchere = [...]string{Passe: "passe", Annonce: "annonce", Coinche: "coinche", Surcoinche: "surcoinche"}

func (t TypeEnchere) Valide() bool {
	return t >= Passe && t <= Surcoinche
}

func (t TypeEnchere) String() string {
	if !t.Valide() {
		return fmt.Sprintf("enchere(%d)", int(t))
	}
	return nomsTypesEnchere[t]
}

func (t TypeEnchere) MarshalText() ([]byte, error) {
	if !t.Valide() {
		return nil, fmt.Errorf("type d'enchère invalide %d", int(t))
	}
	return []byte(t.String()), nil
}

func (t *TypeEnchere) UnmarshalText(b []byte) error {
	for v, nom := range nomsTypesEnchere {
		if nom == string(b) {
			*t = TypeEnchere(v)
			return nil
		}
	}
	return fmt.Errorf("type d'enchère inconnu %q", string(b))
}

type Enchere struct {
	Siege   Siege       `json:"siege"`
	Type    TypeEnchere `json:"action"`
	Contrat *Contrat    `json:"contrat,omitempty"`
}

func NouvellePasse() Enchere {
	return Enchere{Type: Passe}
}

func NouvelleAnnonce(atout Atout, valeur ValeurAnnonce) Enchere {
	return Enchere{Type: Annonce, Contrat: &Contrat{Atout: atout, Valeur: valeur}}
}

type OptionsEnchere struct {
	Passe      bool          `json:"passe"`
	AnnonceMin ValeurAnnonce `json:"annonce_min,omitempty"`
	Coinche    bool          `json:"coinche"`
	Surcoinche bool          `json:"surcoinche"`
}

// Autorise vérifie qu'une enchère respecte les options.
func (o OptionsEnchere) Autorise(e Enchere) bool {
	switch e.Type {
	case Passe:
		return o.Passe
	case Annonce:
		return o.AnnonceMin != 0 && e.Contrat != nil && e.Contrat.Valide() && e.Contrat.Valeur >= o.AnnonceMin
	case Coinche:
		return o.Coinche
	case Surcoinche:
		return o.Surcoinche
	default:
		return false
	}
}

type IssueEnchere struct {
	// EncheresTerminees : le contrat est fixé, le jeu de la carte commence.
	EncheresTerminees bool
	// Redistribution : les quatre joueurs ont passé, la mène est annulée.
	Redistribution bool
}

func (m *Mene) OptionsEnchere(s Siege) (OptionsEnchere, bool) {
	if m.Phase != PhaseEncheres || s != m.Tour {
		return OptionsEnchere{}, false
	}

	if len(m.attenteSurcoinche) > 0 {
		return OptionsEnchere{Passe: true, Surcoinche: true}, true
	}

	o := OptionsEnchere{Passe: true, AnnonceMin: A80}
	if m.Contrat != nil {
		o.AnnonceMin = m.Contrat.Valeur.Suivante()
		o.Coinche = m.Preneur.Equipe() != s.Equipe()
	}
	return o, true
}

func (m *Mene) Encherir(s Siege, e Enchere) (IssueEnchere, error) {
	if m.Phase != PhaseEncheres {
		return IssueEnchere{}, ErrPhase
	}
	if s != m.Tour {
		return IssueEnchere{}, ErrPasSonTour
	}

	options, _ := m.OptionsEnchere(s)
	if !options.Autorise(e) {
		return IssueEnchere{}, fmt.Errorf("%w : %s impossible", ErrEnchereInvalide, decrireEnchere(e))
	}

	e.Siege = s
	if e.Type != Annonce {
		e.Contrat = nil
	} else {
		c := *e.Contrat
		e.Contrat = &c
	}
	m.Encheres = append(m.Encheres, e)

	if len(m.attenteSurcoinche) > 0 {
		if e.Type == Surcoinche {
			m.Multiplicateur = 4
			return m.cloreEncheres(), nil
		}
		m.attenteSurcoinche = m.attenteSurcoinche[1:]
		if len(m.attenteSurcoinche) == 0 {
			return m.cloreEncheres(), nil
		}
		m.Tour = m.attenteSurcoinche[0]
		return IssueEnchere{}, nil
	}

	switch e.Type {
	case Passe:
		m.passesConsecutives++
		if m.Contrat == nil && m.passesConsecutives == NbSieges {
			m.Phase = PhaseTerminee
			m.Resultat = &ResultatMene{Redistribuee: true}
			return IssueEnchere{Redistribution: true}, nil
		}
		if m.Contrat != nil && m.passesConsecutives == NbSieges-1 {
			return m.cloreEncheres(), nil
		}
	case Annonce:
		m.Contrat = e.Contrat
		m.Preneur = s
		m.passesConsecutives = 0
	case Coinche:
		// La coinche clôt les enchères : seule l'équipe preneuse peut encore surcoincher.
		m.Multiplicateur = 2
		m.attenteSurcoinche = []Siege{s.Suivant(), s.Suivant().Partenaire()}
		m.Tour = m.attenteSurcoinche[0]
		return IssueEnchere{}, nil
	}

	m.Tour = s.Suivant()
	return IssueEnchere{}, nil
}

func (m *Mene) cloreEncheres() IssueEnchere {
	m.attenteSurcoinche = nil
	m.Phase = PhaseJeu
	m.Tour = m.Donneur.Suivant()
	m.PliEnCours = Pli{Entame: m.Tour}
	m.Belote = m.detenteurBelote()
	return IssueEnchere{EncheresTerminees: true}
}

func (m *Mene) detenteurBelote() *Siege {
	if !m.Contrat.Atout.EstCouleur() {
		return nil
	}
	couleur := Couleur(m.Contrat.Atout)
	for _, s := range Sieges {
		main := m.Mains[s]
		if main.Contient(Carte{couleur, Roi}) && main.Contient(Carte{couleur, Dame}) {
			return &s
		}
	}
	return nil
}

func decrireEnchere(e Enchere) string {
	if e.Type == Annonce && e.Contrat != nil {
		return "annonce " + e.Contrat.String()
	}
	return e.Type.String()
}
