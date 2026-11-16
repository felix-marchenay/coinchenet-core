package config

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func env(valeurs map[string]string) func(string) string {
	return func(nom string) string { return valeurs[nom] }
}

func minimal() map[string]string {
	return map[string]string{
		"COINCHE_POSTGRES_URL": "postgres://u:p@db/coinche",
		"COINCHE_REDIS_URL":    "redis://redis:6379/0",
	}
}

func TestValeursParDefaut(t *testing.T) {
	c, err := Charger(env(minimal()))
	require.NoError(t, err)

	assert.Equal(t, ":8080", c.Adresse)
	assert.Equal(t, ":9090", c.AdresseAdmin)
	assert.Equal(t, "http://localhost:8080", c.URLPublique)
	assert.Equal(t, c.URLPublique, c.URLFront)
	assert.Equal(t, []string{"http://localhost:8080"}, c.Origines)
	assert.True(t, c.CookieSecure)
	assert.False(t, c.AuthDev)
	assert.False(t, c.Google.Active())
	assert.Equal(t, 30, c.DelaiActionClassee)
	assert.Equal(t, 3*time.Minute, c.DelaiRetour)
	assert.Equal(t, slog.LevelInfo, c.LogNiveau)
	assert.NotEmpty(t, c.Instance)
}

func TestSurcharges(t *testing.T) {
	v := minimal()
	v["COINCHE_URL_PUBLIQUE"] = "https://api.coinche.net/"
	v["COINCHE_URL_FRONT"] = "https://coinche.net"
	v["COINCHE_ORIGINES"] = "https://coinche.net, https://www.coinche.net/"
	v["COINCHE_COOKIE_SECURE"] = "false"
	v["COINCHE_GOOGLE_CLIENT_ID"] = "id"
	v["COINCHE_GOOGLE_CLIENT_SECRET"] = "secret"
	v["COINCHE_DELAI_RETOUR"] = "90s"
	v["COINCHE_LOG_NIVEAU"] = "debug"
	v["COINCHE_DELAI_ACTION_CLASSEE"] = "60"

	c, err := Charger(env(v))
	require.NoError(t, err)

	assert.Equal(t, "https://api.coinche.net", c.URLPublique)
	assert.Equal(t, []string{"https://coinche.net", "https://www.coinche.net"}, c.Origines)
	assert.Equal(t, []string{"coinche.net", "www.coinche.net"}, c.HotesOrigines())
	assert.False(t, c.CookieSecure)
	assert.True(t, c.Google.Active())
	assert.Equal(t, 90*time.Second, c.DelaiRetour)
	assert.Equal(t, slog.LevelDebug, c.LogNiveau)
	assert.Equal(t, 60, c.DelaiActionClassee)
}

func TestErreurs(t *testing.T) {
	_, err := Charger(env(map[string]string{
		"COINCHE_COOKIE_SECURE":        "peut-être",
		"COINCHE_DELAI_RETOUR":         "trois minutes",
		"COINCHE_URL_FRONT":            "coinche.net",
		"COINCHE_DELAI_ACTION_CLASSEE": "45",
		"COINCHE_LOG_FORMAT":           "xml",
	}))
	require.Error(t, err)
	for _, attendu := range []string{"COINCHE_POSTGRES_URL", "COINCHE_REDIS_URL", "COINCHE_COOKIE_SECURE", "COINCHE_DELAI_RETOUR", "COINCHE_URL_FRONT", "COINCHE_DELAI_ACTION_CLASSEE", "COINCHE_LOG_FORMAT"} {
		assert.Contains(t, err.Error(), attendu)
	}
}
