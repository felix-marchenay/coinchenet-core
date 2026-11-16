package table

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
)

// BusTables achemine les commandes vers l'instance qui possède la table.
type BusTables interface {
	PublierTable(ctx context.Context, table uuid.UUID, c Commande) error
	AbonnerTable(ctx context.Context, table uuid.UUID, recevoir func(Commande)) (desabonner func(), err error)
}

// Verrous garantit qu'une table n'est exécutée que par une seule instance.
type Verrous interface {
	Prendre(ctx context.Context, cle, proprietaire string, ttl time.Duration) (bool, error)
	Prolonger(ctx context.Context, cle, proprietaire string, ttl time.Duration) (bool, error)
	Liberer(ctx context.Context, cle, proprietaire string) error
	Proprietaire(ctx context.Context, cle string) (string, bool, error)
}

// Codes associe les codes d'invitation aux tables privées.
type Codes interface {
	Reserver(ctx context.Context, code string, table uuid.UUID, ttl time.Duration) (bool, error)
	Resoudre(ctx context.Context, code string) (uuid.UUID, bool, error)
	Liberer(ctx context.Context, code string, table uuid.UUID) error
}

// Reprise liste les parties en cours d'après le stockage durable, et écarte celles
// dont le journal ne permet pas la reconstruction.
type Reprise interface {
	PartiesEnCours(ctx context.Context) ([]uuid.UUID, error)
	Annuler(ctx context.Context, partie uuid.UUID) error
}

type ConfigRegistre struct {
	Instance          string
	TTLVerrou         time.Duration
	IntervalleReprise time.Duration
	OptionsClassees   Options
	Table             Config
}

type entree struct {
	table      *Table
	annuler    context.CancelFunc
	desabonner func()
	perdue     bool
	// prolongee : dernière prolongation réussie du verrou.
	prolongee time.Time
}

// Registre gère les tables exécutées par cette instance : création, routage des commandes,
// maintien des verrous et reprise des tables orphelines.
type Registre struct {
	cfg     ConfigRegistre
	deps    Dependances
	bus     BusTables
	verrous Verrous
	codes   Codes
	reprise Reprise
	log     *slog.Logger

	mu          sync.Mutex
	tables      map[uuid.UUID]*entree
	racine      context.Context
	wg          sync.WaitGroup
	maintenance context.CancelFunc
	arrete      bool
}

var (
	ErrTableIntrouvable = errors.New("table introuvable")
	ErrRegistreArrete   = errors.New("instance en cours d'arrêt")
)

func NouveauRegistre(cfg ConfigRegistre, deps Dependances, bus BusTables, verrous Verrous, codes Codes, reprise Reprise) *Registre {
	deps = deps.completer()
	if cfg.TTLVerrou == 0 {
		cfg.TTLVerrou = 30 * time.Second
	}
	if cfg.IntervalleReprise == 0 {
		cfg.IntervalleReprise = 10 * time.Second
	}
	return &Registre{
		cfg:     cfg,
		deps:    deps,
		bus:     bus,
		verrous: verrous,
		codes:   codes,
		reprise: reprise,
		log:     deps.Logger.With("composant", "registre"),
		tables:  map[uuid.UUID]*entree{},
		racine:  context.Background(),
	}
}

func cleVerrou(table uuid.UUID) string {
	return "table:" + table.String()
}

// Demarrer lance la maintenance des verrous et la reprise des tables orphelines.
func (r *Registre) Demarrer(ctx context.Context) {
	r.racine = context.WithoutCancel(ctx)
	ctx, r.maintenance = context.WithCancel(ctx)
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ticker := time.NewTicker(min(r.cfg.TTLVerrou/3, r.cfg.IntervalleReprise))
		defer ticker.Stop()
		r.reprendreOrphelines(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.prolongerVerrous(ctx)
				r.reprendreOrphelines(ctx)
			}
		}
	}()
}

// Arreter stoppe toutes les tables locales et libère leurs verrous pour qu'une autre instance les reprenne.
func (r *Registre) Arreter(ctx context.Context) {
	if r.maintenance != nil {
		r.maintenance()
	}
	r.mu.Lock()
	r.arrete = true
	for _, e := range r.tables {
		e.annuler()
	}
	r.mu.Unlock()

	fini := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(fini)
	}()
	select {
	case <-fini:
	case <-ctx.Done():
		r.log.Warn("arrêt des tables interrompu", "erreur", ctx.Err())
	}
}

// CreerClassee lance une partie classée et attend qu'elle soit enregistrée :
// la file a déjà retiré les joueurs, un échec doit lui être signalé.
func (r *Registre) CreerClassee(ctx context.Context, equipeA, equipeB [2]Joueur) (uuid.UUID, error) {
	id := uuid.New()
	t, err := NouvelleTableClassee(id, equipeA, equipeB, r.cfg.OptionsClassees, r.deps, r.cfg.Table)
	if err != nil {
		return uuid.Nil, err
	}
	if err := r.lancer(ctx, t, true); err != nil {
		return uuid.Nil, err
	}
	select {
	case err := <-t.demarree:
		if err != nil {
			return uuid.Nil, err
		}
	case <-ctx.Done():
		return uuid.Nil, ctx.Err()
	}
	return id, nil
}

func (r *Registre) CreerPrivee(ctx context.Context, createur Joueur, options Options) (uuid.UUID, string, error) {
	if err := options.Valider(); err != nil {
		return uuid.Nil, "", err
	}
	id := uuid.New()
	code, err := r.reserverCode(ctx, id)
	if err != nil {
		return uuid.Nil, "", err
	}
	t, err := NouvelleTablePrivee(id, createur, code, options, r.deps, r.cfg.Table)
	if err != nil {
		return uuid.Nil, "", err
	}
	if err := r.lancer(ctx, t, true); err != nil {
		_ = r.codes.Liberer(ctx, code, id)
		return uuid.Nil, "", err
	}
	return id, code, nil
}

func (r *Registre) ResoudreCode(ctx context.Context, code string) (uuid.UUID, bool, error) {
	return r.codes.Resoudre(ctx, code)
}

// Envoyer achemine une commande : directement si la table est locale, sinon par le bus.
func (r *Registre) Envoyer(ctx context.Context, table uuid.UUID, c Commande) error {
	r.mu.Lock()
	e, locale := r.tables[table]
	r.mu.Unlock()
	if locale && e.table.Envoyer(c) {
		return nil
	}
	return r.bus.PublierTable(ctx, table, c)
}

// Vivante indique si la table est exécutée par une instance : sinon, une association
// joueur → table qui la désigne est périmée.
func (r *Registre) Vivante(ctx context.Context, table uuid.UUID) (bool, error) {
	r.mu.Lock()
	_, locale := r.tables[table]
	r.mu.Unlock()
	if locale {
		return true, nil
	}
	_, tenu, err := r.verrous.Proprietaire(ctx, cleVerrou(table))
	return tenu, err
}

// NbTables compte les tables locales actives par type.
func (r *Registre) NbTables() map[Type]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := map[Type]int{Classee: 0, Privee: 0}
	for _, e := range r.tables {
		n[e.table.typ]++
	}
	return n
}

func (r *Registre) reserverCode(ctx context.Context, table uuid.UUID) (string, error) {
	ttl := r.cfg.Table.DelaiLobby + 10*time.Minute
	for range 10 {
		code := GenererCode(r.deps.Hasard)
		ok, err := r.codes.Reserver(ctx, code, table, ttl)
		if err != nil {
			return "", fmt.Errorf("réservation du code : %w", err)
		}
		if ok {
			return code, nil
		}
	}
	return "", errors.New("impossible de générer un code de table unique")
}

// lancer démarre l'acteur après avoir pris (ou en tenant déjà) le verrou de la table.
func (r *Registre) lancer(ctx context.Context, t *Table, prendreVerrou bool) error {
	if prendreVerrou {
		ok, err := r.verrous.Prendre(ctx, cleVerrou(t.id), r.cfg.Instance, r.cfg.TTLVerrou)
		if err != nil {
			return fmt.Errorf("verrou de la table : %w", err)
		}
		if !ok {
			return fmt.Errorf("la table %s est déjà exécutée ailleurs", t.id)
		}
	}

	desabonner, err := r.bus.AbonnerTable(ctx, t.id, func(c Commande) {
		if !t.Envoyer(c) {
			r.log.Warn("commande perdue", "table", t.id, "commande", c.Type)
		}
	})
	if err != nil {
		_ = r.verrous.Liberer(ctx, cleVerrou(t.id), r.cfg.Instance)
		return fmt.Errorf("abonnement aux commandes de la table : %w", err)
	}

	ctxActeur, annuler := context.WithCancel(r.racine)
	e := &entree{table: t, annuler: annuler, desabonner: desabonner, prolongee: time.Now()}
	r.mu.Lock()
	if r.arrete {
		r.mu.Unlock()
		annuler()
		desabonner()
		_ = r.verrous.Liberer(ctx, cleVerrou(t.id), r.cfg.Instance)
		return ErrRegistreArrete
	}
	r.tables[t.id] = e
	r.wg.Add(1)
	r.mu.Unlock()

	go func() {
		defer r.wg.Done()
		defer annuler()
		t.Executer(ctxActeur)
		r.nettoyer(e)
	}()
	return nil
}

func (r *Registre) nettoyer(e *entree) {
	ctx, annuler := context.WithTimeout(context.Background(), 5*time.Second)
	defer annuler()

	e.desabonner()
	r.mu.Lock()
	delete(r.tables, e.table.id)
	perdue := e.perdue
	r.mu.Unlock()

	if e.table.code != "" {
		if err := r.codes.Liberer(ctx, e.table.code, e.table.id); err != nil {
			r.log.Warn("libération du code impossible", "erreur", err)
		}
	}
	if !perdue {
		if err := r.verrous.Liberer(ctx, cleVerrou(e.table.id), r.cfg.Instance); err != nil {
			r.log.Warn("libération du verrou impossible", "table", e.table.id, "erreur", err)
		}
	}
}

func (r *Registre) prolongerVerrous(ctx context.Context) {
	r.mu.Lock()
	entrees := make([]*entree, 0, len(r.tables))
	for _, e := range r.tables {
		entrees = append(entrees, e)
	}
	r.mu.Unlock()

	for _, e := range entrees {
		ok, err := r.verrous.Prolonger(ctx, cleVerrou(e.table.id), r.cfg.Instance, r.cfg.TTLVerrou)
		r.mu.Lock()
		switch {
		case err != nil:
			r.log.Warn("prolongation du verrou impossible", "table", e.table.id, "erreur", err)
			// Sans prolongation, le verrou expire et la table peut être reprise ailleurs :
			// on s'arrête avant pour ne jamais l'exécuter à deux.
			ok = time.Since(e.prolongee) < r.cfg.TTLVerrou*2/3
		case ok:
			e.prolongee = time.Now()
		}
		if !ok {
			e.perdue = true
		}
		r.mu.Unlock()
		if !ok {
			r.log.Error("verrou perdu, la table est abandonnée à une autre instance", "table", e.table.id)
			e.annuler()
		}
	}
}

// reprendreOrphelines restaure les parties en cours qu'aucune instance n'exécute plus.
func (r *Registre) reprendreOrphelines(ctx context.Context) {
	ids, err := r.reprise.PartiesEnCours(ctx)
	if err != nil {
		r.log.Warn("liste des parties en cours indisponible", "erreur", err)
		return
	}
	for _, id := range ids {
		r.mu.Lock()
		_, locale := r.tables[id]
		arrete := r.arrete
		r.mu.Unlock()
		if locale || arrete {
			continue
		}
		if err := r.restaurer(ctx, id); err != nil {
			r.log.Error("reprise de la table impossible", "table", id, "erreur", err)
		}
	}
}

func (r *Registre) restaurer(ctx context.Context, id uuid.UUID) error {
	ok, err := r.verrous.Prendre(ctx, cleVerrou(id), r.cfg.Instance, r.cfg.TTLVerrou)
	if err != nil || !ok {
		return err
	}
	evts, err := r.deps.Journal.Charger(ctx, id)
	if err == nil {
		var t *Table
		if t, err = Restaurer(id, evts, r.deps, r.cfg.Table); err == nil {
			return r.lancer(ctx, t, false)
		}
	}
	if _, incoherent := errors.AsType[*ErrJournalIncoherent](err); incoherent {
		r.log.Error("journal irrécupérable, la partie est annulée", "table", id, "erreur", err)
		if errAnnul := r.reprise.Annuler(ctx, id); errAnnul != nil {
			r.log.Error("annulation impossible", "table", id, "erreur", errAnnul)
		}
	}
	_ = r.verrous.Liberer(ctx, cleVerrou(id), r.cfg.Instance)
	return err
}

const alphabetCode = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// GenererCode produit un code d'invitation lisible, sans caractères ambigus.
func GenererCode(h coinche.Hasard) string {
	b := make([]byte, 6)
	for i := range b {
		b[i] = alphabetCode[h.Intn(len(alphabetCode))]
	}
	return string(b)
}
