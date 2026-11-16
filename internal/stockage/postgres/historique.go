package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/felix-marchenay/coinchenet-core/internal/classement"
	"github.com/felix-marchenay/coinchenet-core/internal/donnees"
)

const (
	TailleClassementParDefaut = 50
	TailleClassementMax       = 100
)

func (b *Base) Classement(ctx context.Context, page, taille int) (donnees.PageClassement, error) {
	page = max(page, 1)
	if taille < 1 {
		taille = TailleClassementParDefaut
	}
	taille = min(taille, TailleClassementMax)
	res := donnees.PageClassement{Page: page, Taille: taille, Lignes: []donnees.LigneClassement{}}

	err := b.pool.QueryRow(ctx, "select count(*) from joueurs where nb_parties_classees >= $1", classement.SeuilClassement).Scan(&res.Total)
	if err != nil {
		return res, fmt.Errorf("comptage du classement : %w", err)
	}

	lignes, err := b.pool.Query(ctx, `select rank() over (order by elo desc), id, pseudo::text, elo, nb_parties_classees, nb_victoires_classees
		from joueurs where nb_parties_classees >= $1
		order by elo desc, pseudo, id
		limit $2 offset $3`, classement.SeuilClassement, taille, (page-1)*taille)
	if err != nil {
		return res, fmt.Errorf("lecture du classement : %w", err)
	}
	res.Lignes, err = pgx.CollectRows(lignes, func(l pgx.CollectableRow) (donnees.LigneClassement, error) {
		var lc donnees.LigneClassement
		err := l.Scan(&lc.Rang, &lc.ID, &lc.Pseudo, &lc.Elo, &lc.NbParties, &lc.NbVictoires)
		return lc, err
	})
	if err != nil {
		return res, fmt.Errorf("lecture du classement : %w", err)
	}
	if res.Lignes == nil {
		res.Lignes = []donnees.LigneClassement{}
	}
	return res, nil
}

const colonnesPartie = "p.id, p.type, p.statut, p.cause, p.equipe_gagnante, p.score_a, p.score_b, p.options, p.debut, p.fin"

func lirePartie(l pgx.Row) (donnees.ResumePartie, error) {
	var r donnees.ResumePartie
	var scoreA, scoreB *int
	var options []byte
	err := l.Scan(&r.ID, &r.Type, &r.Statut, &r.Cause, &r.EquipeGagnante, &scoreA, &scoreB, &options, &r.Debut, &r.Fin)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, donnees.ErrIntrouvable
	}
	if err != nil {
		return r, err
	}
	if scoreA != nil && scoreB != nil {
		r.Scores = &[2]int{*scoreA, *scoreB}
	}
	r.Options = json.RawMessage(options)
	r.Debut = r.Debut.UTC()
	if r.Fin != nil {
		fin := r.Fin.UTC()
		r.Fin = &fin
	}
	r.Participants = []donnees.Participant{}
	return r, nil
}

func (b *Base) Partie(ctx context.Context, id uuid.UUID) (donnees.ResumePartie, error) {
	r, err := lirePartie(b.pool.QueryRow(ctx, "select "+colonnesPartie+" from parties p where p.id = $1", id))
	if errors.Is(err, donnees.ErrIntrouvable) {
		return r, err
	}
	if err != nil {
		return r, fmt.Errorf("lecture de la partie %s : %w", id, err)
	}
	parties := []*donnees.ResumePartie{&r}
	if err := b.completerParticipants(ctx, parties); err != nil {
		return r, err
	}
	return r, nil
}

func (b *Base) PartiesDuJoueur(ctx context.Context, joueur uuid.UUID, avant *time.Time, limite int) ([]donnees.ResumePartie, error) {
	if limite < 1 || limite > 100 {
		limite = 20
	}
	lignes, err := b.pool.Query(ctx, "select "+colonnesPartie+` from parties p
		join participants pa on pa.partie_id = p.id
		where pa.joueur_id = $1 and p.statut <> 'en_cours' and ($2::timestamptz is null or p.debut < $2)
		order by p.debut desc, p.id desc
		limit $3`, joueur, avant, limite)
	if err != nil {
		return nil, fmt.Errorf("historique du joueur %s : %w", joueur, err)
	}
	res, err := pgx.CollectRows(lignes, func(l pgx.CollectableRow) (donnees.ResumePartie, error) {
		return lirePartie(l)
	})
	if err != nil {
		return nil, fmt.Errorf("historique du joueur %s : %w", joueur, err)
	}
	parties := make([]*donnees.ResumePartie, len(res))
	for i := range res {
		parties[i] = &res[i]
	}
	if err := b.completerParticipants(ctx, parties); err != nil {
		return nil, err
	}
	if res == nil {
		res = []donnees.ResumePartie{}
	}
	return res, nil
}

func (b *Base) completerParticipants(ctx context.Context, parties []*donnees.ResumePartie) error {
	if len(parties) == 0 {
		return nil
	}
	index := make(map[uuid.UUID]*donnees.ResumePartie, len(parties))
	ids := make([]uuid.UUID, 0, len(parties))
	for _, p := range parties {
		index[p.ID] = p
		ids = append(ids, p.ID)
	}
	lignes, err := b.pool.Query(ctx, `select partie_id, siege, joueur_id, pseudo, bot, elo_avant, elo_apres, abandon
		from participants where partie_id = any($1) order by partie_id, siege`, ids)
	if err != nil {
		return fmt.Errorf("lecture des participants : %w", err)
	}
	var partie uuid.UUID
	var p donnees.Participant
	_, err = pgx.ForEachRow(lignes, []any{&partie, &p.Siege, &p.JoueurID, &p.Pseudo, &p.Bot, &p.EloAvant, &p.EloApres, &p.Abandon}, func() error {
		r := index[partie]
		r.Participants = append(r.Participants, p)
		p = donnees.Participant{}
		return nil
	})
	if err != nil {
		return fmt.Errorf("lecture des participants : %w", err)
	}
	return nil
}

func (b *Base) EvolutionElo(ctx context.Context, joueur uuid.UUID, limite int) ([]donnees.PointElo, error) {
	if limite < 1 || limite > 1000 {
		limite = 100
	}
	lignes, err := b.pool.Query(ctx, `select id, fin, elo_apres, elo_apres - elo_avant from (
			select p.id, p.fin, pa.elo_apres, pa.elo_avant
			from participants pa join parties p on p.id = pa.partie_id
			where pa.joueur_id = $1 and p.type = 'classee' and p.statut = 'terminee'
				and pa.elo_avant is not null and pa.elo_apres is not null
			order by p.fin desc, p.id desc
			limit $2
		) recents order by fin, id`, joueur, limite)
	if err != nil {
		return nil, fmt.Errorf("évolution de l'ELO du joueur %s : %w", joueur, err)
	}
	points, err := pgx.CollectRows(lignes, func(l pgx.CollectableRow) (donnees.PointElo, error) {
		var pt donnees.PointElo
		err := l.Scan(&pt.Partie, &pt.Date, &pt.Elo, &pt.Delta)
		pt.Date = pt.Date.UTC()
		return pt, err
	})
	if err != nil {
		return nil, fmt.Errorf("évolution de l'ELO du joueur %s : %w", joueur, err)
	}
	if points == nil {
		points = []donnees.PointElo{}
	}
	return points, nil
}
