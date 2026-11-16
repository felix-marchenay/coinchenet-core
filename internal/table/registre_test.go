package table

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type busMemoire struct {
	mu      sync.Mutex
	abonnes map[uuid.UUID]func(Commande)
	publies int
}

func (b *busMemoire) PublierTable(_ context.Context, t uuid.UUID, c Commande) error {
	b.mu.Lock()
	recevoir := b.abonnes[t]
	b.publies++
	b.mu.Unlock()
	if recevoir != nil {
		recevoir(c)
	}
	return nil
}

func (b *busMemoire) AbonnerTable(_ context.Context, t uuid.UUID, recevoir func(Commande)) (func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.abonnes[t] = recevoir
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.abonnes, t)
	}, nil
}

type verrousMemoire struct {
	mu   sync.Mutex
	tenu map[string]string
}

func (v *verrousMemoire) Prendre(_ context.Context, cle, proprietaire string, _ time.Duration) (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if p, ok := v.tenu[cle]; ok && p != proprietaire {
		return false, nil
	}
	v.tenu[cle] = proprietaire
	return true, nil
}

func (v *verrousMemoire) Prolonger(_ context.Context, cle, proprietaire string, _ time.Duration) (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.tenu[cle] == proprietaire, nil
}

func (v *verrousMemoire) Liberer(_ context.Context, cle, proprietaire string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.tenu[cle] == proprietaire {
		delete(v.tenu, cle)
	}
	return nil
}

func (v *verrousMemoire) Proprietaire(_ context.Context, cle string) (string, bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	p, ok := v.tenu[cle]
	return p, ok, nil
}

func (v *verrousMemoire) proprietaire(cle string) string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.tenu[cle]
}

func (v *verrousMemoire) voler(cle, voleur string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.tenu[cle] = voleur
}

type codesMemoire struct {
	mu    sync.Mutex
	codes map[string]uuid.UUID
}

func (c *codesMemoire) Reserver(_ context.Context, code string, t uuid.UUID, _ time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, pris := c.codes[code]; pris {
		return false, nil
	}
	c.codes[code] = t
	return true, nil
}

func (c *codesMemoire) Resoudre(_ context.Context, code string) (uuid.UUID, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.codes[code]
	return t, ok, nil
}

func (c *codesMemoire) Liberer(_ context.Context, code string, t uuid.UUID) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.codes[code] == t {
		delete(c.codes, code)
	}
	return nil
}

type registreDeTest struct {
	*Registre
	env     *environnement
	bus     *busMemoire
	verrous *verrousMemoire
	codes   *codesMemoire
}

func nouveauRegistreDeTest(t *testing.T, instance string, env *environnement, verrous *verrousMemoire) *registreDeTest {
	t.Helper()
	r := &registreDeTest{
		env:     env,
		bus:     &busMemoire{abonnes: map[uuid.UUID]func(Commande){}},
		verrous: verrous,
		codes:   &codesMemoire{codes: map[string]uuid.UUID{}},
	}
	r.Registre = NouveauRegistre(ConfigRegistre{
		Instance:          instance,
		TTLVerrou:         30 * time.Second,
		IntervalleReprise: time.Second,
		OptionsClassees:   OptionsClassees(),
		Table:             env.cfg,
	}, env.deps, r.bus, verrous, r.codes, env.archiviste)
	ctx, annuler := context.WithCancel(context.Background())
	r.Demarrer(ctx)
	t.Cleanup(func() {
		annuler()
		r.Arreter(context.Background())
	})
	return r
}

func TestRegistreTablePrivee(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := nouvelEnvironnement()
		verrous := &verrousMemoire{tenu: map[string]string{}}
		r := nouveauRegistreDeTest(t, "i1", env, verrous)
		alice := joueur("Alice", 1200)

		_, _, err := r.CreerPrivee(context.Background(), alice, Options{DelaiAction: 7})
		require.Error(t, err)

		id, code, err := r.CreerPrivee(context.Background(), alice, optionsRapides())
		require.NoError(t, err)
		assert.Len(t, code, 6)
		assert.Equal(t, "i1", verrous.proprietaire(cleVerrou(id)))
		assert.Equal(t, map[Type]int{Classee: 0, Privee: 1}, r.NbTables())

		resolu, ok, err := r.ResoudreCode(context.Background(), code)
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, id, resolu)

		require.NoError(t, r.Envoyer(context.Background(), id, commande(CmdConnexion, alice, "a", nil)))
		synctest.Wait()
		assert.NotEmpty(t, env.diffuseur.de(alice.ID, MsgEtat))
		assert.Zero(t, r.bus.publies, "une table locale reçoit ses commandes directement")

		require.NoError(t, r.Envoyer(context.Background(), id, commande(CmdQuitter, alice, "a", nil)))
		synctest.Wait()
		assert.Equal(t, map[Type]int{Classee: 0, Privee: 0}, r.NbTables(), "la table fermée est retirée")
		assert.Empty(t, verrous.proprietaire(cleVerrou(id)), "et son verrou libéré")
		_, ok, _ = r.ResoudreCode(context.Background(), code)
		assert.False(t, ok, "et son code aussi")
	})
}

func TestRegistreLancementClasseEnEchec(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := nouvelEnvironnement()
		env.archiviste.echecCommencer = errPanne
		verrous := &verrousMemoire{tenu: map[string]string{}}
		r := nouveauRegistreDeTest(t, "i1", env, verrous)

		_, err := r.CreerClassee(context.Background(), [2]Joueur{joueur("A1", 1500), joueur("A2", 1500)}, [2]Joueur{joueur("B1", 1500), joueur("B2", 1500)})
		require.Error(t, err, "la file doit savoir que la table n'a pas pu démarrer")
		synctest.Wait()
		assert.Zero(t, r.NbTables()[Classee])
	})
}

func TestRegistreVivante(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := nouvelEnvironnement()
		verrous := &verrousMemoire{tenu: map[string]string{}}
		r := nouveauRegistreDeTest(t, "i1", env, verrous)
		id, _, err := r.CreerPrivee(context.Background(), joueur("Alice", 1200), optionsRapides())
		require.NoError(t, err)

		vivante, err := r.Vivante(context.Background(), id)
		require.NoError(t, err)
		assert.True(t, vivante)

		ailleurs := uuid.New()
		verrous.voler(cleVerrou(ailleurs), "i2")
		vivante, err = r.Vivante(context.Background(), ailleurs)
		require.NoError(t, err)
		assert.True(t, vivante, "exécutée par une autre instance")

		vivante, err = r.Vivante(context.Background(), uuid.New())
		require.NoError(t, err)
		assert.False(t, vivante)
	})
}

func TestRegistreRepriseDUneTableOrpheline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := nouvelEnvironnement()
		verrous := &verrousMemoire{tenu: map[string]string{}}
		r1 := nouveauRegistreDeTest(t, "i1", env, verrous)
		a1, b1, a2, b2 := joueur("A1", 1500), joueur("B1", 1500), joueur("A2", 1500), joueur("B2", 1500)

		id, err := r1.CreerClassee(context.Background(), [2]Joueur{a1, a2}, [2]Joueur{b1, b2})
		require.NoError(t, err)
		time.Sleep(time.Minute)
		synctest.Wait()

		// Arrêt de la première instance : elle rend la table.
		r1.Arreter(context.Background())
		synctest.Wait()
		assert.Empty(t, verrous.proprietaire(cleVerrou(id)))

		r2 := nouveauRegistreDeTest(t, "i2", env, verrous)
		time.Sleep(2 * time.Second)
		synctest.Wait()
		assert.Equal(t, "i2", verrous.proprietaire(cleVerrou(id)), "une autre instance reprend la table")
		assert.Equal(t, 1, r2.NbTables()[Classee])
		assert.NotEmpty(t, env.diffuseur.de(a1.ID, MsgCommencee))
	})
}

func TestRegistreVerrouPerdu(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := nouvelEnvironnement()
		verrous := &verrousMemoire{tenu: map[string]string{}}
		r := nouveauRegistreDeTest(t, "i1", env, verrous)
		id, err := r.CreerClassee(context.Background(), [2]Joueur{joueur("A1", 1500), joueur("A2", 1500)}, [2]Joueur{joueur("B1", 1500), joueur("B2", 1500)})
		require.NoError(t, err)

		verrous.voler(cleVerrou(id), "autre")
		time.Sleep(15 * time.Second)
		synctest.Wait()

		assert.Zero(t, r.NbTables()[Classee], "une table dont le verrou est perdu s'arrête")
		assert.Equal(t, "autre", verrous.proprietaire(cleVerrou(id)), "sans libérer le verrou d'un autre")
	})
}

func TestRegistreJournalIrrecuperable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := nouvelEnvironnement()
		id := uuid.New()
		env.archiviste.enCours[id] = true
		verrous := &verrousMemoire{tenu: map[string]string{}}

		nouveauRegistreDeTest(t, "i1", env, verrous)
		time.Sleep(2 * time.Second)
		synctest.Wait()

		assert.Contains(t, env.archiviste.annulees, id, "une partie sans journal exploitable est annulée")
		assert.Empty(t, verrous.proprietaire(cleVerrou(id)))
	})
}
