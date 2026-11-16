package redisstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Sessions stocke les sessions des joueurs. Seule l'empreinte du jeton est conservée :
// une copie de la base ne permet pas d'usurper une session.
type Sessions struct {
	client *redis.Client
	ttl    time.Duration
}

func NouvellesSessions(client *redis.Client, ttl time.Duration) *Sessions {
	return &Sessions{client: client, ttl: ttl}
}

func cleSession(jeton string) string {
	return prefixe + "session:" + empreinte(jeton)
}

func (s *Sessions) Creer(ctx context.Context, joueur uuid.UUID) (string, error) {
	jeton, err := jetonAleatoire()
	if err != nil {
		return "", err
	}
	if err := s.client.Set(ctx, cleSession(jeton), joueur.String(), s.ttl).Err(); err != nil {
		return "", fmt.Errorf("création de la session : %w", err)
	}
	return jeton, nil
}

// Resoudre renvoie le joueur de la session et repousse son expiration.
func (s *Sessions) Resoudre(ctx context.Context, jeton string) (uuid.UUID, bool, error) {
	if jeton == "" {
		return uuid.Nil, false, nil
	}
	return lireUUID(s.client.GetEx(ctx, cleSession(jeton), s.ttl), "joueur de la session")
}

func (s *Sessions) Supprimer(ctx context.Context, jeton string) error {
	if jeton == "" {
		return nil
	}
	if err := s.client.Del(ctx, cleSession(jeton)).Err(); err != nil {
		return fmt.Errorf("suppression de la session : %w", err)
	}
	return nil
}

// Tickets sont des jetons à usage unique et de courte durée, pour ouvrir un WebSocket
// depuis un client qui ne peut pas envoyer de cookie ni d'en-tête.
type Tickets struct {
	client *redis.Client
	ttl    time.Duration
}

func NouveauxTickets(client *redis.Client, ttl time.Duration) *Tickets {
	return &Tickets{client: client, ttl: ttl}
}

func cleTicket(ticket string) string {
	return prefixe + "ticket:" + empreinte(ticket)
}

func (t *Tickets) Emettre(ctx context.Context, joueur uuid.UUID) (string, error) {
	ticket, err := jetonAleatoire()
	if err != nil {
		return "", err
	}
	if err := t.client.Set(ctx, cleTicket(ticket), joueur.String(), t.ttl).Err(); err != nil {
		return "", fmt.Errorf("émission du ticket : %w", err)
	}
	return ticket, nil
}

func (t *Tickets) Consommer(ctx context.Context, ticket string) (uuid.UUID, bool, error) {
	if ticket == "" {
		return uuid.Nil, false, nil
	}
	return lireUUID(t.client.GetDel(ctx, cleTicket(ticket)), "joueur du ticket")
}

type EtatOAuth struct {
	Fournisseur string `json:"fournisseur"`
	Verifier    string `json:"verifier"`
	Nonce       string `json:"nonce"`
	Retour      string `json:"retour"`
}

// EtatsOAuth conserve, le temps d'un aller-retour chez le fournisseur, les secrets d'une connexion.
type EtatsOAuth struct {
	client *redis.Client
}

func NouveauxEtatsOAuth(client *redis.Client) *EtatsOAuth {
	return &EtatsOAuth{client: client}
}

func cleEtatOAuth(state string) string {
	return prefixe + "oauth:" + empreinte(state)
}

func (o *EtatsOAuth) Sauver(ctx context.Context, state string, e EtatOAuth, ttl time.Duration) error {
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("sérialisation de l'état OAuth : %w", err)
	}
	if err := o.client.Set(ctx, cleEtatOAuth(state), b, ttl).Err(); err != nil {
		return fmt.Errorf("sauvegarde de l'état OAuth : %w", err)
	}
	return nil
}

func (o *EtatsOAuth) Consommer(ctx context.Context, state string) (EtatOAuth, bool, error) {
	if state == "" {
		return EtatOAuth{}, false, nil
	}
	b, err := o.client.GetDel(ctx, cleEtatOAuth(state)).Bytes()
	if errors.Is(err, redis.Nil) {
		return EtatOAuth{}, false, nil
	}
	if err != nil {
		return EtatOAuth{}, false, fmt.Errorf("lecture de l'état OAuth : %w", err)
	}
	var e EtatOAuth
	if err := json.Unmarshal(b, &e); err != nil {
		return EtatOAuth{}, false, fmt.Errorf("état OAuth illisible : %w", err)
	}
	return e, true, nil
}
