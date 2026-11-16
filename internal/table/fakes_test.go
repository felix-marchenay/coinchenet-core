package table

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/felix-marchenay/coinchenet-core/internal/bot"
	"github.com/felix-marchenay/coinchenet-core/internal/classement"
	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
	"github.com/felix-marchenay/coinchenet-core/internal/protocole"
)

type journalMemoire struct {
	mu    sync.Mutex
	evts  map[uuid.UUID][]Evenement
	panne error
}

func nouveauJournal() *journalMemoire {
	return &journalMemoire{evts: map[uuid.UUID][]Evenement{}}
}

func (j *journalMemoire) Ajouter(_ context.Context, partie uuid.UUID, evts []Evenement) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.panne != nil {
		return j.panne
	}
	return j.ajouter(partie, evts)
}

func (j *journalMemoire) ajouter(partie uuid.UUID, evts []Evenement) error {
	for _, e := range evts {
		existants := j.evts[partie]
		if attendu := int64(len(existants) + 1); e.Sequence != attendu {
			return fmt.Errorf("séquence %d au lieu de %d", e.Sequence, attendu)
		}
		j.evts[partie] = append(existants, e)
	}
	return nil
}

func (j *journalMemoire) Charger(_ context.Context, partie uuid.UUID) ([]Evenement, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]Evenement(nil), j.evts[partie]...), nil
}

func (j *journalMemoire) types(partie uuid.UUID) []TypeEvenement {
	j.mu.Lock()
	defer j.mu.Unlock()
	var ts []TypeEvenement
	for _, e := range j.evts[partie] {
		ts = append(ts, e.Type)
	}
	return ts
}

func (j *journalMemoire) casser(err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.panne = err
}

type archivisteMemoire struct {
	mu       sync.Mutex
	journal  *journalMemoire
	debuts   []Debut
	fins     []Fin
	enCours  map[uuid.UUID]bool
	annulees []uuid.UUID

	echecCommencer error
}

func nouvelArchiviste(j *journalMemoire) *archivisteMemoire {
	return &archivisteMemoire{journal: j, enCours: map[uuid.UUID]bool{}}
}

func (a *archivisteMemoire) Commencer(_ context.Context, d Debut) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.echecCommencer != nil {
		return a.echecCommencer
	}
	a.debuts = append(a.debuts, d)
	a.enCours[d.PartieID] = true
	a.journal.mu.Lock()
	defer a.journal.mu.Unlock()
	return a.journal.ajouter(d.PartieID, []Evenement{d.Evenement})
}

func (a *archivisteMemoire) Terminer(_ context.Context, f Fin) ([]classement.Variation, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.enCours[f.PartieID] {
		return nil, nil
	}
	a.journal.mu.Lock()
	err := a.journal.ajouter(f.PartieID, []Evenement{f.Evenement})
	a.journal.mu.Unlock()
	if err != nil {
		return nil, err
	}
	a.enCours[f.PartieID] = false
	a.fins = append(a.fins, f)
	if f.Type != Classee {
		return nil, nil
	}
	var ps []classement.Participant
	for s, sf := range f.Sieges {
		ps = append(ps, classement.Participant{JoueurID: sf.Joueur.ID, Equipe: coinche.Siege(s).Equipe(), Elo: sf.Joueur.Elo, Abandon: sf.Abandon})
	}
	return classement.Calculer(ps, f.Gagnante), nil
}

func (a *archivisteMemoire) PartiesEnCours(context.Context) ([]uuid.UUID, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var ids []uuid.UUID
	for id, ok := range a.enCours {
		if ok {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (a *archivisteMemoire) Annuler(_ context.Context, id uuid.UUID) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.enCours[id] = false
	a.annulees = append(a.annulees, id)
	return nil
}

func (a *archivisteMemoire) derniereFin(t *testing.T) Fin {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	require.NotEmpty(t, a.fins, "la partie devrait être terminée")
	return a.fins[len(a.fins)-1]
}

type diffuseurMemoire struct {
	mu       sync.Mutex
	messages map[uuid.UUID][]protocole.Message
}

func nouveauDiffuseur() *diffuseurMemoire {
	return &diffuseurMemoire{messages: map[uuid.UUID][]protocole.Message{}}
}

func (d *diffuseurMemoire) Envoyer(_ context.Context, joueur uuid.UUID, m protocole.Message) {
	d.mu.Lock()
	defer d.mu.Unlock()
	// Passage par JSON, comme sur le fil : les tests lisent ce que verrait un client.
	b, err := json.Marshal(m.Data)
	if err != nil {
		panic(err)
	}
	m.Data = json.RawMessage(b)
	d.messages[joueur] = append(d.messages[joueur], m)
}

func (d *diffuseurMemoire) de(joueur uuid.UUID, typ string) []json.RawMessage {
	d.mu.Lock()
	defer d.mu.Unlock()
	var res []json.RawMessage
	for _, m := range d.messages[joueur] {
		if m.Type == typ {
			res = append(res, m.Data.(json.RawMessage))
		}
	}
	return res
}

func (d *diffuseurMemoire) dernier(t *testing.T, joueur uuid.UUID, typ string, v any) bool {
	t.Helper()
	ms := d.de(joueur, typ)
	if len(ms) == 0 {
		return false
	}
	require.NoError(t, json.Unmarshal(ms[len(ms)-1], v))
	return true
}

func (d *diffuseurMemoire) codesErreur(joueur uuid.UUID) []string {
	var codes []string
	for _, m := range d.de(joueur, protocole.TypeErreur) {
		var e protocole.Erreur
		_ = json.Unmarshal(m, &e)
		codes = append(codes, e.Code)
	}
	return codes
}

func (d *diffuseurMemoire) vider(joueur uuid.UUID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.messages, joueur)
}

type annuaireMemoire struct {
	mu     sync.Mutex
	tables map[uuid.UUID]uuid.UUID
}

func nouvelAnnuaire() *annuaireMemoire {
	return &annuaireMemoire{tables: map[uuid.UUID]uuid.UUID{}}
}

func (a *annuaireMemoire) Associer(_ context.Context, joueur, table uuid.UUID) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tables[joueur] = table
	return nil
}

func (a *annuaireMemoire) Dissocier(_ context.Context, joueur, table uuid.UUID) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.tables[joueur] == table {
		delete(a.tables, joueur)
	}
	return nil
}

func (a *annuaireMemoire) table(joueur uuid.UUID) (uuid.UUID, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	t, ok := a.tables[joueur]
	return t, ok
}

type environnement struct {
	journal    *journalMemoire
	archiviste *archivisteMemoire
	diffuseur  *diffuseurMemoire
	annuaire   *annuaireMemoire
	deps       Dependances
	cfg        Config
}

func nouvelEnvironnement() *environnement {
	j := nouveauJournal()
	env := &environnement{
		journal:    j,
		archiviste: nouvelArchiviste(j),
		diffuseur:  nouveauDiffuseur(),
		annuaire:   nouvelAnnuaire(),
		cfg:        ConfigParDefaut(),
	}
	hasard := coinche.NouveauHasardDeterministe(11)
	env.deps = Dependances{
		Journal:    env.journal,
		Archiviste: env.archiviste,
		Diffuseur:  env.diffuseur,
		Annuaire:   env.annuaire,
		Decideur:   bot.Bots{},
		Melangeur:  coinche.FischerYatesMelangeur{Hasard: hasard},
		Coupeur:    coinche.RandomCoupeur{Hasard: hasard},
		Hasard:     hasard,
	}
	return env
}

func joueur(pseudo string, elo int) Joueur {
	return Joueur{ID: uuid.New(), Pseudo: pseudo, Elo: elo}
}

func commande(typ TypeCommande, j Joueur, connexion string, donnees any) Commande {
	c := Commande{Type: typ, Joueur: j, Connexion: connexion}
	if donnees != nil {
		b, err := json.Marshal(donnees)
		if err != nil {
			panic(err)
		}
		c.Donnees = b
	}
	return c
}

// lancerActeur démarre l'acteur dans la bulle synctest courante ; la fonction renvoyée
// l'arrête et attend sa fin (elle est aussi appelée en fin de test).
func lancerActeur(t *testing.T, tb *Table) func() {
	t.Helper()
	ctx, annuler := context.WithCancel(context.Background())
	go tb.Executer(ctx)
	arreter := func() {
		annuler()
		<-tb.Arretee()
	}
	t.Cleanup(arreter)
	return arreter
}

// repondreAuTour fait jouer un humain : il passe aux enchères et joue sa première carte jouable.
// Renvoie faux si ce n'est pas à lui de jouer.
func repondreAuTour(t *testing.T, env *environnement, tb *Table, j Joueur, siege coinche.Siege, deja *int) bool {
	t.Helper()
	tours := env.diffuseur.de(j.ID, MsgTour)
	if len(tours) <= *deja {
		return false
	}
	*deja = len(tours)
	var tour MessageTour
	require.NoError(t, json.Unmarshal(tours[len(tours)-1], &tour))
	if tour.Siege != siege {
		return false
	}
	if tour.Phase == coinche.PhaseEncheres {
		tb.Envoyer(commande(CmdEnchere, j, "c-"+j.Pseudo, DonneesEnchere{Action: coinche.Passe}))
	} else {
		require.NotEmpty(t, tour.CartesJouables)
		tb.Envoyer(commande(CmdJouer, j, "c-"+j.Pseudo, DonneesJouer{Carte: tour.CartesJouables[0]}))
	}
	return true
}

var errPanne = errors.New("base de données indisponible")

// patienter fait avancer l'horloge virtuelle.
func patienter(d time.Duration) {
	time.Sleep(d)
}
