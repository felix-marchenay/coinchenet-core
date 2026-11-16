package bot

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
)

// Sur des positions issues de vraies parties, chaque décision du bot est légale et reproductible.
func TestDecisionsToujoursLegales(t *testing.T) {
	h := coinche.NouveauHasardDeterministe(2026)
	melangeur := coinche.FischerYatesMelangeur{Hasard: h}
	coupeur := coinche.RandomCoupeur{Hasard: h}
	var b Bot
	encheres, cartes := 0, 0

	for _, regles := range []coinche.Regles{coinche.ReglesClassees(), coinche.ReglesParDefaut()} {
		for range 20 {
			p, err := coinche.NouvellePartie(regles, coinche.Siege(h.Intn(coinche.NbSieges)))
			require.NoError(t, err)

			for !p.Terminee {
				require.Less(t, len(p.Menes), 200)
				m, err := p.Distribuer(p.PaquetSuivant(melangeur, coupeur))
				require.NoError(t, err)

				for !m.Terminee() {
					s := m.Tour
					v := p.Vue(s)
					if m.Phase == coinche.PhaseEncheres {
						e := b.Encherir(v)
						require.NotNil(t, v.Mene.OptionsEnchere)
						assert.True(t, v.Mene.OptionsEnchere.Autorise(e), "enchère %+v refusée par %+v", e, *v.Mene.OptionsEnchere)
						assert.Equal(t, e, b.Encherir(v), "décision non déterministe")
						_, err := p.Encherir(s, e)
						require.NoError(t, err)
						encheres++
						continue
					}
					c := b.Jouer(v)
					assert.Contains(t, v.Mene.CartesJouables, c)
					assert.Equal(t, c, b.Jouer(v), "décision non déterministe")
					_, err := p.Jouer(s, c)
					require.NoError(t, err)
					cartes++
				}
			}
		}
	}

	assert.Greater(t, encheres, 500)
	assert.Greater(t, cartes, 5000)
}
