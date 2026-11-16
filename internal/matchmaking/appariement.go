// Package matchmaking forme les tables de la file classée, en solo ou en duo.
package matchmaking

import (
	"cmp"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/felix-marchenay/coinchenet-core/internal/table"
)

// Ticket représente une entrée dans la file : un joueur seul ou un duo.
type Ticket struct {
	ID       string         `json:"id"`
	Joueurs  []table.Joueur `json:"joueurs"`
	EntreeLe time.Time      `json:"entree_le"`
}

func (t Ticket) Duo() bool {
	return len(t.Joueurs) == 2
}

func (t Ticket) Elo() float64 {
	somme := 0
	for _, j := range t.Joueurs {
		somme += j.Elo
	}
	return float64(somme) / float64(len(t.Joueurs))
}

type Parametres struct {
	FenetreInitiale  int
	Elargissement    int
	PasElargissement time.Duration
	FenetreMax       int
	// AttenteDuoContreSolos : un duo n'affronte deux joueurs seuls qu'après avoir attendu ce délai.
	AttenteDuoContreSolos time.Duration
	DelaiAcceptation      time.Duration
	DelaiInvitation       time.Duration
	// AttenteParBot : sans assez d'adversaires, un bot rejoint la table du plus ancien ticket à chaque
	// palier de cette attente, jusqu'à la compléter. 0 désactive les bots.
	AttenteParBot time.Duration
}

func ParametresParDefaut() Parametres {
	return Parametres{
		FenetreInitiale:       100,
		Elargissement:         50,
		PasElargissement:      15 * time.Second,
		FenetreMax:            400,
		AttenteDuoContreSolos: 30 * time.Second,
		DelaiAcceptation:      15 * time.Second,
		DelaiInvitation:       time.Minute,
		AttenteParBot:         5 * time.Second,
	}
}

// Fenetre est l'écart d'ELO toléré, qui s'élargit avec l'attente.
func (p Parametres) Fenetre(attente time.Duration) int {
	if attente < 0 {
		attente = 0
	}
	pas := int(attente / p.PasElargissement)
	return min(p.FenetreInitiale+pas*p.Elargissement, p.FenetreMax)
}

// Appariement : un Joueur zéro dans une équipe est un siège tenu par un bot.
type Appariement struct {
	EquipeA [2]table.Joueur
	EquipeB [2]table.Joueur
	Tickets []Ticket
}

// Apparier forme le plus de tables possible, en servant d'abord les tickets qui attendent depuis le plus
// longtemps. Les tables entre humains passent avant celles que des bots complètent.
func Apparier(tickets []Ticket, maintenant time.Time, p Parametres) []Appariement {
	ordre := slices.Clone(tickets)
	slices.SortStableFunc(ordre, func(a, b Ticket) int {
		return cmp.Or(a.EntreeLe.Compare(b.EntreeLe), cmp.Compare(a.ID, b.ID))
	})

	utilises := map[string]bool{}
	res := apparierHumains(ordre, utilises, maintenant, p)
	if p.AttenteParBot <= 0 {
		return res
	}
	for _, pivot := range ordre {
		if utilises[pivot.ID] {
			continue
		}
		var autres []Ticket
		for _, c := range ordre {
			if c.ID != pivot.ID && !utilises[c.ID] {
				autres = append(autres, c)
			}
		}
		slices.SortStableFunc(autres, func(a, b Ticket) int {
			return cmp.Compare(math.Abs(a.Elo()-pivot.Elo()), math.Abs(b.Elo()-pivot.Elo()))
		})
		a := avecBots(pivot, autres, maintenant.Sub(pivot.EntreeLe), p)
		if a == nil {
			continue
		}
		for _, t := range a.Tickets {
			utilises[t.ID] = true
		}
		res = append(res, *a)
	}
	return res
}

func apparierHumains(ordre []Ticket, utilises map[string]bool, maintenant time.Time, p Parametres) []Appariement {
	var res []Appariement
	for _, pivot := range ordre {
		if utilises[pivot.ID] {
			continue
		}
		fenetre := float64(p.Fenetre(maintenant.Sub(pivot.EntreeLe)))
		var duos, solos []Ticket
		for _, c := range ordre {
			if c.ID == pivot.ID || utilises[c.ID] || math.Abs(c.Elo()-pivot.Elo()) > fenetre {
				continue
			}
			if c.Duo() {
				duos = append(duos, c)
			} else {
				solos = append(solos, c)
			}
		}
		parProximite := func(a, b Ticket) int {
			return cmp.Compare(math.Abs(a.Elo()-pivot.Elo()), math.Abs(b.Elo()-pivot.Elo()))
		}
		slices.SortStableFunc(duos, parProximite)
		slices.SortStableFunc(solos, parProximite)
		patient := func(t Ticket) bool {
			return maintenant.Sub(t.EntreeLe) >= p.AttenteDuoContreSolos
		}

		var a *Appariement
		if pivot.Duo() {
			switch {
			case len(duos) > 0:
				a = duoContreDuo(pivot, duos[0])
			case len(solos) >= 2 && patient(pivot):
				a = duoContreSolos(pivot, solos[0], solos[1])
			}
		} else {
			switch {
			case len(solos) >= 3:
				a = quatreSolos(pivot, solos[0], solos[1], solos[2])
			case len(solos) >= 1 && len(duos) > 0 && patient(duos[0]):
				a = duoContreSolos(duos[0], pivot, solos[0])
			}
		}
		if a == nil {
			continue
		}
		for _, t := range a.Tickets {
			utilises[t.ID] = true
		}
		res = append(res, *a)
	}
	return res
}

// avecBots réunit au pivot jusqu'à trois humains de la file, quel que soit leur ELO : la partie ne
// sera pas classée. Elle part dès que les bots que l'attente du pivot autorise complètent la table.
func avecBots(pivot Ticket, autres []Ticket, attente time.Duration, p Parametres) *Appariement {
	bots := min(3, int(attente/p.AttenteParBot))
	if bots == 0 {
		return nil
	}
	tickets := []Ticket{pivot}
	humains := len(pivot.Joueurs)
	for _, t := range autres {
		if humains+len(t.Joueurs) <= 3 {
			tickets = append(tickets, t)
			humains += len(t.Joueurs)
		}
	}
	if humains+bots < 4 {
		return nil
	}

	a := &Appariement{Tickets: tickets}
	var solos []table.Joueur
	for _, t := range tickets {
		if t.Duo() {
			a.EquipeA = [2]table.Joueur{t.Joueurs[0], t.Joueurs[1]}
		} else {
			solos = append(solos, t.Joueurs[0])
		}
	}
	slices.SortStableFunc(solos, func(a, b table.Joueur) int { return cmp.Compare(b.Elo, a.Elo) })
	// Un duo affronte les autres ; sinon le plus fort fait équipe avec un bot.
	if a.EquipeA[0].ID == uuid.Nil {
		a.EquipeA[0], solos = solos[0], solos[1:]
	}
	copy(a.EquipeB[:], solos)
	return a
}

func duoContreDuo(a, b Ticket) *Appariement {
	return &Appariement{
		EquipeA: [2]table.Joueur{a.Joueurs[0], a.Joueurs[1]},
		EquipeB: [2]table.Joueur{b.Joueurs[0], b.Joueurs[1]},
		Tickets: []Ticket{a, b},
	}
}

func duoContreSolos(duo, s1, s2 Ticket) *Appariement {
	return &Appariement{
		EquipeA: [2]table.Joueur{duo.Joueurs[0], duo.Joueurs[1]},
		EquipeB: [2]table.Joueur{s1.Joueurs[0], s2.Joueurs[0]},
		Tickets: []Ticket{duo, s1, s2},
	}
}

// quatreSolos équilibre les équipes : le plus fort avec le plus faible.
func quatreSolos(tickets ...Ticket) *Appariement {
	joueurs := make([]table.Joueur, 0, 4)
	for _, t := range tickets {
		joueurs = append(joueurs, t.Joueurs[0])
	}
	slices.SortStableFunc(joueurs, func(a, b table.Joueur) int { return cmp.Compare(b.Elo, a.Elo) })
	return &Appariement{
		EquipeA: [2]table.Joueur{joueurs[0], joueurs[3]},
		EquipeB: [2]table.Joueur{joueurs[1], joueurs[2]},
		Tickets: tickets,
	}
}
