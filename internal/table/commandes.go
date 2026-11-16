package table

import (
	"encoding/json"

	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
)

type TypeCommande string

const (
	CmdConnexion   TypeCommande = "connexion"
	CmdDeconnexion TypeCommande = "deconnexion"
	CmdEtat        TypeCommande = "etat"
	CmdEnchere     TypeCommande = "enchere"
	CmdJouer       TypeCommande = "jouer"
	CmdAbandonner  TypeCommande = "abandonner"
	CmdPresent     TypeCommande = "present"
	CmdChat        TypeCommande = "chat"
	CmdSiege       TypeCommande = "siege"
	CmdBot         TypeCommande = "bot"
	CmdLiberer     TypeCommande = "liberer"
	CmdLancer      TypeCommande = "lancer"
	CmdQuitter     TypeCommande = "quitter"
)

// Commande est envoyée par la passerelle WebSocket au nom d'un joueur authentifié.
type Commande struct {
	Type      TypeCommande    `json:"type"`
	Joueur    Joueur          `json:"joueur"`
	Connexion string          `json:"connexion,omitempty"`
	Ref       string          `json:"ref,omitempty"`
	Donnees   json.RawMessage `json:"donnees,omitempty"`
}

type DonneesEnchere struct {
	Action coinche.TypeEnchere   `json:"action"`
	Valeur coinche.ValeurAnnonce `json:"valeur,omitempty"`
	Atout  *coinche.Atout        `json:"atout,omitempty"`
}

func (d DonneesEnchere) Enchere() coinche.Enchere {
	e := coinche.Enchere{Type: d.Action}
	if d.Action == coinche.Annonce && d.Atout != nil {
		e.Contrat = &coinche.Contrat{Atout: *d.Atout, Valeur: d.Valeur}
	}
	return e
}

type DonneesJouer struct {
	Carte coinche.Carte `json:"carte"`
}

type DonneesSiege struct {
	Siege coinche.Siege `json:"siege"`
}

// DonneesBot : sans profil, la table attribue le premier qui n'est pas déjà assis.
type DonneesBot struct {
	Siege  coinche.Siege `json:"siege"`
	Profil string        `json:"profil,omitempty"`
}

type DonneesChat struct {
	Texte   string `json:"texte,omitempty"`
	Message string `json:"message,omitempty"`
}

func decoder[T any](c Commande) (T, error) {
	var v T
	if len(c.Donnees) == 0 {
		return v, ErrDonneesInvalides
	}
	if err := json.Unmarshal(c.Donnees, &v); err != nil {
		return v, ErrDonneesInvalides
	}
	return v, nil
}
