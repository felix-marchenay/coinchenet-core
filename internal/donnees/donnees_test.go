package donnees

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestValiderPseudo(t *testing.T) {
	for _, p := range []string{"abc", "Felix_42", "le-roi-du-9", "12345678901234567890"} {
		assert.NoError(t, ValiderPseudo(p), p)
	}
	for _, p := range []string{"", "ab", "123456789012345678901", "avec espace", "émile", "a/b", "<script>"} {
		assert.ErrorIs(t, ValiderPseudo(p), ErrPseudoInvalide, p)
	}
}

func TestSuggererPseudo(t *testing.T) {
	assert.Equal(t, "Jean_Dupont", SuggererPseudo("Jean Dupont"))
	assert.Equal(t, "Jrme", SuggererPseudo("Jérôme"))
	assert.Equal(t, "Joueur", SuggererPseudo("李"))
	assert.Equal(t, "Joueur", SuggererPseudo(""))
	assert.Len(t, SuggererPseudo("unnomvraimenttrestreslongquidepasse"), PseudoMax-4)
	assert.NoError(t, ValiderPseudo(SuggererPseudo("  _-Bob-_  ")))
}

func TestParticipe(t *testing.T) {
	id := uuid.New()
	r := ResumePartie{Participants: []Participant{{Pseudo: "bot", Bot: true}, {JoueurID: &id}}}
	assert.True(t, r.Participe(id))
	assert.False(t, r.Participe(uuid.New()))
}
