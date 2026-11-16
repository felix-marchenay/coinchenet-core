package simulation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
)

func TestSimuler(t *testing.T) {
	contratSeul := coinche.ReglesParDefaut()
	contratSeul.Decompte = coinche.DecompteContratSeul

	tests := []struct {
		nom    string
		regles coinche.Regles
	}{
		{"règles classées", coinche.ReglesClassees()},
		{"distribution traditionnelle", coinche.ReglesParDefaut()},
		{"contrat seul", contratSeul},
	}

	for _, tt := range tests {
		t.Run(tt.nom, func(t *testing.T) {
			r, err := Simuler(300, 1, tt.regles)
			require.NoError(t, err)

			assert.Equal(t, 300, r.Parties)
			assert.Equal(t, 300, r.Victoires[coinche.EquipeA]+r.Victoires[coinche.EquipeB])
			assert.Equal(t, r.Menes-r.Redistributions, r.ContratsReussis+r.ContratsChutes)
			assert.Positive(t, r.ContratsReussis)
			assert.Positive(t, r.ContratsChutes)
			assert.Zero(t, r.Coinches, "le bot ne coinche jamais")
			assert.Positive(t, r.PointsMoyensParMene)
		})
	}
}

func TestSimulerEstReproductible(t *testing.T) {
	a, err := Simuler(50, 42, coinche.ReglesClassees())
	require.NoError(t, err)
	b, err := Simuler(50, 42, coinche.ReglesClassees())
	require.NoError(t, err)
	c, err := Simuler(50, 43, coinche.ReglesClassees())
	require.NoError(t, err)

	a.Duree, b.Duree, c.Duree = 0, 0, 0
	assert.Equal(t, a, b)
	assert.NotEqual(t, a, c)
}

func TestSimulerReglesInvalides(t *testing.T) {
	_, err := Simuler(1, 1, coinche.Regles{Objectif: 123})
	assert.Error(t, err)
}

func TestRapportString(t *testing.T) {
	r := Rapport{Parties: 2, Victoires: [2]int{1, 1}, Menes: 10, Redistributions: 1, ContratsReussis: 6, ContratsChutes: 3}

	s := r.String()

	assert.Contains(t, s, "Parties : 2 (équipe A 1, équipe B 1)")
	assert.Contains(t, s, "66.7 % de réussite")
}
