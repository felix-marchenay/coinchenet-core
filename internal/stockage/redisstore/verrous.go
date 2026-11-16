package redisstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Verrous implémente des verrous à durée de vie, détenus par un propriétaire nommé.
type Verrous struct {
	client *redis.Client
}

func NouveauxVerrous(client *redis.Client) *Verrous {
	return &Verrous{client: client}
}

func cleVerrou(cle string) string {
	return prefixe + "verrou:" + cle
}

func (v *Verrous) Prendre(ctx context.Context, cle, proprietaire string, ttl time.Duration) (bool, error) {
	ok, err := v.client.SetNX(ctx, cleVerrou(cle), proprietaire, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("prise du verrou %s : %w", cle, err)
	}
	return ok, nil
}

func (v *Verrous) Prolonger(ctx context.Context, cle, proprietaire string, ttl time.Duration) (bool, error) {
	n, err := comparerEtProlonger.Run(ctx, v.client, []string{cleVerrou(cle)}, proprietaire, ttl.Milliseconds()).Int()
	if err != nil {
		return false, fmt.Errorf("prolongation du verrou %s : %w", cle, err)
	}
	return n == 1, nil
}

// Proprietaire renvoie le détenteur actuel du verrou, s'il y en a un.
func (v *Verrous) Proprietaire(ctx context.Context, cle string) (string, bool, error) {
	p, err := v.client.Get(ctx, cleVerrou(cle)).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("lecture du verrou %s : %w", cle, err)
	}
	return p, true, nil
}

func (v *Verrous) Liberer(ctx context.Context, cle, proprietaire string) error {
	if err := comparerEtSupprimer.Run(ctx, v.client, []string{cleVerrou(cle)}, proprietaire).Err(); err != nil {
		return fmt.Errorf("libération du verrou %s : %w", cle, err)
	}
	return nil
}
