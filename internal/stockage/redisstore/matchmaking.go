package redisstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/felix-marchenay/coinchenet-core/internal/matchmaking"
)

const (
	cleCommandesMatchmaking = prefixe + "matchmaking:commandes"
	cleEtatMatchmaking      = prefixe + "matchmaking:etat"
)

// FileMatchmaking transporte les commandes de la file vers l'instance leader.
type FileMatchmaking struct {
	client *redis.Client
}

func NouvelleFileMatchmaking(client *redis.Client) *FileMatchmaking {
	return &FileMatchmaking{client: client}
}

func (f *FileMatchmaking) Publier(ctx context.Context, c matchmaking.Commande) error {
	b, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("sérialisation de la commande de file : %w", err)
	}
	if err := f.client.RPush(ctx, cleCommandesMatchmaking, b).Err(); err != nil {
		return fmt.Errorf("envoi de la commande de file : %w", err)
	}
	return nil
}

// Recevoir attend une commande au plus `attente` ; au-delà, elle renvoie (zéro, false, nil).
// Une commande illisible est écartée sans erreur.
func (f *FileMatchmaking) Recevoir(ctx context.Context, attente time.Duration) (matchmaking.Commande, bool, error) {
	valeurs, err := f.bloquer(ctx, attente)
	if errors.Is(err, redis.Nil) {
		return matchmaking.Commande{}, false, nil
	}
	if err != nil {
		return matchmaking.Commande{}, false, fmt.Errorf("lecture de la file de commandes : %w", err)
	}
	if len(valeurs) != 2 {
		return matchmaking.Commande{}, false, fmt.Errorf("réponse BLPOP inattendue : %v", valeurs)
	}
	var c matchmaking.Commande
	if json.Unmarshal([]byte(valeurs[1]), &c) != nil {
		return matchmaking.Commande{}, false, nil //nolint:nilerr // une commande illisible est écartée sans bloquer le leader.
	}
	return c, true, nil
}

// bloquer utilise un délai fractionnaire sous la seconde (Redis ≥ 6), que BLPop arrondirait à 1 s.
func (f *FileMatchmaking) bloquer(ctx context.Context, attente time.Duration) ([]string, error) {
	if attente >= time.Second || attente <= 0 {
		return f.client.BLPop(ctx, max(attente, time.Second), cleCommandesMatchmaking).Result()
	}
	secondes := strconv.FormatFloat(attente.Seconds(), 'f', 3, 64)
	return f.client.Do(ctx, "BLPOP", cleCommandesMatchmaking, secondes).StringSlice()
}

// EtatMatchmaking conserve l'état de la file pour qu'un nouveau leader le reprenne.
type EtatMatchmaking struct {
	client *redis.Client
}

func NouvelEtatMatchmaking(client *redis.Client) *EtatMatchmaking {
	return &EtatMatchmaking{client: client}
}

func (e *EtatMatchmaking) Charger(ctx context.Context) (matchmaking.Etat, error) {
	b, err := e.client.Get(ctx, cleEtatMatchmaking).Bytes()
	if errors.Is(err, redis.Nil) {
		return matchmaking.NouvelEtat(), nil
	}
	if err != nil {
		return matchmaking.Etat{}, fmt.Errorf("lecture de l'état de la file : %w", err)
	}
	var etat matchmaking.Etat
	if err := json.Unmarshal(b, &etat); err != nil {
		return matchmaking.Etat{}, fmt.Errorf("état de la file illisible : %w", err)
	}
	vide := matchmaking.NouvelEtat()
	if etat.Tickets == nil {
		etat.Tickets = vide.Tickets
	}
	if etat.Propositions == nil {
		etat.Propositions = vide.Propositions
	}
	if etat.Invitations == nil {
		etat.Invitations = vide.Invitations
	}
	return etat, nil
}

func (e *EtatMatchmaking) Sauver(ctx context.Context, etat matchmaking.Etat) error {
	b, err := json.Marshal(etat)
	if err != nil {
		return fmt.Errorf("sérialisation de l'état de la file : %w", err)
	}
	if err := e.client.Set(ctx, cleEtatMatchmaking, b, 0).Err(); err != nil {
		return fmt.Errorf("sauvegarde de l'état de la file : %w", err)
	}
	return nil
}
