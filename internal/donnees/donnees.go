// Package donnees décrit les modèles de lecture partagés entre le stockage et l'API.
package donnees

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

var (
	ErrIntrouvable    = errors.New("introuvable")
	ErrPseudoPris     = errors.New("ce pseudo est déjà pris")
	ErrPseudoInvalide = errors.New("le pseudo doit faire de 3 à 20 caractères : lettres, chiffres, « _ » ou « - »")
)

const (
	PseudoMin = 3
	PseudoMax = 20
)

// Identite est l'identité renvoyée par un fournisseur OAuth.
type Identite struct {
	Fournisseur string
	Sujet       string
	Email       string
	Nom         string
}

type Joueur struct {
	ID          uuid.UUID `json:"id"`
	Pseudo      string    `json:"pseudo"`
	Elo         int       `json:"elo"`
	EloMax      int       `json:"elo_max"`
	NbParties   int       `json:"nb_parties"`
	NbVictoires int       `json:"nb_victoires"`
	// Provisoire : pas encore assez de parties classées pour figurer au classement.
	Provisoire bool      `json:"provisoire"`
	Rang       *int      `json:"rang,omitempty"`
	CreeLe     time.Time `json:"cree_le"`
}

type LigneClassement struct {
	Rang        int       `json:"rang"`
	ID          uuid.UUID `json:"id"`
	Pseudo      string    `json:"pseudo"`
	Elo         int       `json:"elo"`
	NbParties   int       `json:"nb_parties"`
	NbVictoires int       `json:"nb_victoires"`
}

type PageClassement struct {
	Lignes []LigneClassement `json:"lignes"`
	Total  int               `json:"total"`
	Page   int               `json:"page"`
	Taille int               `json:"taille"`
}

type Participant struct {
	Siege    int        `json:"siege"`
	JoueurID *uuid.UUID `json:"joueur,omitempty"`
	Pseudo   string     `json:"pseudo"`
	Bot      bool       `json:"bot"`
	EloAvant *int       `json:"elo_avant,omitempty"`
	EloApres *int       `json:"elo_apres,omitempty"`
	Abandon  bool       `json:"abandon"`
}

type ResumePartie struct {
	ID             uuid.UUID       `json:"id"`
	Type           string          `json:"type"`
	Statut         string          `json:"statut"`
	Cause          *string         `json:"cause,omitempty"`
	EquipeGagnante *int            `json:"equipe_gagnante,omitempty"`
	Scores         *[2]int         `json:"scores,omitempty"`
	Options        json.RawMessage `json:"options"`
	Debut          time.Time       `json:"debut"`
	Fin            *time.Time      `json:"fin,omitempty"`
	Participants   []Participant   `json:"participants"`
}

// PartieJouee est une partie terminée, avec les scores cumulés après chaque mène.
type PartieJouee struct {
	ResumePartie
	Menes [][2]int `json:"menes"`
}

// Participe indique si le joueur était assis à cette partie.
func (r ResumePartie) Participe(joueur uuid.UUID) bool {
	for _, p := range r.Participants {
		if p.JoueurID != nil && *p.JoueurID == joueur {
			return true
		}
	}
	return false
}

type PointElo struct {
	Partie uuid.UUID `json:"partie"`
	Date   time.Time `json:"date"`
	Elo    int       `json:"elo"`
	Delta  int       `json:"delta"`
}

// Joueurs donne accès aux comptes.
type Joueurs interface {
	JoueurParID(ctx context.Context, id uuid.UUID) (Joueur, error)
	JoueurParPseudo(ctx context.Context, pseudo string) (Joueur, error)
	// TrouverOuCreer renvoie le joueur lié à l'identité, en le créant au besoin (cree = vrai).
	TrouverOuCreer(ctx context.Context, id Identite) (j Joueur, cree bool, err error)
	Renommer(ctx context.Context, id uuid.UUID, pseudo string) (Joueur, error)
}

// Historique donne accès aux parties jouées et au classement.
type Historique interface {
	Classement(ctx context.Context, page, taille int) (PageClassement, error)
	PartiesDuJoueur(ctx context.Context, joueur uuid.UUID, avant *time.Time, limite int) ([]ResumePartie, error)
	// PartiesJouees liste les parties terminées, des plus récentes aux plus anciennes ; types vide : toutes.
	PartiesJouees(ctx context.Context, types []string, avant *time.Time, limite int) ([]PartieJouee, error)
	EvolutionElo(ctx context.Context, joueur uuid.UUID, limite int) ([]PointElo, error)
	Partie(ctx context.Context, id uuid.UUID) (ResumePartie, error)
}

func ValiderPseudo(p string) error {
	if n := len([]rune(p)); n < PseudoMin || n > PseudoMax {
		return ErrPseudoInvalide
	}
	for _, r := range p {
		if !estCaracterePseudo(r) {
			return ErrPseudoInvalide
		}
	}
	return nil
}

// SuggererPseudo dérive un pseudo valide d'un nom affiché ; à défaut, « Joueur ».
func SuggererPseudo(nom string) string {
	var b strings.Builder
	for _, r := range nom {
		switch {
		case estCaracterePseudo(r):
			b.WriteRune(r)
		case unicode.IsSpace(r) && b.Len() > 0:
			b.WriteRune('_')
		}
		if len([]rune(b.String())) >= PseudoMax-4 {
			break
		}
	}
	p := strings.Trim(b.String(), "_-")
	if len([]rune(p)) < PseudoMin {
		return "Joueur"
	}
	return p
}

func estCaracterePseudo(r rune) bool {
	return r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-')
}
