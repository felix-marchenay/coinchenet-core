package table

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
	"github.com/felix-marchenay/coinchenet-core/internal/protocole"
)

// Types des messages envoyés aux joueurs.
const (
	MsgEtat           = "partie.etat"
	MsgCommencee      = "partie.commencee"
	MsgDonne          = "partie.donne"
	MsgTour           = "partie.tour"
	MsgEnchere        = "partie.enchere"
	MsgContrat        = "partie.contrat"
	MsgRedistribution = "partie.redistribution"
	MsgCarte          = "partie.carte"
	MsgPli            = "partie.pli"
	MsgMene           = "partie.mene"
	MsgStatut         = "partie.statut"
	MsgChat           = "partie.chat"
	MsgTerminee       = "partie.terminee"
	MsgFermee         = "table.fermee"
	MsgExclu          = "table.exclu"
)

// Statuts d'un siège, vus par les clients.
const (
	SiegeConnecte   = "connecte"
	SiegeDeconnecte = "deconnecte"
	SiegeAbsent     = "absent"
	SiegeBot        = "bot"
	SiegeAbandon    = "abandon"
)

type Entete struct {
	Table uuid.UUID `json:"table"`
	Seq   int64     `json:"seq"`
}

type EtatSiege struct {
	Joueur      *Joueur    `json:"joueur,omitempty"`
	Bot         bool       `json:"bot"`
	Profil      string     `json:"profil,omitempty"`
	Statut      string     `json:"statut"`
	RetourAvant *time.Time `json:"retour_avant,omitempty"`
}

// EtatTable est l'instantané complet envoyé à un joueur à sa (re)connexion.
type EtatTable struct {
	Entete
	Type     Type                         `json:"type"`
	Statut   Statut                       `json:"statut"`
	Code     string                       `json:"code,omitempty"`
	Createur *uuid.UUID                   `json:"createur,omitempty"`
	Options  Options                      `json:"options"`
	Sieges   [coinche.NbSieges]*EtatSiege `json:"sieges"`
	Moi      *coinche.Siege               `json:"moi,omitempty"`
	Echeance *time.Time                   `json:"echeance,omitempty"`
	Jeu      *coinche.Vue                 `json:"jeu,omitempty"`
	Fin      *FinPartie                   `json:"fin,omitempty"`
}

type MessageDonne struct {
	Entete
	Mene    int             `json:"mene"`
	Donneur coinche.Siege   `json:"donneur"`
	Main    []coinche.Carte `json:"main"`
}

type MessageTour struct {
	Entete
	Siege          coinche.Siege           `json:"siege"`
	Phase          coinche.Phase           `json:"phase"`
	Echeance       *time.Time              `json:"echeance,omitempty"`
	CartesJouables []coinche.Carte         `json:"cartes_jouables,omitempty"`
	OptionsEnchere *coinche.OptionsEnchere `json:"options_enchere,omitempty"`
}

type MessageStatut struct {
	Entete
	Siege       coinche.Siege `json:"siege"`
	Statut      string        `json:"statut"`
	RetourAvant *time.Time    `json:"retour_avant,omitempty"`
}

type MessageCommencee struct {
	Table uuid.UUID `json:"table"`
	Type  Type      `json:"type"`
}

type MessageFermee struct {
	Table  uuid.UUID `json:"table"`
	Raison string    `json:"raison"`
}

var messagesPublics = map[TypeEvenement]string{
	EvtEnchere:        MsgEnchere,
	EvtContrat:        MsgContrat,
	EvtRedistribution: MsgRedistribution,
	EvtCarte:          MsgCarte,
	EvtPli:            MsgPli,
	EvtMene:           MsgMene,
	EvtChat:           MsgChat,
}

// messagePublic traduit un événement en message diffusé à toute la table :
// les champs de l'événement, complétés de la table et du numéro de séquence.
func messagePublic(table uuid.UUID, e Evenement) (protocole.Message, bool) {
	t, ok := messagesPublics[e.Type]
	if !ok {
		return protocole.Message{}, false
	}
	champs := map[string]json.RawMessage{}
	if err := json.Unmarshal(e.Donnees, &champs); err != nil {
		return protocole.Message{}, false
	}
	champs["table"], _ = json.Marshal(table)
	champs["seq"], _ = json.Marshal(e.Sequence)
	return protocole.Message{Type: t, Data: champs}, true
}
