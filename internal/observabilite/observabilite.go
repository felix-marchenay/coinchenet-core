// Package observabilite regroupe les journaux structurés et les métriques Prometheus.
package observabilite

import (
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/felix-marchenay/coinchenet-core/internal/table"
)

func NouveauLogger(w io.Writer, niveau slog.Level, format string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: niveau}
	if format == "texte" {
		return slog.New(slog.NewTextHandler(w, opts))
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}

// Metriques implémente les interfaces de métriques de la table, du matchmaking et de l'API.
type Metriques struct {
	registre   *prometheus.Registry
	requetes   *prometheus.HistogramVec
	connexions prometheus.Gauge
	parties    *prometheus.CounterVec
	actions    *prometheus.HistogramVec
	file       prometheus.Gauge
	attente    prometheus.Histogram
}

// NouvellesMetriques ; tables fournit le nombre de tables actives de l'instance, par type.
func NouvellesMetriques(tables func() map[table.Type]int) *Metriques {
	r := prometheus.NewRegistry()
	m := &Metriques{
		registre: r,
		requetes: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "coinche_http_requetes_secondes",
			Help:    "Durée des requêtes HTTP.",
			Buckets: prometheus.DefBuckets,
		}, []string{"methode", "route", "statut"}),
		connexions: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "coinche_websocket_connexions",
			Help: "Connexions WebSocket ouvertes sur cette instance.",
		}),
		parties: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "coinche_parties_terminees_total",
			Help: "Parties terminées, par type et par cause.",
		}, []string{"type", "cause"}),
		actions: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "coinche_table_commande_secondes",
			Help:    "Temps de traitement d'une commande par l'acteur de table.",
			Buckets: []float64{.0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5, 1},
		}, []string{"commande"}),
		file: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "coinche_file_joueurs",
			Help: "Joueurs en attente dans la file classée (mesuré par le leader).",
		}),
		attente: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "coinche_file_attente_secondes",
			Help:    "Temps d'attente en file avant de trouver une table.",
			Buckets: []float64{1, 5, 10, 20, 30, 45, 60, 90, 120, 180, 300, 600},
		}),
	}
	r.MustRegister(m.requetes, m.connexions, m.parties, m.actions, m.file, m.attente,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	for _, typ := range []table.Type{table.Classee, table.Privee} {
		r.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name:        "coinche_tables_actives",
			Help:        "Tables exécutées par cette instance.",
			ConstLabels: prometheus.Labels{"type": string(typ)},
		}, func() float64 {
			if tables == nil {
				return 0
			}
			return float64(tables()[typ])
		}))
	}
	return m
}

func (m *Metriques) Handler() http.Handler {
	return promhttp.HandlerFor(m.registre, promhttp.HandlerOpts{Registry: m.registre})
}

func (m *Metriques) PartieTerminee(t table.Type, c table.Cause) {
	m.parties.WithLabelValues(string(t), string(c)).Inc()
}

func (m *Metriques) ActionTraitee(commande string, duree time.Duration) {
	m.actions.WithLabelValues(commande).Observe(duree.Seconds())
}

func (m *Metriques) JoueursEnFile(n int) {
	m.file.Set(float64(n))
}

func (m *Metriques) Apparie(attente time.Duration) {
	m.attente.Observe(attente.Seconds())
}

func (m *Metriques) ConnexionOuverte() {
	m.connexions.Inc()
}

func (m *Metriques) ConnexionFermee() {
	m.connexions.Dec()
}

func (m *Metriques) Requete(methode, route string, statut int, duree time.Duration) {
	if route == "" {
		route = "inconnue"
	}
	m.requetes.WithLabelValues(methode, route, strconv.Itoa(statut)).Observe(duree.Seconds())
}
