// Package protocole définit l'enveloppe des messages échangés sur le WebSocket.
package protocole

import "encoding/json"

// Message est envoyé par le serveur. Data est sérialisé tel quel.
type Message struct {
	Type string `json:"type"`
	Ref  string `json:"ref,omitempty"`
	Data any    `json:"data,omitempty"`
}

// Enveloppe est reçue du client ; Data est décodé selon Type.
type Enveloppe struct {
	Type string          `json:"type"`
	Ref  string          `json:"ref,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

const TypeErreur = "erreur"

// Codes d'erreur stables, destinés aux clients.
const (
	CodeRequeteInvalide = "requete_invalide"
	CodeNonAuthentifie  = "non_authentifie"
	CodeInterdit        = "interdit"
	CodeIntrouvable     = "introuvable"
	CodeConflit         = "conflit"
	CodeTropDeRequetes  = "trop_de_requetes"
	CodeIndisponible    = "indisponible"
	CodeInterne         = "interne"
)

type Erreur struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NouvelleErreur(ref, code, message string) Message {
	return Message{Type: TypeErreur, Ref: ref, Data: Erreur{Code: code, Message: message}}
}
