// Package config lit la configuration du serveur depuis les variables d'environnement.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

type OAuth struct {
	ClientID     string
	ClientSecret string
}

func (o OAuth) Active() bool {
	return o.ClientID != "" && o.ClientSecret != ""
}

type Config struct {
	Adresse      string
	AdresseAdmin string
	PostgresURL  string
	RedisURL     string
	// URLPublique est l'adresse du serveur vue des navigateurs (retours OAuth).
	URLPublique string
	// URLFront est l'adresse du client web, où l'on renvoie après connexion.
	URLFront          string
	Origines          []string
	CookieSecure      bool
	CookieDomaine     string
	DureeSession      time.Duration
	Google            OAuth
	Discord           OAuth
	AuthDev           bool
	Instance          string
	LogNiveau         slog.Level
	LogFormat         string
	MigrerAuDemarrage bool
	ArretGracieux     time.Duration
	// DebitWS : messages par seconde acceptés d'un client WebSocket.
	DebitWS int

	DelaiActionClassee int
	DelaiBot           time.Duration
	DelaiDemarrage     time.Duration
	DelaiEntreMenes    time.Duration
	DelaiRetour        time.Duration
	DelaiLobby         time.Duration
	// IntervalleReprise : fréquence de recherche des parties orphelines à reprendre.
	IntervalleReprise time.Duration
	// AttenteParBot : faute d'adversaires dans la file, un bot rejoint la table à chaque palier ; 0 désactive.
	AttenteParBot time.Duration
}

// Charger lit la configuration ; getenv vaut os.Getenv en production.
func Charger(getenv func(string) string) (Config, error) {
	l := lecteur{getenv: getenv}
	hote, _ := os.Hostname()

	c := Config{
		Adresse:            l.texte("COINCHE_ADRESSE", ":8080"),
		AdresseAdmin:       l.texte("COINCHE_ADRESSE_ADMIN", ":9090"),
		PostgresURL:        l.texte("COINCHE_POSTGRES_URL", ""),
		RedisURL:           l.texte("COINCHE_REDIS_URL", ""),
		URLPublique:        strings.TrimRight(l.texte("COINCHE_URL_PUBLIQUE", "http://localhost:8080"), "/"),
		CookieSecure:       l.booleen("COINCHE_COOKIE_SECURE", true),
		CookieDomaine:      l.texte("COINCHE_COOKIE_DOMAINE", ""),
		DureeSession:       l.duree("COINCHE_DUREE_SESSION", 30*24*time.Hour),
		Google:             OAuth{l.texte("COINCHE_GOOGLE_CLIENT_ID", ""), l.texte("COINCHE_GOOGLE_CLIENT_SECRET", "")},
		Discord:            OAuth{l.texte("COINCHE_DISCORD_CLIENT_ID", ""), l.texte("COINCHE_DISCORD_CLIENT_SECRET", "")},
		AuthDev:            l.booleen("COINCHE_AUTH_DEV", false),
		Instance:           l.texte("COINCHE_INSTANCE", hote),
		LogNiveau:          l.niveau("COINCHE_LOG_NIVEAU", slog.LevelInfo),
		LogFormat:          l.texte("COINCHE_LOG_FORMAT", "json"),
		MigrerAuDemarrage:  l.booleen("COINCHE_MIGRER_AU_DEMARRAGE", true),
		ArretGracieux:      l.duree("COINCHE_ARRET_GRACIEUX", 20*time.Second),
		DebitWS:            l.entier("COINCHE_DEBIT_WS", 10),
		DelaiActionClassee: l.entier("COINCHE_DELAI_ACTION_CLASSEE", 30),
		DelaiBot:           l.duree("COINCHE_DELAI_BOT", 800*time.Millisecond),
		DelaiDemarrage:     l.duree("COINCHE_DELAI_DEMARRAGE", 3*time.Second),
		DelaiEntreMenes:    l.duree("COINCHE_DELAI_ENTRE_MENES", 3*time.Second),
		DelaiRetour:        l.duree("COINCHE_DELAI_RETOUR", 3*time.Minute),
		DelaiLobby:         l.duree("COINCHE_DELAI_LOBBY", 30*time.Minute),
		IntervalleReprise:  l.duree("COINCHE_INTERVALLE_REPRISE", 10*time.Second),
		AttenteParBot:      l.duree("COINCHE_ATTENTE_PAR_BOT", 5*time.Second),
	}
	c.URLFront = strings.TrimRight(l.texte("COINCHE_URL_FRONT", c.URLPublique), "/")
	c.Origines = l.liste("COINCHE_ORIGINES", []string{c.URLFront})
	if c.Instance == "" {
		c.Instance = "coinchenet"
	}

	if err := errors.Join(append(l.erreurs, c.valider()...)...); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c Config) valider() []error {
	var errs []error
	if c.PostgresURL == "" {
		errs = append(errs, errors.New("COINCHE_POSTGRES_URL est obligatoire"))
	}
	if c.RedisURL == "" {
		errs = append(errs, errors.New("COINCHE_REDIS_URL est obligatoire"))
	}
	for nom, u := range map[string]string{"COINCHE_URL_PUBLIQUE": c.URLPublique, "COINCHE_URL_FRONT": c.URLFront} {
		if p, err := url.Parse(u); err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Host == "" {
			errs = append(errs, fmt.Errorf("%s doit être une URL http(s) absolue", nom))
		}
	}
	for _, o := range c.Origines {
		if p, err := url.Parse(o); err != nil || p.Host == "" {
			errs = append(errs, fmt.Errorf("origine invalide %q dans COINCHE_ORIGINES", o))
		}
	}
	if !slices.Contains([]int{15, 30, 60}, c.DelaiActionClassee) {
		errs = append(errs, errors.New("COINCHE_DELAI_ACTION_CLASSEE doit valoir 15, 30 ou 60"))
	}
	if c.DebitWS < 1 {
		errs = append(errs, errors.New("COINCHE_DEBIT_WS doit être positif"))
	}
	if c.LogFormat != "json" && c.LogFormat != "texte" {
		errs = append(errs, errors.New("COINCHE_LOG_FORMAT doit valoir json ou texte"))
	}
	return errs
}

// HotesOrigines renvoie les hôtes (avec port) autorisés à ouvrir un WebSocket.
func (c Config) HotesOrigines() []string {
	var hotes []string
	for _, o := range c.Origines {
		if p, err := url.Parse(o); err == nil && p.Host != "" {
			hotes = append(hotes, p.Host)
		}
	}
	return hotes
}

type lecteur struct {
	getenv  func(string) string
	erreurs []error
}

func (l *lecteur) texte(nom, defaut string) string {
	if v := strings.TrimSpace(l.getenv(nom)); v != "" {
		return v
	}
	return defaut
}

func (l *lecteur) booleen(nom string, defaut bool) bool {
	v := l.texte(nom, "")
	if v == "" {
		return defaut
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		l.erreurs = append(l.erreurs, fmt.Errorf("%s : booléen attendu, %q reçu", nom, v))
		return defaut
	}
	return b
}

func (l *lecteur) entier(nom string, defaut int) int {
	v := l.texte(nom, "")
	if v == "" {
		return defaut
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		l.erreurs = append(l.erreurs, fmt.Errorf("%s : entier attendu, %q reçu", nom, v))
		return defaut
	}
	return n
}

func (l *lecteur) duree(nom string, defaut time.Duration) time.Duration {
	v := l.texte(nom, "")
	if v == "" {
		return defaut
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		l.erreurs = append(l.erreurs, fmt.Errorf("%s : durée attendue (ex. 30s, 5m), %q reçu", nom, v))
		return defaut
	}
	return d
}

func (l *lecteur) niveau(nom string, defaut slog.Level) slog.Level {
	v := l.texte(nom, "")
	if v == "" {
		return defaut
	}
	var n slog.Level
	if err := n.UnmarshalText([]byte(v)); err != nil {
		l.erreurs = append(l.erreurs, fmt.Errorf("%s : niveau debug, info, warn ou error attendu", nom))
		return defaut
	}
	return n
}

func (l *lecteur) liste(nom string, defaut []string) []string {
	v := l.texte(nom, "")
	if v == "" {
		return defaut
	}
	var res []string
	for e := range strings.SplitSeq(v, ",") {
		if e = strings.TrimSpace(e); e != "" {
			res = append(res, strings.TrimRight(e, "/"))
		}
	}
	return res
}
