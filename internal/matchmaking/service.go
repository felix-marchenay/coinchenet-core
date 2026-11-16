package matchmaking

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/felix-marchenay/coinchenet-core/internal/table"
)

// Source est la file des commandes, partagée entre les instances et consommée par le leader.
type Source interface {
	Publier(ctx context.Context, c Commande) error
	Recevoir(ctx context.Context, attente time.Duration) (Commande, bool, error)
}

// Stockage conserve l'état de la file pour qu'un nouveau leader le reprenne.
type Stockage interface {
	Charger(ctx context.Context) (Etat, error)
	Sauver(ctx context.Context, e Etat) error
}

type CreateurTables interface {
	CreerClassee(ctx context.Context, equipeA, equipeB [2]table.Joueur) (uuid.UUID, error)
}

// Metriques est optionnel.
type Metriques interface {
	JoueursEnFile(n int)
	Apparie(attente time.Duration)
}

type Config struct {
	Instance   string
	TTLLeader  time.Duration
	Intervalle time.Duration
	Parametres Parametres
}

const cleLeader = "matchmaking:leader"

// Service fait tourner la file classée : une seule instance (le leader) applique les commandes.
type Service struct {
	cfg        Config
	source     Source
	stockage   Stockage
	verrous    table.Verrous
	tables     CreateurTables
	diffuseur  table.Diffuseur
	metriques  Metriques
	log        *slog.Logger
	maintenant func() time.Time
}

func NouveauService(cfg Config, source Source, stockage Stockage, verrous table.Verrous, tables CreateurTables, diffuseur table.Diffuseur, metriques Metriques, log *slog.Logger) *Service {
	if cfg.TTLLeader == 0 {
		cfg.TTLLeader = 15 * time.Second
	}
	if cfg.Intervalle == 0 {
		cfg.Intervalle = time.Second
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Service{
		cfg:        cfg,
		source:     source,
		stockage:   stockage,
		verrous:    verrous,
		tables:     tables,
		diffuseur:  diffuseur,
		metriques:  metriques,
		log:        log.With("composant", "matchmaking"),
		maintenant: time.Now,
	}
}

// Envoyer transmet une commande au leader, quelle que soit l'instance qui la reçoit.
func (s *Service) Envoyer(ctx context.Context, c Commande) error {
	return s.source.Publier(ctx, c)
}

// Executer candidate au rôle de leader jusqu'à l'annulation du contexte.
func (s *Service) Executer(ctx context.Context) {
	for {
		ok, err := s.verrous.Prendre(ctx, cleLeader, s.cfg.Instance, s.cfg.TTLLeader)
		if err != nil {
			s.log.Warn("élection du leader impossible", "erreur", err)
		}
		if ok {
			s.diriger(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(s.cfg.TTLLeader / 3):
		}
	}
}

func (s *Service) diriger(ctx context.Context) {
	defer func() {
		liberation, annuler := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer annuler()
		_ = s.verrous.Liberer(liberation, cleLeader, s.cfg.Instance)
	}()

	etat, err := s.stockage.Charger(ctx)
	if err != nil {
		s.log.Error("chargement de la file impossible", "erreur", err)
		return
	}
	m := NouveauMoteur(etat, s.cfg.Parametres)
	s.log.Info("cette instance dirige la file classée", "joueurs_en_file", m.NbEnFile())

	tick := time.NewTicker(s.cfg.Intervalle)
	defer tick.Stop()
	prolonger := time.NewTicker(s.cfg.TTLLeader / 3)
	defer prolonger.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-prolonger.C:
			ok, err := s.verrous.Prolonger(ctx, cleLeader, s.cfg.Instance, s.cfg.TTLLeader)
			if err != nil || !ok {
				s.log.Warn("direction de la file perdue", "erreur", err)
				return
			}
			continue
		case <-tick.C:
			s.executerEffets(ctx, m, m.Tick(s.maintenant()))
			continue
		default:
		}

		c, recue, err := s.source.Recevoir(ctx, s.cfg.Intervalle/2)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.log.Warn("lecture des commandes de la file impossible", "erreur", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(s.cfg.Intervalle):
			}
			continue
		}
		if recue {
			s.executerEffets(ctx, m, m.Appliquer(c, s.maintenant()))
		}
	}
}

func (s *Service) executerEffets(ctx context.Context, m *Moteur, e Effets) {
	for _, c := range e.Tables {
		id, err := s.tables.CreerClassee(ctx, c.EquipeA, c.EquipeB)
		if err != nil {
			s.log.Error("création de la table classée impossible", "erreur", err)
			e.Notifications = append(e.Notifications, m.TableEchouee(c).Notifications...)
			continue
		}
		s.log.Info("table classée formée", "table", id, "proposition", c.Proposition)
		if s.metriques != nil {
			for _, a := range c.Attentes {
				s.metriques.Apparie(a)
			}
		}
	}
	for _, n := range e.Notifications {
		s.diffuseur.Envoyer(ctx, n.Joueur, n.Message)
	}
	if e.Modifie || len(e.Tables) > 0 {
		if err := s.stockage.Sauver(ctx, m.Etat); err != nil {
			s.log.Warn("sauvegarde de la file impossible", "erreur", err)
		}
		if s.metriques != nil {
			s.metriques.JoueursEnFile(m.NbEnFile())
		}
	}
}
