// Package table fait vivre une partie en ligne : un acteur (une goroutine) par table
// sérialise les commandes des joueurs, les délais, les bots et les déconnexions,
// et journalise chaque fait sous forme d'événement.
package table

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/felix-marchenay/coinchenet-core/internal/classement"
	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
	"github.com/felix-marchenay/coinchenet-core/internal/protocole"
)

type Type string

const (
	Classee Type = "classee"
	Privee  Type = "privee"
	// AvecBots : partie de la file que des bots complètent, faute d'adversaires ; elle ne compte pas
	// pour l'ELO et, comme en privé, un bot reprend le siège d'un joueur qui abandonne.
	AvecBots Type = "avec_bots"
)

var Types = []Type{Classee, Privee, AvecBots}

type Statut string

const (
	StatutAttente  Statut = "attente"
	StatutEnCours  Statut = "en_cours"
	StatutTerminee Statut = "terminee"
)

type Cause string

const (
	CauseScore   Cause = "score"
	CauseForfait Cause = "forfait"
)

type Joueur struct {
	ID     uuid.UUID `json:"id"`
	Pseudo string    `json:"pseudo"`
	Elo    int       `json:"elo"`
}

// DelaisAutorises liste les délais par action possibles, en secondes (0 = sans limite).
var DelaisAutorises = []int{0, 15, 30, 60}

type Options struct {
	Regles      coinche.Regles `json:"regles"`
	DelaiAction int            `json:"delai_action"`
}

func OptionsParDefaut() Options {
	return Options{Regles: coinche.ReglesParDefaut(), DelaiAction: 30}
}

func OptionsClassees() Options {
	return Options{Regles: coinche.ReglesClassees(), DelaiAction: 30}
}

func (o Options) Valider() error {
	if err := o.Regles.Valider(); err != nil {
		return err
	}
	if !slices.Contains(DelaisAutorises, o.DelaiAction) {
		return fmt.Errorf("délai par action %d invalide, valeurs possibles : %v", o.DelaiAction, DelaisAutorises)
	}
	return nil
}

func (o Options) Delai() time.Duration {
	return time.Duration(o.DelaiAction) * time.Second
}

// MessagesPredefinis sont les messages rapides du chat, que le client traduit.
var MessagesPredefinis = []string{"bonne_chance", "bien_joue", "merci", "desole", "oups", "gg"}

type SiegeInitial struct {
	Joueur *Joueur `json:"joueur,omitempty"`
	Bot    bool    `json:"bot"`
	Profil string  `json:"profil,omitempty"`
}

// Debut est transmis à l'archiviste au lancement de la partie.
type Debut struct {
	PartieID  uuid.UUID
	Type      Type
	Options   Options
	Sieges    [coinche.NbSieges]SiegeInitial
	Evenement Evenement
	Date      time.Time
}

type SiegeFinal struct {
	Joueur  *Joueur
	Bot     bool
	Abandon bool
}

// Fin est transmise à l'archiviste, qui calcule l'ELO des parties classées.
type Fin struct {
	PartieID  uuid.UUID
	Type      Type
	Gagnante  coinche.Equipe
	Cause     Cause
	Scores    [2]int
	Sieges    [coinche.NbSieges]SiegeFinal
	Evenement Evenement
	Date      time.Time
}

// Journal stocke les événements d'une partie, dans l'ordre.
type Journal interface {
	Ajouter(ctx context.Context, partie uuid.UUID, evts []Evenement) error
	Charger(ctx context.Context, partie uuid.UUID) ([]Evenement, error)
}

// Archiviste enregistre le début et la fin d'une partie, événement compris, de façon atomique.
// Terminer doit être idempotent.
type Archiviste interface {
	Commencer(ctx context.Context, d Debut) error
	Terminer(ctx context.Context, f Fin) ([]classement.Variation, error)
}

// Diffuseur livre un message à toutes les connexions d'un joueur, où qu'elles soient.
type Diffuseur interface {
	Envoyer(ctx context.Context, joueur uuid.UUID, m protocole.Message)
}

// Annuaire mémorise la table courante de chaque joueur, pour la reconnexion automatique.
type Annuaire interface {
	Associer(ctx context.Context, joueur, table uuid.UUID) error
	Dissocier(ctx context.Context, joueur, table uuid.UUID) error
}

// Decideur choisit les actions des sièges tenus par un bot, selon le profil de chacun : c'est sa
// famille qui décide de sa façon de jouer. Un profil inconnu joue avec le profil par défaut.
type Decideur interface {
	Encherir(profil string, v coinche.Vue) coinche.Enchere
	Jouer(profil string, v coinche.Vue) coinche.Carte
	Profils() []string
	// Famille renvoie le nom de famille d'un profil et les prénoms que portent ses bots.
	Famille(profil string) (string, []string, bool)
}

// Metriques est optionnel.
type Metriques interface {
	PartieTerminee(t Type, c Cause)
	ActionTraitee(commande string, duree time.Duration)
}

type Dependances struct {
	Journal    Journal
	Archiviste Archiviste
	Diffuseur  Diffuseur
	Annuaire   Annuaire
	Decideur   Decideur
	Melangeur  coinche.Melangeur
	Coupeur    coinche.Coupeur
	Hasard     coinche.Hasard
	Metriques  Metriques
	Logger     *slog.Logger
	Maintenant func() time.Time
}

func (d Dependances) completer() Dependances {
	if d.Melangeur == nil {
		d.Melangeur = coinche.FischerYatesMelangeur{}
	}
	if d.Coupeur == nil {
		d.Coupeur = coinche.RandomCoupeur{}
	}
	if d.Hasard == nil {
		d.Hasard = coinche.HasardCrypto{}
	}
	if d.Logger == nil {
		d.Logger = slog.New(slog.DiscardHandler)
	}
	if d.Maintenant == nil {
		d.Maintenant = time.Now
	}
	return d
}

type Config struct {
	// DelaiBot rythme les actions automatiques pour que les clients puissent les afficher.
	DelaiBot        time.Duration
	DelaiDemarrage  time.Duration
	DelaiEntreMenes time.Duration
	// DelaiRetour est le temps laissé à un joueur déconnecté en partie classée avant le forfait.
	DelaiRetour time.Duration
	// DelaiFermeture garde la table en mémoire après la fin, le temps que les clients lisent le résultat.
	DelaiFermeture time.Duration
	DelaiLobby     time.Duration
	// DelaisAvantAbsence : nombre de délais expirés d'affilée avant de déclarer un joueur absent.
	DelaisAvantAbsence int
	ChatMax            int
	ChatFenetre        time.Duration
	TimeoutStockage    time.Duration
}

func ConfigParDefaut() Config {
	return Config{
		DelaiBot:           800 * time.Millisecond,
		DelaiDemarrage:     3 * time.Second,
		DelaiEntreMenes:    3 * time.Second,
		DelaiRetour:        3 * time.Minute,
		DelaiFermeture:     time.Minute,
		DelaiLobby:         30 * time.Minute,
		DelaisAvantAbsence: 2,
		ChatMax:            5,
		ChatFenetre:        10 * time.Second,
		TimeoutStockage:    5 * time.Second,
	}
}

// ErreurTable est renvoyée au joueur avec un code stable.
type ErreurTable struct {
	Code    string
	Message string
}

func (e *ErreurTable) Error() string {
	return e.Message
}

func erreur(code, message string) *ErreurTable {
	return &ErreurTable{Code: code, Message: message}
}

var (
	ErrNonAssis             = erreur("non_assis", "vous n'êtes pas assis à cette table")
	ErrExclu                = erreur("exclu", "le créateur de la table vous en a exclu")
	ErrTableComplete        = erreur("table_complete", "la table est complète")
	ErrReserveAuCreateur    = erreur(protocole.CodeInterdit, "action réservée au créateur de la table")
	ErrSiegeInvalide        = erreur(protocole.CodeRequeteInvalide, "siège invalide")
	ErrSiegeOccupe          = erreur("siege_occupe", "ce siège est occupé")
	ErrProfilInconnu        = erreur("profil_inconnu", "profil de bot inconnu")
	ErrSiegeLibre           = erreur("siege_libre", "ce siège est libre")
	ErrTableIncomplete      = erreur("table_incomplete", "il faut quatre joueurs pour lancer la partie")
	ErrPartieNonCommencee   = erreur("partie_non_commencee", "la partie n'a pas commencé")
	ErrPartieDejaLancee     = erreur("partie_deja_lancee", "la partie est déjà lancée")
	ErrPartieFinie          = erreur("partie_terminee", "la partie est terminée")
	ErrCommandeInconnue     = erreur(protocole.CodeRequeteInvalide, "commande inconnue")
	ErrDonneesInvalides     = erreur(protocole.CodeRequeteInvalide, "données invalides")
	ErrChatInvalide         = erreur("chat_invalide", "message de chat invalide")
	ErrChatTropRapide       = erreur(protocole.CodeTropDeRequetes, "trop de messages, patientez un peu")
	ErrStockageIndisponible = erreur(protocole.CodeIndisponible, "la partie est momentanément indisponible")
)

// codeErreur traduit une erreur en code et message pour le client.
func codeErreur(err error) (string, string) {
	var et *ErreurTable
	if errors.As(err, &et) {
		return et.Code, et.Message
	}
	codes := []struct {
		err  error
		code string
	}{
		{coinche.ErrPasSonTour, "pas_ton_tour"},
		{coinche.ErrCarteInterdite, "carte_interdite"},
		{coinche.ErrCarteAbsente, "carte_absente"},
		{coinche.ErrEnchereInvalide, "enchere_invalide"},
		{coinche.ErrPhase, "phase"},
		{coinche.ErrAucuneMene, "phase"},
		{coinche.ErrPartieTerminee, "partie_terminee"},
	}
	for _, c := range codes {
		if errors.Is(err, c.err) {
			return c.code, err.Error()
		}
	}
	return protocole.CodeInterne, "erreur interne"
}
