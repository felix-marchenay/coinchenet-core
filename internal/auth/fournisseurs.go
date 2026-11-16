package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/felix-marchenay/coinchenet-core/internal/donnees"
)

// Fournisseur est un fournisseur d'identité OAuth 2.0.
type Fournisseur interface {
	Nom() string
	URLAutorisation(ctx context.Context, state, verifier, nonce string) (string, error)
	Identifier(ctx context.Context, code, verifier, nonce string) (donnees.Identite, error)
}

const emetteurGoogle = "https://accounts.google.com"

// Google utilise OpenID Connect (code + PKCE) ; la découverte est faite au premier usage
// pour ne pas empêcher le démarrage si Google est momentanément injoignable.
type Google struct {
	clientID, secret, retour string

	mu       sync.Mutex
	config   *oauth2.Config
	verifier *oidc.IDTokenVerifier
}

func NouveauGoogle(clientID, secret, urlRetour string) *Google {
	return &Google{clientID: clientID, secret: secret, retour: urlRetour}
}

func (g *Google) Nom() string {
	return "google"
}

func (g *Google) initialiser(ctx context.Context) (*oauth2.Config, *oidc.IDTokenVerifier, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.config != nil {
		return g.config, g.verifier, nil
	}
	p, err := oidc.NewProvider(ctx, emetteurGoogle)
	if err != nil {
		return nil, nil, fmt.Errorf("découverte OpenID de Google : %w", err)
	}
	g.config = &oauth2.Config{
		ClientID:     g.clientID,
		ClientSecret: g.secret,
		RedirectURL:  g.retour,
		Endpoint:     p.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}
	g.verifier = p.Verifier(&oidc.Config{ClientID: g.clientID})
	return g.config, g.verifier, nil
}

func (g *Google) URLAutorisation(ctx context.Context, state, verifier, nonce string) (string, error) {
	c, _, err := g.initialiser(ctx)
	if err != nil {
		return "", err
	}
	return c.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oidc.Nonce(nonce)), nil
}

func (g *Google) Identifier(ctx context.Context, code, verifier, nonce string) (donnees.Identite, error) {
	c, v, err := g.initialiser(ctx)
	if err != nil {
		return donnees.Identite{}, err
	}
	jeton, err := c.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return donnees.Identite{}, fmt.Errorf("échange du code Google : %w", err)
	}
	brut, ok := jeton.Extra("id_token").(string)
	if !ok {
		return donnees.Identite{}, errors.New("réponse Google sans id_token")
	}
	idToken, err := v.Verify(ctx, brut)
	if err != nil {
		return donnees.Identite{}, fmt.Errorf("id_token Google invalide : %w", err)
	}
	if idToken.Nonce != nonce {
		return donnees.Identite{}, errors.New("nonce Google invalide")
	}
	var claims struct {
		Email   string `json:"email"`
		Verifie bool   `json:"email_verified"`
		Nom     string `json:"name"`
		Prenom  string `json:"given_name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return donnees.Identite{}, fmt.Errorf("lecture de l'identité Google : %w", err)
	}
	id := donnees.Identite{Fournisseur: g.Nom(), Sujet: idToken.Subject, Nom: claims.Prenom}
	if id.Nom == "" {
		id.Nom = claims.Nom
	}
	if claims.Verifie {
		id.Email = claims.Email
	}
	return id, nil
}

// Discord utilise OAuth 2.0 classique puis l'API utilisateur.
type Discord struct {
	config *oauth2.Config
	urlMoi string
	client *http.Client
}

func NouveauDiscord(clientID, secret, urlRetour string) *Discord {
	return &Discord{
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: secret,
			RedirectURL:  urlRetour,
			Endpoint: oauth2.Endpoint{ //nolint:gosec // URLs publiques, pas des secrets.
				AuthURL:   "https://discord.com/oauth2/authorize",
				TokenURL:  "https://discord.com/api/oauth2/token",
				AuthStyle: oauth2.AuthStyleInParams,
			},
			Scopes: []string{"identify"},
		},
		urlMoi: "https://discord.com/api/users/@me",
		client: http.DefaultClient,
	}
}

func (d *Discord) Nom() string {
	return "discord"
}

func (d *Discord) URLAutorisation(_ context.Context, state, verifier, _ string) (string, error) {
	return d.config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier)), nil
}

func (d *Discord) Identifier(ctx context.Context, code, verifier, _ string) (donnees.Identite, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, d.client)
	jeton, err := d.config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return donnees.Identite{}, fmt.Errorf("échange du code Discord : %w", err)
	}
	requete, err := http.NewRequestWithContext(ctx, http.MethodGet, d.urlMoi, http.NoBody) //nolint:gosec // URL fixe de l'API Discord.
	if err != nil {
		return donnees.Identite{}, err
	}
	reponse, err := d.config.Client(ctx, jeton).Do(requete) //nolint:gosec // URL fixe de l'API Discord.
	if err != nil {
		return donnees.Identite{}, fmt.Errorf("profil Discord : %w", err)
	}
	defer reponse.Body.Close()
	if reponse.StatusCode != http.StatusOK {
		return donnees.Identite{}, fmt.Errorf("profil Discord : statut %d", reponse.StatusCode)
	}
	var u struct {
		ID         string `json:"id"`
		Username   string `json:"username"`
		GlobalName string `json:"global_name"`
	}
	if err := json.NewDecoder(reponse.Body).Decode(&u); err != nil {
		return donnees.Identite{}, fmt.Errorf("profil Discord illisible : %w", err)
	}
	if u.ID == "" {
		return donnees.Identite{}, errors.New("profil Discord sans identifiant")
	}
	nom := u.GlobalName
	if nom == "" {
		nom = u.Username
	}
	return donnees.Identite{Fournisseur: d.Nom(), Sujet: u.ID, Nom: nom}, nil
}
