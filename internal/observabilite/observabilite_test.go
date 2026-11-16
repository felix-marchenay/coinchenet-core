package observabilite

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/felix-marchenay/coinchenet-core/internal/table"
)

func TestMetriques(t *testing.T) {
	m := NouvellesMetriques(func() map[table.Type]int { return map[table.Type]int{table.Classee: 3, table.Privee: 1} })
	m.PartieTerminee(table.Classee, table.CauseForfait)
	m.ActionTraitee("jouer", time.Millisecond)
	m.JoueursEnFile(7)
	m.Apparie(12 * time.Second)
	m.ConnexionOuverte()
	m.Requete(http.MethodGet, "GET /classement", http.StatusOK, 5*time.Millisecond)
	m.Requete(http.MethodGet, "", http.StatusNotFound, time.Millisecond)

	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, w.Code)
	corps, _ := io.ReadAll(w.Body)

	for _, attendu := range []string{
		`coinche_tables_actives{type="classee"} 3`,
		`coinche_tables_actives{type="privee"} 1`,
		`coinche_parties_terminees_total{cause="forfait",type="classee"} 1`,
		`coinche_file_joueurs 7`,
		`coinche_websocket_connexions 1`,
		`coinche_http_requetes_secondes_count{methode="GET",route="inconnue",statut="404"} 1`,
		`coinche_table_commande_secondes_count{commande="jouer"} 1`,
		"go_goroutines",
	} {
		assert.Contains(t, string(corps), attendu)
	}
}

func TestLogger(t *testing.T) {
	var b bytes.Buffer
	NouveauLogger(&b, slog.LevelInfo, "json").Debug("invisible")
	NouveauLogger(&b, slog.LevelInfo, "json").Info("visible", "cle", 1)
	assert.NotContains(t, b.String(), "invisible")
	assert.Contains(t, b.String(), `"msg":"visible"`)

	b.Reset()
	NouveauLogger(&b, slog.LevelDebug, "texte").Debug("texte")
	assert.Contains(t, b.String(), "msg=texte")
}
