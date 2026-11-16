package redisstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/felix-marchenay/coinchenet-core/internal/protocole"
	"github.com/felix-marchenay/coinchenet-core/internal/table"
)

var ErrBusArrete = errors.New("bus de messages non démarré")

// Bus partage une seule connexion PubSub par instance. Les messages d'un canal sont
// distribués dans l'ordre d'arrivée, depuis une seule goroutine : les fonctions
// d'abonnement ne doivent pas bloquer.
type Bus struct {
	client *redis.Client
	log    *slog.Logger

	mu      sync.Mutex
	pubsub  *redis.PubSub
	abonnes map[string]map[int]func([]byte)
	// confirmations attend l'accusé de SUBSCRIBE : sans lui, un message publié
	// juste après l'abonnement pourrait être perdu.
	confirmations map[string]chan struct{}
	suivant       int
	ferme         bool
}

const delaiConfirmation = 5 * time.Second

func NouveauBus(client *redis.Client, log *slog.Logger) *Bus {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Bus{
		client:        client,
		log:           log.With("composant", "bus"),
		abonnes:       map[string]map[int]func([]byte){},
		confirmations: map[string]chan struct{}{},
	}
}

func canalJoueur(joueur uuid.UUID) string {
	return prefixe + "joueur:" + joueur.String()
}

func canalTable(t uuid.UUID) string {
	return prefixe + "table:" + t.String()
}

// Demarrer ouvre la connexion PubSub et lance la distribution jusqu'à l'annulation de ctx.
func (b *Bus) Demarrer(ctx context.Context) error {
	// Un canal propre à l'instance met la connexion en mode abonnement dès le départ.
	interne := prefixe + "bus:" + uuid.NewString()
	ps := b.client.Subscribe(ctx, interne)
	if _, err := ps.Receive(ctx); err != nil {
		_ = ps.Close()
		return fmt.Errorf("abonnement au bus impossible : %w", err)
	}

	b.mu.Lock()
	if b.pubsub != nil || b.ferme {
		b.mu.Unlock()
		_ = ps.Close()
		return errors.New("bus déjà démarré")
	}
	b.pubsub = ps
	b.mu.Unlock()

	messages := ps.ChannelWithSubscriptions(redis.WithChannelSize(1024))
	go func() {
		for m := range messages {
			switch m := m.(type) {
			case *redis.Message:
				b.distribuer(m.Channel, []byte(m.Payload))
			case *redis.Subscription:
				if m.Kind == "subscribe" {
					b.confirmer(m.Channel)
				}
			}
		}
	}()
	go func() {
		<-ctx.Done()
		_ = b.Fermer()
	}()
	return nil
}

func (b *Bus) Fermer() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.ferme {
		return nil
	}
	b.ferme = true
	if b.pubsub == nil {
		return nil
	}
	return b.pubsub.Close()
}

func (b *Bus) distribuer(canal string, payload []byte) {
	b.mu.Lock()
	fonctions := make([]func([]byte), 0, len(b.abonnes[canal]))
	for _, f := range b.abonnes[canal] {
		fonctions = append(fonctions, f)
	}
	b.mu.Unlock()
	for _, f := range fonctions {
		f(payload)
	}
}

func (b *Bus) publier(ctx context.Context, canal string, payload []byte) error {
	if err := b.client.Publish(ctx, canal, payload).Err(); err != nil {
		return fmt.Errorf("publication sur %s : %w", canal, err)
	}
	return nil
}

func (b *Bus) confirmer(canal string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c, ok := b.confirmations[canal]; ok {
		close(c)
		delete(b.confirmations, canal)
	}
}

// abonner ajoute une fonction sur un canal et attend que Redis ait confirmé l'abonnement ;
// le ctx ne sert qu'à cette attente.
func (b *Bus) abonner(ctx context.Context, canal string, recevoir func([]byte)) (func(), error) {
	b.mu.Lock()
	if b.pubsub == nil || b.ferme {
		b.mu.Unlock()
		return nil, ErrBusArrete
	}
	fonctions, existe := b.abonnes[canal]
	if !existe {
		b.confirmations[canal] = make(chan struct{})
		if err := b.pubsub.Subscribe(ctx, canal); err != nil {
			delete(b.confirmations, canal)
			b.mu.Unlock()
			return nil, fmt.Errorf("abonnement à %s : %w", canal, err)
		}
		fonctions = map[int]func([]byte){}
		b.abonnes[canal] = fonctions
	}
	id := b.suivant
	b.suivant++
	fonctions[id] = recevoir
	confirmation := b.confirmations[canal]
	b.mu.Unlock()

	var une sync.Once
	desabonner := func() { une.Do(func() { b.desabonner(canal, id) }) }
	if confirmation != nil {
		select {
		case <-confirmation:
		case <-ctx.Done():
			desabonner()
			return nil, ctx.Err()
		case <-time.After(delaiConfirmation):
			b.log.Warn("abonnement non confirmé à temps", "canal", canal)
		}
	}
	return desabonner, nil
}

func (b *Bus) desabonner(canal string, id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	fonctions := b.abonnes[canal]
	delete(fonctions, id)
	if len(fonctions) > 0 {
		return
	}
	delete(b.abonnes, canal)
	delete(b.confirmations, canal)
	if b.pubsub == nil || b.ferme {
		return
	}
	if err := b.pubsub.Unsubscribe(context.Background(), canal); err != nil {
		b.log.Warn("désabonnement impossible", "canal", canal, "erreur", err)
	}
}

func (b *Bus) PublierJoueur(ctx context.Context, joueur uuid.UUID, payload []byte) error {
	return b.publier(ctx, canalJoueur(joueur), payload)
}

func (b *Bus) AbonnerJoueur(ctx context.Context, joueur uuid.UUID, recevoir func([]byte)) (func(), error) {
	return b.abonner(ctx, canalJoueur(joueur), recevoir)
}

func (b *Bus) PublierTable(ctx context.Context, t uuid.UUID, c table.Commande) error {
	payload, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("sérialisation de la commande : %w", err)
	}
	return b.publier(ctx, canalTable(t), payload)
}

func (b *Bus) AbonnerTable(ctx context.Context, t uuid.UUID, recevoir func(table.Commande)) (func(), error) {
	return b.abonner(ctx, canalTable(t), func(payload []byte) {
		var c table.Commande
		if err := json.Unmarshal(payload, &c); err != nil {
			b.log.Warn("commande de table illisible ignorée", "table", t, "erreur", err)
			return
		}
		recevoir(c)
	})
}

// Diffuseur livre les messages du protocole aux joueurs, via le bus.
type Diffuseur struct {
	bus *Bus
	log *slog.Logger
}

func NouveauDiffuseur(bus *Bus, log *slog.Logger) *Diffuseur {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Diffuseur{bus: bus, log: log.With("composant", "diffuseur")}
}

func (d *Diffuseur) Envoyer(ctx context.Context, joueur uuid.UUID, m protocole.Message) {
	payload, err := json.Marshal(m)
	if err != nil {
		d.log.Error("message illisible", "type", m.Type, "erreur", err)
		return
	}
	if err := d.bus.PublierJoueur(ctx, joueur, payload); err != nil {
		d.log.Warn("message non délivré", "joueur", joueur, "type", m.Type, "erreur", err)
	}
}
