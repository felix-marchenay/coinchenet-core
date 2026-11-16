package redisstore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// DureeAnnuaire borne la vie d'une association joueur → table si sa fin n'a jamais été notée.
const DureeAnnuaire = 12 * time.Hour

// Annuaire mémorise la table courante de chaque joueur.
type Annuaire struct {
	client *redis.Client
}

func NouvelAnnuaire(client *redis.Client) *Annuaire {
	return &Annuaire{client: client}
}

func cleAnnuaire(joueur uuid.UUID) string {
	return prefixe + "joueur:" + joueur.String() + ":table"
}

func (a *Annuaire) Associer(ctx context.Context, joueur, table uuid.UUID) error {
	if err := a.client.Set(ctx, cleAnnuaire(joueur), table.String(), DureeAnnuaire).Err(); err != nil {
		return fmt.Errorf("association du joueur à sa table : %w", err)
	}
	return nil
}

func (a *Annuaire) Dissocier(ctx context.Context, joueur, table uuid.UUID) error {
	if err := comparerEtSupprimer.Run(ctx, a.client, []string{cleAnnuaire(joueur)}, table.String()).Err(); err != nil {
		return fmt.Errorf("dissociation du joueur de sa table : %w", err)
	}
	return nil
}

func (a *Annuaire) TableDe(ctx context.Context, joueur uuid.UUID) (uuid.UUID, bool, error) {
	return lireUUID(a.client.Get(ctx, cleAnnuaire(joueur)), "table du joueur")
}

// Codes associe les codes d'invitation aux tables privées, sans tenir compte de la casse.
type Codes struct {
	client *redis.Client
}

func NouveauxCodes(client *redis.Client) *Codes {
	return &Codes{client: client}
}

func NormaliserCode(code string) string {
	return strings.ToUpper(strings.Join(strings.Fields(code), ""))
}

func cleCode(code string) string {
	return prefixe + "code:" + NormaliserCode(code)
}

func (c *Codes) Reserver(ctx context.Context, code string, table uuid.UUID, ttl time.Duration) (bool, error) {
	ok, err := c.client.SetNX(ctx, cleCode(code), table.String(), ttl).Result()
	if err != nil {
		return false, fmt.Errorf("réservation du code : %w", err)
	}
	return ok, nil
}

func (c *Codes) Resoudre(ctx context.Context, code string) (uuid.UUID, bool, error) {
	if NormaliserCode(code) == "" {
		return uuid.Nil, false, nil
	}
	return lireUUID(c.client.Get(ctx, cleCode(code)), "code de table")
}

// Liberer ne supprime le code que s'il désigne encore cette table.
func (c *Codes) Liberer(ctx context.Context, code string, table uuid.UUID) error {
	if err := comparerEtSupprimer.Run(ctx, c.client, []string{cleCode(code)}, table.String()).Err(); err != nil {
		return fmt.Errorf("libération du code : %w", err)
	}
	return nil
}

func lireUUID(cmd *redis.StringCmd, quoi string) (uuid.UUID, bool, error) {
	v, err := cmd.Result()
	if errors.Is(err, redis.Nil) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("lecture du %s : %w", quoi, err)
	}
	id, err := uuid.Parse(v)
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("%s illisible : %w", quoi, err)
	}
	return id, true, nil
}
