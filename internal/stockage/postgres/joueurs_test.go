package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/felix-marchenay/coinchenet-core/internal/donnees"
)

func TestPseudoAvecSuffixe(t *testing.T) {
	assert.Equal(t, "Bob0042", pseudoAvecSuffixe("Bob", 42))
	assert.Equal(t, "Joueur9999", pseudoAvecSuffixe("Joueur", 9999))
	assert.Equal(t, "Joueur0001", pseudoAvecSuffixe("Joueur", 10001))

	long := pseudoAvecSuffixe("abcdefghijklmnopqrstuvwxyz", 7)
	assert.Equal(t, "abcdefghijklmnop0007", long)
	assert.NoError(t, donnees.ValiderPseudo(long))
}

func TestLireMigrations(t *testing.T) {
	ms, err := lireMigrations()
	assert.NoError(t, err)
	assert.NotEmpty(t, ms)
	for i, m := range ms {
		assert.Equal(t, i+1, m.version, "versions contiguës et triées")
		assert.NotEmpty(t, m.sql)
	}
}
