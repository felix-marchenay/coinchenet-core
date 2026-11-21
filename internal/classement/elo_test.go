package classement

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
)

func participants(elos [4]int, nbParties int) []Participant {
	ps := make([]Participant, 4)
	for i, elo := range elos {
		ps[i] = Participant{JoueurID: uuid.New(), Equipe: coinche.Siege(i).Equipe(), Elo: elo, NbParties: nbParties}
	}
	return ps
}

func deltas(vs []Variation) []int {
	d := make([]int, len(vs))
	for i, v := range vs {
		d[i] = v.Delta
	}
	return d
}

func TestEsperance(t *testing.T) {
	assert.InDelta(t, 0.5, Esperance(1500, 1500), 1e-9)
	assert.InDelta(t, 0.909, Esperance(1800, 1400), 1e-3)
	assert.InDelta(t, 1, Esperance(1800, 1400)+Esperance(1400, 1800), 1e-9)
}

func TestK(t *testing.T) {
	assert.Equal(t, KDebutant, K(0))
	assert.Equal(t, KDebutant, K(SeuilConfirme-1))
	assert.Equal(t, KConfirme, K(SeuilConfirme))
}

func TestEquipesDeMemeNiveau(t *testing.T) {
	vs := Calculer(participants([4]int{1500, 1500, 1500, 1500}, 100), coinche.EquipeA)

	assert.Equal(t, []int{10, -10, 10, -10}, deltas(vs))
	assert.Equal(t, 1510, vs[0].Apres)
	assert.Equal(t, 1500, vs[0].Avant)
}

func TestMoyenneDEquipe(t *testing.T) {
	// A : (1700 + 1300) / 2 = 1500, B : 1500 → mêmes variations qu'à niveau égal.
	vs := Calculer(participants([4]int{1700, 1500, 1300, 1500}, 100), coinche.EquipeB)

	assert.Equal(t, []int{-10, 10, -10, 10}, deltas(vs))
}

func TestUnBotCompteComme1200(t *testing.T) {
	// A : un humain à 1600 et un bot, (1600 + 1200) / 2 = 1400 ; B : 1400 → comme à niveau égal.
	ps := []Participant{
		{JoueurID: uuid.New(), Equipe: coinche.EquipeA, Elo: 1600, NbParties: 100},
		{JoueurID: uuid.New(), Equipe: coinche.EquipeB, Elo: 1400, NbParties: 100},
		{JoueurID: uuid.New(), Equipe: coinche.EquipeB, Elo: 1400, NbParties: 100},
	}
	assert.Equal(t, []int{10, -10, -10}, deltas(Calculer(ps, coinche.EquipeA)))
}

func TestVictoireDuFavori(t *testing.T) {
	vs := Calculer(participants([4]int{1800, 1400, 1800, 1400}, 100), coinche.EquipeA)
	assert.Equal(t, []int{2, -2, 2, -2}, deltas(vs), "le favori gagne peu")

	vs = Calculer(participants([4]int{1800, 1400, 1800, 1400}, 100), coinche.EquipeB)
	assert.Equal(t, []int{-18, 18, -18, 18}, deltas(vs), "l'outsider gagne beaucoup")
}

func TestDebutantsVarientPlus(t *testing.T) {
	ps := participants([4]int{1200, 1200, 1200, 1200}, 100)
	ps[0].NbParties = 3

	vs := Calculer(ps, coinche.EquipeA)

	assert.Equal(t, []int{20, -10, 10, -10}, deltas(vs))
}

func TestAbandon(t *testing.T) {
	ps := participants([4]int{1500, 1500, 1500, 1500}, 100)
	ps[0].Abandon = true

	vs := Calculer(ps, coinche.EquipeB)

	assert.Equal(t, -10-PenaliteAbandon, vs[0].Delta, "l'auteur de l'abandon est pénalisé")
	assert.Equal(t, 0, vs[2].Delta, "son partenaire ne perd rien")
	assert.Equal(t, 10, vs[1].Delta, "les adversaires gagnent normalement")
	assert.Equal(t, 10, vs[3].Delta)
}

func TestPlancher(t *testing.T) {
	ps := participants([4]int{EloPlancher + 3, 1500, EloPlancher, 1500}, 0)
	ps[0].Abandon = true

	vs := Calculer(ps, coinche.EquipeB)

	assert.Equal(t, EloPlancher, vs[0].Apres)
	assert.Equal(t, -3, vs[0].Delta)
	assert.Equal(t, EloPlancher, vs[2].Apres)
}
