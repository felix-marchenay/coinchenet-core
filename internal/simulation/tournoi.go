package simulation

import (
	"fmt"
	"math"
	"strings"

	"github.com/felix-marchenay/coinchenet-core/internal/bot"
	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
)

// LigneTournoi résume un profil : sa force estimée et sa façon de jouer.
type LigneTournoi struct {
	Profil                  bot.Profil
	Elo                     int
	Parties, Victoires      int
	Menes                   int
	Prises, Reussies        int
	Coinches, CapotsAnnonce int
}

// Tournoi oppose chaque paire de profils (deux bots identiques par équipe) sur nbParties parties.
// L'ELO de chaque profil vaut 1200 plus la moyenne des écarts déduits de ses taux de victoire.
func Tournoi(nbParties int, graine uint64, regles coinche.Regles, profils []bot.Profil) ([]LigneTournoi, error) {
	lignes := make([]LigneTournoi, len(profils))
	ecarts := make([]float64, len(profils))
	for i, p := range profils {
		lignes[i].Profil = p
	}

	rencontre := uint64(0)
	for i := range profils {
		for j := i + 1; j < len(profils); j++ {
			rencontre++
			r, err := SimulerEntre(nbParties, graine+rencontre, regles, [2]bot.Profil{profils[i], profils[j]})
			if err != nil {
				return nil, fmt.Errorf("%s contre %s : %w", profils[i].Nom, profils[j].Nom, err)
			}
			jouees := r.Menes - r.Redistributions
			for k, idx := range [2]int{i, j} {
				l := &lignes[idx]
				l.Parties += r.Parties
				l.Victoires += r.Victoires[k]
				l.Menes += jouees
				l.Prises += r.Equipes[k].Prises
				l.Reussies += r.Equipes[k].Reussies
				l.Coinches += r.Equipes[k].Coinches
				l.CapotsAnnonce += r.Equipes[k].CapotsAnnonces
			}
			e := ecartElo(float64(r.Victoires[coinche.EquipeA]) / float64(r.Parties))
			ecarts[i] += e
			ecarts[j] -= e
		}
	}
	for i := range lignes {
		lignes[i].Elo = 1200
		if len(profils) > 1 {
			lignes[i].Elo += int(math.Round(ecarts[i] / float64(len(profils)-1)))
		}
	}
	return lignes, nil
}

// ecartElo est l'écart de classement qui correspond à un taux de victoire.
func ecartElo(taux float64) float64 {
	taux = min(max(taux, 0.005), 0.995)
	return 400 * math.Log10(taux/(1-taux))
}

func FormaterTournoi(lignes []LigneTournoi) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-22s %5s %10s %13s %9s %12s %8s\n", "Profil", "ELO", "Victoires", "Prises/mène", "Réussite", "Coinches %", "Capots")
	for _, l := range lignes {
		fmt.Fprintf(&b, "%-22s %5d %9.1f%% %12.1f%% %8.1f%% %11.1f%% %8d\n",
			l.Profil.Nom+" ("+l.Profil.Style+")", l.Elo,
			pourcentage(l.Victoires, l.Parties),
			pourcentage(l.Prises, l.Menes),
			pourcentage(l.Reussies, l.Prises),
			pourcentage(l.Coinches, l.Menes),
			l.CapotsAnnonce)
	}
	return b.String()
}
