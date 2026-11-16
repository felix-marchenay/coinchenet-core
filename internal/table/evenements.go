package table

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/felix-marchenay/coinchenet-core/internal/classement"
	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
)

type TypeEvenement string

// Les événements « source » suffisent à reconstruire la partie ; les autres sont des faits
// dérivés, conservés pour le replay et l'analyse.
const (
	EvtPartieCreee    TypeEvenement = "partie_creee"
	EvtDonne          TypeEvenement = "donne"
	EvtEnchere        TypeEvenement = "enchere"
	EvtContrat        TypeEvenement = "contrat"
	EvtRedistribution TypeEvenement = "redistribution"
	EvtCarte          TypeEvenement = "carte"
	EvtPli            TypeEvenement = "pli"
	EvtMene           TypeEvenement = "mene"
	EvtAbandon        TypeEvenement = "abandon"
	EvtChat           TypeEvenement = "chat"
	EvtPartieTerminee TypeEvenement = "partie_terminee"
)

type Evenement struct {
	Sequence   int64           `json:"seq"`
	Type       TypeEvenement   `json:"type"`
	Donnees    json.RawMessage `json:"donnees"`
	Horodatage time.Time       `json:"horodatage"`
}

func (e Evenement) Decoder(v any) error {
	if err := json.Unmarshal(e.Donnees, v); err != nil {
		return fmt.Errorf("événement %d (%s) illisible : %w", e.Sequence, e.Type, err)
	}
	return nil
}

type PartieCreee struct {
	Type           Type                           `json:"type"`
	Options        Options                        `json:"options"`
	Sieges         [coinche.NbSieges]SiegeInitial `json:"sieges"`
	PremierDonneur coinche.Siege                  `json:"premier_donneur"`
}

// DonneDistribuee contient le paquet complet : la partie se rejoue sans aucun tirage.
type DonneDistribuee struct {
	Mene    int             `json:"mene"`
	Donneur coinche.Siege   `json:"donneur"`
	Paquet  []coinche.Carte `json:"paquet"`
}

type ContratFixe struct {
	Contrat        coinche.Contrat `json:"contrat"`
	Preneur        coinche.Siege   `json:"preneur"`
	Multiplicateur int             `json:"multiplicateur"`
}

type Redistribution struct {
	Mene int `json:"mene"`
}

type CarteJouee struct {
	Siege  coinche.Siege         `json:"siege"`
	Carte  coinche.Carte         `json:"carte"`
	Belote coinche.AnnonceBelote `json:"belote,omitempty"`
}

type PliRemporte struct {
	Pli    coinche.Pli `json:"pli"`
	Points int         `json:"points"`
}

type MeneTerminee struct {
	Mene     int                  `json:"mene"`
	Resultat coinche.ResultatMene `json:"resultat"`
	Scores   [2]int               `json:"scores"`
}

type Abandon struct {
	Siege coinche.Siege `json:"siege"`
}

type Chat struct {
	Siege   coinche.Siege `json:"siege"`
	Pseudo  string        `json:"pseudo"`
	Texte   string        `json:"texte,omitempty"`
	Message string        `json:"message,omitempty"`
}

type FinPartie struct {
	Gagnante coinche.Equipe         `json:"gagnante"`
	Cause    Cause                  `json:"cause"`
	Scores   [2]int                 `json:"scores"`
	Elo      []classement.Variation `json:"elo,omitempty"`
}

func nouvelEvenement(seq int64, t TypeEvenement, donnees any, date time.Time) (Evenement, error) {
	b, err := json.Marshal(donnees)
	if err != nil {
		return Evenement{}, fmt.Errorf("sérialisation de l'événement %s : %w", t, err)
	}
	return Evenement{Sequence: seq, Type: t, Donnees: b, Horodatage: date.UTC()}, nil
}

// ErrJournalIncoherent signale un journal qui ne permet pas de reconstruire la partie.
type ErrJournalIncoherent struct {
	Partie   uuid.UUID
	Sequence int64
	Cause    error
}

func (e *ErrJournalIncoherent) Error() string {
	return fmt.Sprintf("journal de la partie %s incohérent à l'événement %d : %v", e.Partie, e.Sequence, e.Cause)
}

func (e *ErrJournalIncoherent) Unwrap() error {
	return e.Cause
}
