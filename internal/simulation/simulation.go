// Package simulation fait jouer des parties complètes entre bots en vérifiant
// les invariants du moteur à chaque mène.
package simulation

import (
	"fmt"
	"strings"
	"time"

	"github.com/felix-marchenay/coinchenet-core/internal/bot"
	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
)

// MaxMenesParPartie protège contre une partie qui ne se terminerait jamais.
const MaxMenesParPartie = 200

type Rapport struct {
	Parties, Menes, Redistributions                   int
	Victoires                                         [2]int
	ContratsReussis, ContratsChutes, Capots, Coinches int
	PointsMoyensParMene                               float64
	Duree                                             time.Duration
}

func (r Rapport) String() string {
	jouees := r.ContratsReussis + r.ContratsChutes
	var b strings.Builder
	fmt.Fprintf(&b, "Parties : %d (équipe A %d, équipe B %d)\n", r.Parties, r.Victoires[coinche.EquipeA], r.Victoires[coinche.EquipeB])
	fmt.Fprintf(&b, "Mènes : %d dont %d redistributions\n", r.Menes, r.Redistributions)
	fmt.Fprintf(&b, "Contrats : %d réussis, %d chutés (%.1f %% de réussite)\n", r.ContratsReussis, r.ContratsChutes, pourcentage(r.ContratsReussis, jouees))
	fmt.Fprintf(&b, "Capots : %d, mènes coinchées : %d\n", r.Capots, r.Coinches)
	fmt.Fprintf(&b, "Points marqués par mène jouée : %.1f en moyenne\n", r.PointsMoyensParMene)
	fmt.Fprintf(&b, "Durée : %s", r.Duree.Round(time.Millisecond))
	return b.String()
}

func pourcentage(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(n) / float64(total)
}

func Simuler(nbParties int, graine uint64, regles coinche.Regles) (Rapport, error) {
	debut := time.Now()
	s := simulateur{
		hasard: coinche.NouveauHasardDeterministe(graine),
		regles: regles,
	}
	s.melangeur = coinche.FischerYatesMelangeur{Hasard: s.hasard}
	s.coupeur = coinche.RandomCoupeur{Hasard: s.hasard}

	for i := range nbParties {
		if err := s.jouerPartie(); err != nil {
			s.rapport.Duree = time.Since(debut)
			return s.rapport, fmt.Errorf("partie %d : %w", i+1, err)
		}
	}

	jouees := s.rapport.Menes - s.rapport.Redistributions
	if jouees > 0 {
		s.rapport.PointsMoyensParMene = float64(s.pointsMarques) / float64(jouees)
	}
	s.rapport.Duree = time.Since(debut)
	return s.rapport, nil
}

type simulateur struct {
	hasard        coinche.Hasard
	melangeur     coinche.Melangeur
	coupeur       coinche.Coupeur
	regles        coinche.Regles
	bot           bot.Bot
	rapport       Rapport
	pointsMarques int
}

func (s *simulateur) jouerPartie() error {
	p, err := coinche.NouvellePartie(s.regles, coinche.Siege(s.hasard.Intn(coinche.NbSieges)))
	if err != nil {
		return err
	}

	var cumul [2]int
	for !p.Terminee {
		if len(p.Menes) >= MaxMenesParPartie {
			return fmt.Errorf("partie non terminée après %d mènes (scores %v)", MaxMenesParPartie, p.Scores)
		}
		r, err := s.jouerMene(p)
		if err != nil {
			return fmt.Errorf("mène %d : %w", len(p.Menes), err)
		}
		cumul[coinche.EquipeA] += r.Points[coinche.EquipeA]
		cumul[coinche.EquipeB] += r.Points[coinche.EquipeB]
		if cumul != p.Scores {
			return fmt.Errorf("score cumulé %v différent de la somme des mènes %v", p.Scores, cumul)
		}
	}

	s.rapport.Parties++
	s.rapport.Victoires[p.Gagnante]++
	return nil
}

func (s *simulateur) jouerMene(p *coinche.Partie) (coinche.ResultatMene, error) {
	paquet := p.PaquetSuivant(s.melangeur, s.coupeur)
	if !paquet.EstPaquetComplet() {
		return coinche.ResultatMene{}, fmt.Errorf("paquet distribué incomplet : %s", coinche.SerializePaquet(paquet))
	}
	m, err := p.Distribuer(paquet)
	if err != nil {
		return coinche.ResultatMene{}, fmt.Errorf("distribution refusée : %w", err)
	}
	s.rapport.Menes++

	for m.Phase == coinche.PhaseEncheres {
		siege := m.Tour
		e := s.bot.Encherir(p.Vue(siege))
		if _, err := p.Encherir(siege, e); err != nil {
			return coinche.ResultatMene{}, fmt.Errorf("enchère %v du %v refusée : %w", e.Type, siege, err)
		}
	}
	if m.Resultat != nil && m.Resultat.Redistribuee {
		s.rapport.Redistributions++
		return *m.Resultat, nil
	}

	jouees := make(map[coinche.Carte]bool, 32)
	for !m.Terminee() {
		siege := m.Tour
		c := s.bot.Jouer(p.Vue(siege))
		if jouees[c] {
			return coinche.ResultatMene{}, fmt.Errorf("carte %v jouée deux fois", c)
		}
		jouees[c] = true
		if _, err := p.Jouer(siege, c); err != nil {
			return coinche.ResultatMene{}, fmt.Errorf("carte %v du %v refusée : %w", c, siege, err)
		}
	}
	if len(jouees) != 32 {
		return coinche.ResultatMene{}, fmt.Errorf("%d cartes jouées au lieu de 32", len(jouees))
	}

	r := m.Resultat
	if r == nil {
		return coinche.ResultatMene{}, fmt.Errorf("mène terminée sans résultat")
	}
	attendu := 162
	if r.Capot != nil {
		attendu = coinche.PointsCapot
		s.rapport.Capots++
	}
	if total := r.PointsFaits[coinche.EquipeA] + r.PointsFaits[coinche.EquipeB]; total != attendu {
		return coinche.ResultatMene{}, fmt.Errorf("points faits %v : total %d au lieu de %d", r.PointsFaits, total, attendu)
	}

	if r.Reussi {
		s.rapport.ContratsReussis++
	} else {
		s.rapport.ContratsChutes++
	}
	if r.Multiplicateur > 1 {
		s.rapport.Coinches++
	}
	s.pointsMarques += r.Points[coinche.EquipeA] + r.Points[coinche.EquipeB]
	return *r, nil
}
