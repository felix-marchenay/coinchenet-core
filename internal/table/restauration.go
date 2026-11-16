package table

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
	"github.com/felix-marchenay/coinchenet-core/internal/protocole"
)

// Restaurer reconstruit une table en cours à partir de son journal, en rejouant les événements source.
func Restaurer(id uuid.UUID, evts []Evenement, deps Dependances, cfg Config) (*Table, error) {
	if len(evts) == 0 || evts[0].Type != EvtPartieCreee {
		return nil, &ErrJournalIncoherent{Partie: id, Cause: errors.New("le journal doit commencer par la création de la partie")}
	}

	var t *Table
	precedente := evts[0].Sequence - 1
	for _, e := range evts {
		if e.Sequence != precedente+1 {
			return nil, &ErrJournalIncoherent{Partie: id, Sequence: e.Sequence, Cause: errors.New("séquence discontinue")}
		}
		precedente = e.Sequence
		var err error
		if t == nil {
			t, err = tableDepuisCreation(id, e, deps, cfg)
		} else {
			err = t.rejouer(e)
		}
		if err != nil {
			return nil, &ErrJournalIncoherent{Partie: id, Sequence: e.Sequence, Cause: err}
		}
		t.seq = e.Sequence
	}
	return t, nil
}

func tableDepuisCreation(id uuid.UUID, e Evenement, deps Dependances, cfg Config) (*Table, error) {
	var pc PartieCreee
	if err := e.Decoder(&pc); err != nil {
		return nil, err
	}
	partie, err := coinche.NouvellePartie(pc.Options.Regles, pc.PremierDonneur)
	if err != nil {
		return nil, err
	}
	t := nouvelleTable(id, pc.Type, pc.Options, deps, cfg)
	for s, si := range pc.Sieges {
		o := &occupant{bot: si.Bot, connexions: map[string]struct{}{}}
		if si.Joueur != nil {
			o.joueur = *si.Joueur
		}
		if !o.bot && o.joueur.ID == uuid.Nil {
			return nil, fmt.Errorf("siège %d sans joueur", s)
		}
		t.sieges[s] = o
	}
	t.partie = partie
	t.statut = StatutEnCours
	t.restauree = true
	return t, nil
}

func (t *Table) rejouer(e Evenement) error {
	switch e.Type {
	case EvtDonne:
		var d DonneDistribuee
		if err := e.Decoder(&d); err != nil {
			return err
		}
		m, err := t.partie.Distribuer(coinche.CarteCollection{Cartes: d.Paquet})
		if err == nil && (m.Numero != d.Mene || m.Donneur != d.Donneur) {
			err = fmt.Errorf("donne %d du siège %d attendue, %d du siège %d obtenue", d.Mene, d.Donneur, m.Numero, m.Donneur)
		}
		return err

	case EvtEnchere:
		var en coinche.Enchere
		if err := e.Decoder(&en); err != nil {
			return err
		}
		_, err := t.partie.Encherir(en.Siege, en)
		return err

	case EvtCarte:
		var c CarteJouee
		if err := e.Decoder(&c); err != nil {
			return err
		}
		_, err := t.partie.Jouer(c.Siege, c.Carte)
		return err

	case EvtAbandon:
		var a Abandon
		if err := e.Decoder(&a); err != nil {
			return err
		}
		if !a.Siege.Valide() {
			return fmt.Errorf("siège %d invalide", a.Siege)
		}
		t.sieges[a.Siege].abandon = true
		if t.typ == Classee || len(t.joueursRestants()) == 0 {
			t.partie.Abandonner(a.Siege.Equipe())
		}
		return nil

	case EvtPartieTerminee:
		var f FinPartie
		if err := e.Decoder(&f); err != nil {
			return err
		}
		t.fin = &f
		t.statut = StatutTerminee
		return nil

	case EvtChat:
		var c Chat
		if err := e.Decoder(&c); err != nil {
			return err
		}
		t.memoriserChat(c)
		return nil

	default:
		return nil
	}
}

// reprendre relance une table restaurée : les joueurs sont prévenus pour se reconnecter
// et, en partie classée, disposent du délai de retour habituel.
func (t *Table) reprendre(ctx context.Context) {
	if t.statut == StatutTerminee {
		t.ferme = true
		return
	}
	t.log.Info("table reprise depuis le journal", "seq", t.seq)
	for s, o := range t.sieges {
		if !o.humain() || o.abandon {
			continue
		}
		t.associer(ctx, o)
		t.lancerDelaiRetour(coinche.Siege(s))
		t.envoyer(ctx, o.joueur.ID, protocole.Message{Type: MsgCommencee, Data: MessageCommencee{Table: t.id, Type: t.typ}})
	}
	t.planifier(ctx)
}
