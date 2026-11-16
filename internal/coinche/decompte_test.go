package coinche

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// plisParValeur forme 8 plis, un par valeur (7, 8, 9, 10, V, D, R, A), chacun avec les 4 couleurs.
// À l'atout cœur ils valent 0, 0, 14, 40, 26, 12, 16 et 44 points (+10 de der pour le dernier).
func plisParValeur(gagnants [NbPlis]Siege) []Pli {
	plis := make([]Pli, NbPlis)
	for i, v := range Valeurs {
		p := Pli{Entame: gagnants[i], Gagnant: gagnants[i]}
		for _, c := range Couleurs {
			p.Cartes = append(p.Cartes, CarteJouee{Siege: Siege(c), Carte: Carte{c, v}})
		}
		plis[i] = p
	}
	return plis
}

func meneJouee(contrat Contrat, preneur Siege, multiplicateur int, gagnants [NbPlis]Siege, belote *Siege) *Mene {
	return &Mene{
		Contrat:        &contrat,
		Preneur:        preneur,
		Multiplicateur: multiplicateur,
		Plis:           plisParValeur(gagnants),
		Belote:         belote,
		Phase:          PhaseTerminee,
	}
}

func siege(s Siege) *Siege {
	return &s
}

func equipe(e Equipe) *Equipe {
	return &e
}

func TestDecompter(t *testing.T) {
	// L'équipe A remporte 9, 10, V et A : 124 points + 10 de der = 134 ; B fait 28.
	aFait134 := [NbPlis]Siege{1, 3, 0, 2, 0, 1, 3, 0}
	// A remporte 7, 8, V et A : 80 points ; B fait 82.
	aFait80 := [NbPlis]Siege{0, 0, 1, 3, 2, 1, 3, 0}
	toutPourA := [NbPlis]Siege{0, 2, 0, 2, 0, 2, 0, 0}
	toutPourLe0 := [NbPlis]Siege{0, 0, 0, 0, 0, 0, 0, 0}
	septPlisPourA := [NbPlis]Siege{1, 2, 0, 2, 0, 2, 0, 0}

	tests := []struct {
		nom            string
		decompte       Decompte
		contrat        Contrat
		multiplicateur int
		gagnants       [NbPlis]Siege
		belote         *Siege
		reussi         bool
		faits          [2]int
		points         [2]int
		capot          *Equipe
	}{
		{
			nom: "contrat réussi : points faits + contrat", contrat: Contrat{AtoutCoeur, A80}, multiplicateur: 1,
			gagnants: aFait134, reussi: true, faits: [2]int{134, 28}, points: [2]int{214, 28},
		},
		{
			nom: "contrat chuté : 160 + contrat pour la défense", contrat: Contrat{AtoutCoeur, A140}, multiplicateur: 1,
			gagnants: aFait134, reussi: false, faits: [2]int{134, 28}, points: [2]int{0, 300},
		},
		{
			nom: "la belote compte pour remplir le contrat", contrat: Contrat{AtoutCoeur, A140}, multiplicateur: 1,
			gagnants: aFait134, belote: siege(2), reussi: true, faits: [2]int{134, 28}, points: [2]int{294, 28},
		},
		{
			nom: "la belote reste acquise à la défense quand le contrat chute", contrat: Contrat{AtoutCoeur, A140}, multiplicateur: 1,
			gagnants: aFait134, belote: siege(1), reussi: false, faits: [2]int{134, 28}, points: [2]int{0, 320},
		},
		{
			nom: "il faut faire plus que la défense", contrat: Contrat{AtoutCoeur, A80}, multiplicateur: 1,
			gagnants: aFait80, reussi: false, faits: [2]int{80, 82}, points: [2]int{0, 240},
		},
		{
			nom: "coinché réussi : 160 + contrat × 2", contrat: Contrat{AtoutCoeur, A80}, multiplicateur: 2,
			gagnants: aFait134, reussi: true, faits: [2]int{134, 28}, points: [2]int{320, 0},
		},
		{
			nom: "surcoinché chuté : 160 + contrat × 4 pour la défense", contrat: Contrat{AtoutCoeur, A140}, multiplicateur: 4,
			gagnants: aFait134, reussi: false, faits: [2]int{134, 28}, points: [2]int{0, 720},
		},
		{
			nom: "capot non annoncé : 250 points faits", contrat: Contrat{AtoutCoeur, A80}, multiplicateur: 1,
			gagnants: toutPourA, reussi: true, faits: [2]int{250, 0}, points: [2]int{330, 0}, capot: equipe(EquipeA),
		},
		{
			nom: "capot annoncé réussi : 250 + 250", contrat: Contrat{AtoutCoeur, Capot}, multiplicateur: 1,
			gagnants: toutPourA, reussi: true, faits: [2]int{250, 0}, points: [2]int{500, 0}, capot: equipe(EquipeA),
		},
		{
			nom: "capot annoncé chuté : 160 + 250 pour la défense", contrat: Contrat{AtoutCoeur, Capot}, multiplicateur: 1,
			gagnants: septPlisPourA, reussi: false, faits: [2]int{162, 0}, points: [2]int{0, 410},
		},
		{
			nom: "générale réussie : 250 + 500", contrat: Contrat{AtoutCoeur, Generale}, multiplicateur: 1,
			gagnants: toutPourLe0, reussi: true, faits: [2]int{250, 0}, points: [2]int{750, 0}, capot: equipe(EquipeA),
		},
		{
			nom: "générale chutée si le partenaire prend un pli", contrat: Contrat{AtoutCoeur, Generale}, multiplicateur: 1,
			gagnants: toutPourA, reussi: false, faits: [2]int{250, 0}, points: [2]int{0, 660}, capot: equipe(EquipeA),
		},
		{
			nom: "chuté par un capot de la défense : 250 + contrat", contrat: Contrat{AtoutCoeur, A80}, multiplicateur: 1,
			gagnants: [NbPlis]Siege{1, 1, 3, 1, 3, 1, 1, 3}, reussi: false, faits: [2]int{0, 250}, points: [2]int{0, 330}, capot: equipe(EquipeB),
		},
		{
			nom: "contrat seul réussi", decompte: DecompteContratSeul, contrat: Contrat{AtoutCoeur, A80}, multiplicateur: 1,
			gagnants: aFait134, reussi: true, faits: [2]int{134, 28}, points: [2]int{80, 0},
		},
		{
			nom: "contrat seul coinché chuté, avec belote", decompte: DecompteContratSeul, contrat: Contrat{AtoutCoeur, A160}, multiplicateur: 2,
			gagnants: aFait134, belote: siege(0), reussi: false, faits: [2]int{134, 28}, points: [2]int{20, 320},
		},
	}

	for _, tt := range tests {
		t.Run(tt.nom, func(t *testing.T) {
			m := meneJouee(tt.contrat, 0, tt.multiplicateur, tt.gagnants, tt.belote)

			r := Decompter(tt.decompte, m)

			assert.Equal(t, tt.reussi, r.Reussi, "réussite")
			assert.Equal(t, tt.faits, r.PointsFaits, "points faits")
			assert.Equal(t, tt.points, r.Points, "points marqués")
			assert.Equal(t, tt.capot, r.Capot, "capot")
			assert.Equal(t, tt.multiplicateur, r.Multiplicateur)
		})
	}
}

func TestDecompterPreneurEquipeB(t *testing.T) {
	gagnants := [NbPlis]Siege{0, 2, 1, 3, 1, 0, 2, 3}
	m := meneJouee(Contrat{AtoutCoeur, A80}, 3, 1, gagnants, nil)

	r := Decompter(DecompteFaitsEtContrat, m)

	assert.True(t, r.Reussi)
	assert.Equal(t, [2]int{28, 134}, r.PointsFaits)
	assert.Equal(t, [2]int{28, 214}, r.Points)
	assert.Equal(t, [2]int{4, 4}, r.PlisGagnes)
}
