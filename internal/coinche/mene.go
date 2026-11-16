package coinche

import "fmt"

const NbPlis = 8

type Phase int

const (
	PhaseEncheres Phase = iota
	PhaseJeu
	PhaseTerminee
)

var nomsPhases = [...]string{PhaseEncheres: "encheres", PhaseJeu: "jeu", PhaseTerminee: "terminee"}

func (p Phase) String() string {
	if p < PhaseEncheres || p > PhaseTerminee {
		return fmt.Sprintf("phase(%d)", int(p))
	}
	return nomsPhases[p]
}

func (p Phase) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

func (p *Phase) UnmarshalText(b []byte) error {
	for v, nom := range nomsPhases {
		if nom == string(b) {
			*p = Phase(v)
			return nil
		}
	}
	return fmt.Errorf("phase inconnue %q", string(b))
}

type AnnonceBelote int

const (
	PasDeBelote AnnonceBelote = iota
	BeloteAnnoncee
	RebeloteAnnoncee
)

func (a AnnonceBelote) String() string {
	switch a {
	case BeloteAnnoncee:
		return "belote"
	case RebeloteAnnoncee:
		return "rebelote"
	default:
		return ""
	}
}

func (a AnnonceBelote) MarshalText() ([]byte, error) {
	return []byte(a.String()), nil
}

func (a *AnnonceBelote) UnmarshalText(b []byte) error {
	for _, v := range [...]AnnonceBelote{PasDeBelote, BeloteAnnoncee, RebeloteAnnoncee} {
		if v.String() == string(b) {
			*a = v
			return nil
		}
	}
	return fmt.Errorf("annonce de belote inconnue %q", string(b))
}

type Mene struct {
	Numero         int
	Donneur        Siege
	MainsInitiales [NbSieges][]Carte
	Mains          [NbSieges]CarteCollection
	Encheres       []Enchere
	Contrat        *Contrat
	Preneur        Siege
	Multiplicateur int
	Phase          Phase
	Tour           Siege
	PliEnCours     Pli
	Plis           []Pli
	// Belote désigne le joueur qui détient roi et dame d'atout, connu dès la fin des enchères.
	Belote       *Siege
	BeloteJouees int
	Resultat     *ResultatMene

	passesConsecutives int
	attenteSurcoinche  []Siege
}

type IssueCarte struct {
	Belote AnnonceBelote
	// Pli est renseigné quand la carte complète le pli.
	Pli *Pli
	// Mene est renseignée quand la carte termine la mène.
	Mene *ResultatMene
	// PartieTerminee indique que la mène a fait atteindre l'objectif.
	PartieTerminee bool
}

func nouvelleMene(numero int, donneur Siege, paquet CarteCollection) *Mene {
	m := &Mene{
		Numero:         numero,
		Donneur:        donneur,
		Multiplicateur: 1,
		Phase:          PhaseEncheres,
		Tour:           donneur.Suivant(),
	}

	p := CarteCollection{Cartes: append([]Carte(nil), paquet.Cartes...)}
	for _, nb := range [...]int{3, 2, 3} {
		s := donneur.Suivant()
		for range NbSieges {
			m.Mains[s].Cartes = append(m.Mains[s].Cartes, p.Tirer(nb)...)
			s = s.Suivant()
		}
	}
	for _, s := range Sieges {
		m.MainsInitiales[s] = append([]Carte(nil), m.Mains[s].Cartes...)
	}
	return m
}

func (m *Mene) Terminee() bool {
	return m.Phase == PhaseTerminee
}

func (m *Mene) Demarree() bool {
	return m.Phase == PhaseJeu && (len(m.Plis) > 0 || len(m.PliEnCours.Cartes) > 0)
}

func (m *Mene) CartesJouables(s Siege) []Carte {
	if m.Phase != PhaseJeu || s != m.Tour {
		return nil
	}
	return CartesJouables(m.Mains[s].Cartes, m.PliEnCours, m.Contrat.Atout, s)
}

func (m *Mene) Jouer(s Siege, c Carte) (IssueCarte, error) {
	if m.Phase != PhaseJeu {
		return IssueCarte{}, ErrPhase
	}
	if s != m.Tour {
		return IssueCarte{}, ErrPasSonTour
	}
	if !m.Mains[s].Contient(c) {
		return IssueCarte{}, fmt.Errorf("%w : %v", ErrCarteAbsente, c)
	}
	atout := m.Contrat.Atout
	if !PeutJouer(m.Mains[s].Cartes, c, m.PliEnCours, atout, s) {
		return IssueCarte{}, ErrCarteInterdite
	}

	if _, err := m.Mains[s].Pop(c); err != nil {
		return IssueCarte{}, err
	}

	var issue IssueCarte
	if m.Belote != nil && *m.Belote == s && c.Couleur == Couleur(atout) && (c.Valeur == Roi || c.Valeur == Dame) {
		m.BeloteJouees++
		issue.Belote = AnnonceBelote(m.BeloteJouees)
	}

	m.PliEnCours.Cartes = append(m.PliEnCours.Cartes, CarteJouee{Siege: s, Carte: c})
	if !m.PliEnCours.Complet() {
		m.Tour = s.Suivant()
		return issue, nil
	}

	pli := m.PliEnCours
	pli.Gagnant = pli.Remporte(atout)
	m.Plis = append(m.Plis, pli)
	issue.Pli = &pli
	m.Tour = pli.Gagnant
	m.PliEnCours = Pli{Entame: pli.Gagnant}

	if len(m.Plis) == NbPlis {
		m.Phase = PhaseTerminee
	}
	return issue, nil
}

// Ramasser reforme le paquet pour une distribution traditionnelle :
// les plis dans l'ordre où ils ont été joués, ou les mains si tout le monde a passé.
func (m *Mene) Ramasser() CarteCollection {
	var cartes []Carte
	if len(m.Plis) == 0 {
		s := m.Donneur.Suivant()
		for range NbSieges {
			cartes = append(cartes, m.MainsInitiales[s]...)
			s = s.Suivant()
		}
		return CarteCollection{Cartes: cartes}
	}
	for _, pli := range m.Plis {
		cartes = append(cartes, pli.SCartes().Cartes...)
	}
	return CarteCollection{Cartes: cartes}
}
