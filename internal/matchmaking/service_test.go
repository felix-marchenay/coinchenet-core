package matchmaking

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/felix-marchenay/coinchenet-core/internal/protocole"
	"github.com/felix-marchenay/coinchenet-core/internal/table"
)

type sourceMemoire struct {
	c chan Commande
}

func (s *sourceMemoire) Publier(_ context.Context, c Commande) error {
	s.c <- c
	return nil
}

func (s *sourceMemoire) Recevoir(ctx context.Context, attente time.Duration) (Commande, bool, error) {
	select {
	case c := <-s.c:
		return c, true, nil
	case <-time.After(attente):
		return Commande{}, false, nil
	case <-ctx.Done():
		return Commande{}, false, ctx.Err()
	}
}

type stockageMemoire struct {
	mu   sync.Mutex
	etat Etat
}

func (s *stockageMemoire) Charger(context.Context) (Etat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.etat, nil
}

func (s *stockageMemoire) Sauver(_ context.Context, e Etat) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	var copie Etat
	if err := json.Unmarshal(b, &copie); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.etat = copie
	return nil
}

func (s *stockageMemoire) lire() Etat {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.etat
}

type verrouMemoire struct {
	mu           sync.Mutex
	proprietaire string
}

func (v *verrouMemoire) Prendre(_ context.Context, _, proprietaire string, _ time.Duration) (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.proprietaire != "" && v.proprietaire != proprietaire {
		return false, nil
	}
	v.proprietaire = proprietaire
	return true, nil
}

func (v *verrouMemoire) Prolonger(_ context.Context, _, proprietaire string, _ time.Duration) (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.proprietaire == proprietaire, nil
}

func (v *verrouMemoire) lire() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.proprietaire
}

func (v *verrouMemoire) Proprietaire(context.Context, string) (string, bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.proprietaire, v.proprietaire != "", nil
}

func (v *verrouMemoire) Liberer(_ context.Context, _, proprietaire string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.proprietaire == proprietaire {
		v.proprietaire = ""
	}
	return nil
}

type createurMemoire struct {
	mu     sync.Mutex
	tables [][4]table.Joueur
	echec  bool
}

func (c *createurMemoire) CreerClassee(_ context.Context, a, b [2]table.Joueur) (uuid.UUID, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.echec {
		return uuid.Nil, errors.New("plus de place")
	}
	c.tables = append(c.tables, [4]table.Joueur{a[0], b[0], a[1], b[1]})
	return uuid.New(), nil
}

func (c *createurMemoire) lire() [][4]table.Joueur {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tables
}

type diffuseurMemoire struct {
	mu       sync.Mutex
	messages map[uuid.UUID][]protocole.Message
}

func (d *diffuseurMemoire) Envoyer(_ context.Context, j uuid.UUID, m protocole.Message) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.messages[j] = append(d.messages[j], m)
}

func (d *diffuseurMemoire) dernier(j uuid.UUID) protocole.Message {
	d.mu.Lock()
	defer d.mu.Unlock()
	ms := d.messages[j]
	if len(ms) == 0 {
		return protocole.Message{}
	}
	return ms[len(ms)-1]
}

func TestServiceFormeUneTable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source := &sourceMemoire{c: make(chan Commande, 64)}
		stockage := &stockageMemoire{}
		verrou := &verrouMemoire{}
		createur := &createurMemoire{}
		diffuseur := &diffuseurMemoire{messages: map[uuid.UUID][]protocole.Message{}}
		s := NouveauService(Config{Instance: "i1", Parametres: ParametresParDefaut()}, source, stockage, verrou, createur, diffuseur, nil, nil)

		ctx, annuler := context.WithCancel(context.Background())
		defer annuler()
		go s.Executer(ctx)

		js := joueurs(4)
		for _, j := range js {
			require.NoError(t, s.Envoyer(ctx, Commande{Type: CmdRejoindre, Joueur: j}))
		}
		time.Sleep(2 * time.Second)
		synctest.Wait()

		msg := diffuseur.dernier(js[0].ID)
		require.Equal(t, MsgTrouvee, msg.Type)
		proposition := msg.Data.(MessageTrouvee).Proposition
		assert.Len(t, stockage.lire().Propositions, 1, "l'état est sauvegardé")

		for _, j := range js {
			require.NoError(t, s.Envoyer(ctx, Commande{Type: CmdAccepter, Joueur: j, Proposition: proposition}))
		}
		time.Sleep(time.Second)
		synctest.Wait()

		require.Len(t, createur.lire(), 1)
		assert.Empty(t, stockage.lire().Propositions)

		annuler()
		synctest.Wait()
		assert.Empty(t, verrou.proprietaire, "le leader libère la direction en s'arrêtant")
	})
}

func TestServiceCreationEchouee(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source := &sourceMemoire{c: make(chan Commande, 64)}
		createur := &createurMemoire{echec: true}
		diffuseur := &diffuseurMemoire{messages: map[uuid.UUID][]protocole.Message{}}
		s := NouveauService(Config{Instance: "i1", Parametres: ParametresParDefaut()}, source, &stockageMemoire{}, &verrouMemoire{}, createur, diffuseur, nil, nil)

		ctx, annuler := context.WithCancel(context.Background())
		defer annuler()
		go s.Executer(ctx)

		js := joueurs(4)
		for _, j := range js {
			require.NoError(t, s.Envoyer(ctx, Commande{Type: CmdRejoindre, Joueur: j}))
		}
		time.Sleep(2 * time.Second)
		synctest.Wait()
		proposition := diffuseur.dernier(js[0].ID).Data.(MessageTrouvee).Proposition
		for _, j := range js {
			require.NoError(t, s.Envoyer(ctx, Commande{Type: CmdAccepter, Joueur: j, Proposition: proposition}))
		}
		time.Sleep(time.Second)
		synctest.Wait()

		msg := diffuseur.dernier(js[2].ID)
		require.Equal(t, MsgAnnulee, msg.Type)
		assert.Equal(t, "erreur", msg.Data.(MessageAnnulee).Raison)
	})
}

func TestUnSeulLeader(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		verrou := &verrouMemoire{}
		source := &sourceMemoire{c: make(chan Commande, 64)}
		stockage := &stockageMemoire{}
		d := &diffuseurMemoire{messages: map[uuid.UUID][]protocole.Message{}}
		s1 := NouveauService(Config{Instance: "i1", Parametres: ParametresParDefaut()}, source, stockage, verrou, &createurMemoire{}, d, nil, nil)
		s2 := NouveauService(Config{Instance: "i2", Parametres: ParametresParDefaut()}, source, stockage, verrou, &createurMemoire{}, d, nil, nil)

		ctx1, annuler1 := context.WithCancel(context.Background())
		ctx2, annuler2 := context.WithCancel(context.Background())
		defer annuler2()
		go s1.Executer(ctx1)
		time.Sleep(time.Millisecond)
		go s2.Executer(ctx2)
		time.Sleep(time.Second)
		synctest.Wait()
		assert.Equal(t, "i1", verrou.lire())

		j := joueurs(1)[0]
		require.NoError(t, s2.Envoyer(ctx2, Commande{Type: CmdRejoindre, Joueur: j}))
		time.Sleep(time.Second)
		synctest.Wait()
		assert.Len(t, stockage.lire().Tickets, 1, "la commande reçue par une autre instance est traitée par le leader")

		annuler1()
		time.Sleep(10 * time.Second)
		synctest.Wait()
		assert.Equal(t, "i2", verrou.proprietaire, "une autre instance prend le relais")

		require.NoError(t, s2.Envoyer(ctx2, Commande{Type: CmdQuitter, Joueur: j}))
		time.Sleep(time.Second)
		synctest.Wait()
		assert.Empty(t, stockage.lire().Tickets, "le nouveau leader a repris l'état")
	})
}
