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
	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
	"github.com/felix-marchenay/coinchenet-core/internal/donnees"
	"github.com/felix-marchenay/coinchenet-core/internal/table"
)

// ErrSequenceExistante signale qu'une autre instance a déjà écrit cet événement :
// la table locale ne doit pas continuer.
var ErrSequenceExistante = errors.New("séquence d'événements déjà écrite")

func (b *Base) Ajouter(ctx context.Context, partie uuid.UUID, evts []table.Evenement) error {
	if len(evts) == 0 {
		return nil
	}
	return pgx.BeginFunc(ctx, b.pool, func(tx pgx.Tx) error {
		return insererEvenements(ctx, tx, partie, evts)
	})
}

func insererEvenements(ctx context.Context, tx pgx.Tx, partie uuid.UUID, evts []table.Evenement) error {
	for _, e := range evts {
		_, err := tx.Exec(ctx,
			"insert into evenements (partie_id, sequence, type, donnees, horodatage) values ($1, $2, $3, $4, $5)",
			partie, e.Sequence, string(e.Type), []byte(e.Donnees), e.Horodatage)
		if contrainte, ok := violationUnicite(err); ok && contrainte == "evenements_pkey" {
			return fmt.Errorf("%w : partie %s, séquence %d", ErrSequenceExistante, partie, e.Sequence)
		}
		if err != nil {
			return fmt.Errorf("écriture de l'événement %d de la partie %s : %w", e.Sequence, partie, err)
		}
	}
	return nil
}

func (b *Base) Charger(ctx context.Context, partie uuid.UUID) ([]table.Evenement, error) {
	lignes, err := b.pool.Query(ctx,
		"select sequence, type, donnees, horodatage from evenements where partie_id = $1 order by sequence", partie)
	if err != nil {
		return nil, fmt.Errorf("lecture du journal de la partie %s : %w", partie, err)
	}
	evts, err := pgx.CollectRows(lignes, func(l pgx.CollectableRow) (table.Evenement, error) {
		var e table.Evenement
		var typ string
		var donnees []byte
		if err := l.Scan(&e.Sequence, &typ, &donnees, &e.Horodatage); err != nil {
			return e, err
		}
		e.Type = table.TypeEvenement(typ)
		e.Donnees = json.RawMessage(donnees)
		e.Horodatage = e.Horodatage.UTC()
		return e, nil
	})
	if err != nil {
		return nil, fmt.Errorf("lecture du journal de la partie %s : %w", partie, err)
	}
	return evts, nil
}

func (b *Base) Commencer(ctx context.Context, d table.Debut) error {
	options, err := json.Marshal(d.Options)
	if err != nil {
		return fmt.Errorf("sérialisation des options : %w", err)
	}
	err = pgx.BeginFunc(ctx, b.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			"insert into parties (id, type, options, statut, debut) values ($1, $2, $3, 'en_cours', $4)",
			d.PartieID, string(d.Type), options, d.Date)
		if err != nil {
			return err
		}
		for s, si := range d.Sieges {
			var joueur *uuid.UUID
			pseudo := "Bot"
			if si.Joueur != nil {
				pseudo = si.Joueur.Pseudo
				if !si.Bot {
					id := si.Joueur.ID
					joueur = &id
				}
			}
			_, err := tx.Exec(ctx,
				"insert into participants (partie_id, siege, joueur_id, pseudo, bot) values ($1, $2, $3, $4, $5)",
				d.PartieID, s, joueur, pseudo, si.Bot)
			if err != nil {
				return err
			}
		}
		return insererEvenements(ctx, tx, d.PartieID, []table.Evenement{d.Evenement})
	})
	if err != nil {
		return fmt.Errorf("enregistrement du début de la partie %s : %w", d.PartieID, err)
	}
	return nil
}

func (b *Base) Terminer(ctx context.Context, f table.Fin) ([]classement.Variation, error) {
	var variations []classement.Variation
	err := pgx.BeginFunc(ctx, b.pool, func(tx pgx.Tx) error {
		var statut, typ string
		err := tx.QueryRow(ctx, "select statut, type from parties where id = $1 for update", f.PartieID).Scan(&statut, &typ)
		if errors.Is(err, pgx.ErrNoRows) {
			return donnees.ErrIntrouvable
		}
		if err != nil {
			return err
		}
		if statut != "en_cours" {
			variations, err = variationsEnregistrees(ctx, tx, f.PartieID, typ)
			return err
		}

		if err := insererEvenements(ctx, tx, f.PartieID, []table.Evenement{f.Evenement}); err != nil {
			return err
		}
		if table.Type(typ) == table.Classee {
			if variations, err = appliquerElo(ctx, tx, f); err != nil {
				return err
			}
		} else {
			for s, sf := range f.Sieges {
				if !sf.Abandon {
					continue
				}
				if _, err := tx.Exec(ctx, "update participants set abandon = true where partie_id = $1 and siege = $2", f.PartieID, s); err != nil {
					return err
				}
			}
		}

		_, err = tx.Exec(ctx, `update parties
			set statut = 'terminee', cause = $2, equipe_gagnante = $3, score_a = $4, score_b = $5, fin = $6
			where id = $1`,
			f.PartieID, string(f.Cause), int(f.Gagnante), f.Scores[coinche.EquipeA], f.Scores[coinche.EquipeB], f.Date)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("enregistrement de la fin de la partie %s : %w", f.PartieID, err)
	}
	return variations, nil
}

type humainFinal struct {
	siege   int
	id      uuid.UUID
	abandon bool
	elo     int
	parties int
}

// appliquerElo calcule les variations à partir de l'ELO actuel en base, verrouillé pendant la transaction.
func appliquerElo(ctx context.Context, tx pgx.Tx, f table.Fin) ([]classement.Variation, error) {
	var humains []*humainFinal
	for s, sf := range f.Sieges {
		if sf.Bot || sf.Joueur == nil {
			continue
		}
		humains = append(humains, &humainFinal{siege: s, id: sf.Joueur.ID, abandon: sf.Abandon, elo: sf.Joueur.Elo, parties: classement.SeuilConfirme})
	}
	if len(humains) == 0 {
		return nil, nil
	}

	ids := make([]uuid.UUID, len(humains))
	for i, h := range humains {
		ids[i] = h.id
	}

	// Le tri par id fixe l'ordre de verrouillage et évite les interblocages entre parties.
	lignes, err := tx.Query(ctx,
		"select id, elo, nb_parties_classees from joueurs where id = any($1) order by id for update", ids)
	if err != nil {
		return nil, err
	}
	presents := map[uuid.UUID]bool{}
	for lignes.Next() {
		var id uuid.UUID
		var elo, nb int
		if err := lignes.Scan(&id, &elo, &nb); err != nil {
			lignes.Close()
			return nil, err
		}
		for _, h := range humains {
			if h.id == id {
				h.elo, h.parties = elo, nb
			}
		}
		presents[id] = true
	}
	lignes.Close()
	if err := lignes.Err(); err != nil {
		return nil, err
	}

	participants := make([]classement.Participant, len(humains))
	for i, h := range humains {
		participants[i] = classement.Participant{
			JoueurID:  h.id,
			Equipe:    coinche.Siege(h.siege).Equipe(),
			Elo:       h.elo,
			NbParties: h.parties,
			Abandon:   h.abandon,
		}
	}
	variations := classement.Calculer(participants, f.Gagnante)

	for i, h := range humains {
		v := variations[i]
		if presents[h.id] {
			victoire := 0
			if coinche.Siege(h.siege).Equipe() == f.Gagnante {
				victoire = 1
			}
			_, err := tx.Exec(ctx, `update joueurs
				set elo = $2, elo_max = greatest(elo_max, $2),
					nb_parties_classees = nb_parties_classees + 1,
					nb_victoires_classees = nb_victoires_classees + $3
				where id = $1`, h.id, v.Apres, victoire)
			if err != nil {
				return nil, err
			}
		}
		_, err := tx.Exec(ctx,
			"update participants set elo_avant = $3, elo_apres = $4, abandon = $5 where partie_id = $1 and siege = $2",
			f.PartieID, h.siege, v.Avant, v.Apres, h.abandon)
		if err != nil {
			return nil, err
		}
	}
	return variations, nil
}

func variationsEnregistrees(ctx context.Context, tx pgx.Tx, partie uuid.UUID, typ string) ([]classement.Variation, error) {
	if table.Type(typ) != table.Classee {
		return nil, nil
	}
	lignes, err := tx.Query(ctx, `select joueur_id, elo_avant, elo_apres from participants
		where partie_id = $1 and joueur_id is not null and elo_avant is not null and elo_apres is not null
		order by siege`, partie)
	if err != nil {
		return nil, err
	}
	vs, err := pgx.CollectRows(lignes, func(l pgx.CollectableRow) (classement.Variation, error) {
		var v classement.Variation
		err := l.Scan(&v.JoueurID, &v.Avant, &v.Apres)
		v.Delta = v.Apres - v.Avant
		return v, err
	})
	if err != nil || len(vs) == 0 {
		return nil, err
	}
	return vs, nil
}

func (b *Base) PartiesEnCours(ctx context.Context) ([]uuid.UUID, error) {
	lignes, err := b.pool.Query(ctx, "select id from parties where statut = 'en_cours' order by debut")
	if err != nil {
		return nil, fmt.Errorf("liste des parties en cours : %w", err)
	}
	ids, err := pgx.CollectRows(lignes, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, fmt.Errorf("liste des parties en cours : %w", err)
	}
	return ids, nil
}

func (b *Base) Annuler(ctx context.Context, partie uuid.UUID) error {
	_, err := b.pool.Exec(ctx, "update parties set statut = 'annulee', fin = $2 where id = $1 and statut = 'en_cours'", partie, time.Now())
	if err != nil {
		return fmt.Errorf("annulation de la partie %s : %w", partie, err)
	}
	return nil
}
