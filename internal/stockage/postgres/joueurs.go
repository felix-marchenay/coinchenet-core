package postgres

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/felix-marchenay/coinchenet-core/internal/classement"
	"github.com/felix-marchenay/coinchenet-core/internal/donnees"
)

const colonnesJoueur = `j.id, j.pseudo::text, j.elo, j.elo_max, j.nb_parties_classees, j.nb_victoires_classees, j.cree_le,
	case when j.nb_parties_classees >= $1 then
		1 + (select count(*) from joueurs autre where autre.nb_parties_classees >= $1 and autre.elo > j.elo)
	end`

func lireJoueur(l pgx.Row) (donnees.Joueur, error) {
	var j donnees.Joueur
	var rang *int
	err := l.Scan(&j.ID, &j.Pseudo, &j.Elo, &j.EloMax, &j.NbParties, &j.NbVictoires, &j.CreeLe, &rang)
	if errors.Is(err, pgx.ErrNoRows) {
		return j, donnees.ErrIntrouvable
	}
	if err != nil {
		return j, err
	}
	j.Provisoire = j.NbParties < classement.SeuilClassement
	j.Rang = rang
	j.CreeLe = j.CreeLe.UTC()
	return j, nil
}

func (b *Base) JoueurParID(ctx context.Context, id uuid.UUID) (donnees.Joueur, error) {
	j, err := lireJoueur(b.pool.QueryRow(ctx,
		"select "+colonnesJoueur+" from joueurs j where j.id = $2", classement.SeuilClassement, id))
	if err != nil && !errors.Is(err, donnees.ErrIntrouvable) {
		return j, fmt.Errorf("lecture du joueur %s : %w", id, err)
	}
	return j, err
}

func (b *Base) JoueurParPseudo(ctx context.Context, pseudo string) (donnees.Joueur, error) {
	j, err := lireJoueur(b.pool.QueryRow(ctx,
		"select "+colonnesJoueur+" from joueurs j where j.pseudo = $2::text::citext", classement.SeuilClassement, pseudo))
	if err != nil && !errors.Is(err, donnees.ErrIntrouvable) {
		return j, fmt.Errorf("lecture du joueur %q : %w", pseudo, err)
	}
	return j, err
}

func (b *Base) joueurParIdentite(ctx context.Context, id donnees.Identite) (donnees.Joueur, error) {
	return lireJoueur(b.pool.QueryRow(ctx,
		`select `+colonnesJoueur+` from joueurs j join identites i on i.joueur_id = j.id
		where i.fournisseur = $2 and i.sujet = $3`, classement.SeuilClassement, id.Fournisseur, id.Sujet))
}

const essaisPseudo = 20

func (b *Base) TrouverOuCreer(ctx context.Context, id donnees.Identite) (donnees.Joueur, bool, error) {
	j, err := b.retrouver(ctx, id)
	if err == nil {
		return j, false, nil
	}
	if !errors.Is(err, donnees.ErrIntrouvable) {
		return j, false, err
	}

	base := donnees.SuggererPseudo(id.Nom)
	for essai := range essaisPseudo {
		pseudo := base
		if essai > 0 {
			pseudo = pseudoAvecSuffixe(base, rand.IntN(10000)) //nolint:gosec // suffixe de pseudo, sans enjeu de sécurité.
		}
		j, err := b.creer(ctx, id, pseudo)
		contrainte, unicite := violationUnicite(err)
		switch {
		case err == nil:
			return j, true, nil
		case unicite && contrainte == "identites_pkey":
			// Création concurrente de la même identité : l'autre transaction l'a emporté.
			j, err := b.retrouver(ctx, id)
			return j, false, err
		case unicite && contrainte == "joueurs_pseudo_key":
			continue
		default:
			return j, false, fmt.Errorf("création du joueur : %w", err)
		}
	}
	return donnees.Joueur{}, false, fmt.Errorf("aucun pseudo libre trouvé à partir de %q", base)
}

func (b *Base) retrouver(ctx context.Context, id donnees.Identite) (donnees.Joueur, error) {
	j, err := b.joueurParIdentite(ctx, id)
	if err != nil {
		if errors.Is(err, donnees.ErrIntrouvable) {
			return j, err
		}
		return j, fmt.Errorf("recherche de l'identité %s : %w", id.Fournisseur, err)
	}
	if _, err := b.pool.Exec(ctx, "update joueurs set vu_le = now() where id = $1", j.ID); err != nil {
		return j, fmt.Errorf("mise à jour de la dernière visite : %w", err)
	}
	return j, nil
}

func (b *Base) creer(ctx context.Context, id donnees.Identite, pseudo string) (donnees.Joueur, error) {
	var j donnees.Joueur
	err := pgx.BeginFunc(ctx, b.pool, func(tx pgx.Tx) error {
		nouveau := uuid.New()
		_, err := tx.Exec(ctx,
			"insert into joueurs (id, pseudo, elo, elo_max) values ($1, $2::text::citext, $3, $3)",
			nouveau, pseudo, classement.EloInitial)
		if err != nil {
			return err
		}
		var email *string
		if id.Email != "" {
			email = &id.Email
		}
		_, err = tx.Exec(ctx,
			"insert into identites (fournisseur, sujet, joueur_id, email) values ($1, $2, $3, $4)",
			id.Fournisseur, id.Sujet, nouveau, email)
		if err != nil {
			return err
		}
		j, err = lireJoueur(tx.QueryRow(ctx, "select "+colonnesJoueur+" from joueurs j where j.id = $2", classement.SeuilClassement, nouveau))
		return err
	})
	return j, err
}

// pseudoAvecSuffixe ajoute quatre chiffres en restant dans la longueur maximale d'un pseudo.
func pseudoAvecSuffixe(base string, n int) string {
	runes := []rune(base)
	if len(runes) > donnees.PseudoMax-4 {
		runes = runes[:donnees.PseudoMax-4]
	}
	return fmt.Sprintf("%s%04d", string(runes), n%10000)
}

func (b *Base) Renommer(ctx context.Context, id uuid.UUID, pseudo string) (donnees.Joueur, error) {
	if err := donnees.ValiderPseudo(pseudo); err != nil {
		return donnees.Joueur{}, err
	}
	tag, err := b.pool.Exec(ctx, "update joueurs set pseudo = $2::text::citext where id = $1", id, pseudo)
	if _, unicite := violationUnicite(err); unicite {
		return donnees.Joueur{}, donnees.ErrPseudoPris
	}
	if err != nil {
		return donnees.Joueur{}, fmt.Errorf("renommage du joueur %s : %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return donnees.Joueur{}, donnees.ErrIntrouvable
	}
	return b.JoueurParID(ctx, id)
}
