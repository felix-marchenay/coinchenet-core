package simulation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/felix-marchenay/coinchenet-core/internal/bot"
	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
)

func TestTournoi(t *testing.T) {
	lignes, err := Tournoi(30, 1, coinche.ReglesClassees(), bot.Profils)
	require.NoError(t, err)
	require.Len(t, lignes, len(bot.Profils))

	somme := 0
	for i, l := range lignes {
		assert.Equal(t, bot.Profils[i].ID, l.Profil.ID)
		assert.Equal(t, 3*30, l.Parties, "chaque profil rencontre les trois autres")
		assert.Positive(t, l.Prises)
		somme += l.Elo
	}
	assert.InDelta(t, 4*1200, somme, 2, "les écarts se compensent")
	assert.Contains(t, FormaterTournoi(lignes), "Léo (casse-cou)")
}

func TestEcartElo(t *testing.T) {
	assert.InDelta(t, 0, ecartElo(0.5), 1e-9)
	assert.InDelta(t, 191, ecartElo(0.75), 1)
	assert.InDelta(t, -ecartElo(0.75), ecartElo(0.25), 1e-9)
	assert.InDelta(t, ecartElo(0.995), ecartElo(1), 1e-9, "un taux extrême est borné")
}
