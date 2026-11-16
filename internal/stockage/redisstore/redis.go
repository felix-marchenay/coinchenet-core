// Package redisstore regroupe l'état partagé entre les instances : verrous, bus de messages,
// sessions, annuaire des joueurs et file du matchmaking.
package redisstore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"github.com/redis/go-redis/v9"
)

const prefixe = "coinche:"

func Ouvrir(ctx context.Context, url string) (*redis.Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("URL Redis invalide : %w", err)
	}
	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connexion à Redis impossible : %w", err)
	}
	return client, nil
}

var (
	// comparerEtSupprimer ne supprime la clé que si elle a encore la valeur attendue.
	comparerEtSupprimer = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0`)

	// comparerEtProlonger ne prolonge la clé que si elle a encore la valeur attendue.
	comparerEtProlonger = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0`)
)

func jetonAleatoire() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("génération d'un jeton impossible : %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func empreinte(jeton string) string {
	h := sha256.Sum256([]byte(jeton))
	return hex.EncodeToString(h[:])
}
