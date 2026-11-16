package table

import (
	"context"
	"time"

	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
	"github.com/felix-marchenay/coinchenet-core/internal/protocole"
)

func (t *Table) executerJeu(ctx context.Context, c Commande) error {
	s, assis := t.siegeDe(c.Joueur.ID)
	if !assis || t.sieges[s].abandon {
		return ErrNonAssis
	}
	o := t.sieges[s]

	switch c.Type {
	case CmdEnchere:
		d, err := decoder[DonneesEnchere](c)
		if err != nil {
			return err
		}
		t.actionHumaine(ctx, s)
		return t.encherir(ctx, s, d.Enchere())

	case CmdJouer:
		d, err := decoder[DonneesJouer](c)
		if err != nil {
			return err
		}
		t.actionHumaine(ctx, s)
		return t.jouer(ctx, s, d.Carte)

	case CmdPresent:
		if o.absent {
			o.absent = false
			o.delaisExpires = 0
			t.diffuserStatut(ctx, s)
			if tour, _, ok := t.partie.Tour(); ok && tour == s {
				t.planifier(ctx)
			}
		}
		return nil

	case CmdAbandonner:
		return t.abandonner(ctx, s)

	case CmdSiege, CmdBot, CmdLiberer, CmdLancer, CmdQuitter:
		return ErrPartieDejaLancee

	default:
		return ErrCommandeInconnue
	}
}

// actionHumaine : un joueur qui agit lui-même n'est plus considéré comme absent. Elle précède
// l'action, pour que la planification qui suit ne confie pas son prochain tour au bot.
func (t *Table) actionHumaine(ctx context.Context, s coinche.Siege) {
	o := t.sieges[s]
	o.delaisExpires = 0
	if o.absent {
		o.absent = false
		t.diffuserStatut(ctx, s)
	}
}

func (t *Table) traiterInterneJeu(ctx context.Context, i interne) {
	switch i.typ {
	case interneDonne:
		if t.partie.AttendDonne() {
			t.donner(ctx)
		}
	case interneBot:
		t.jouerAutomatiquement(ctx, i.siege)
	case interneDelai:
		o := t.sieges[i.siege]
		o.delaisExpires++
		if o.delaisExpires >= t.cfg.DelaisAvantAbsence && !o.absent {
			o.absent = true
			t.diffuserStatut(ctx, i.siege)
		}
		t.jouerAutomatiquement(ctx, i.siege)
	}
}

func (t *Table) donner(ctx context.Context) {
	paquet := t.partie.PaquetSuivant(t.deps.Melangeur, t.deps.Coupeur)
	m, err := t.partie.Distribuer(paquet)
	if err != nil {
		t.panne(err)
		return
	}
	evt, err := t.evenement(EvtDonne, DonneDistribuee{Mene: m.Numero, Donneur: m.Donneur, Paquet: paquet.Cartes})
	if err != nil {
		t.panne(err)
		return
	}
	if t.persister(ctx, evt) != nil {
		return
	}

	for s, o := range t.sieges {
		if o.humain() && !o.abandon {
			t.envoyer(ctx, o.joueur.ID, protocole.Message{Type: MsgDonne, Data: MessageDonne{
				Entete:  Entete{Table: t.id, Seq: evt.Sequence},
				Mene:    m.Numero,
				Donneur: m.Donneur,
				Main:    append([]coinche.Carte{}, m.Mains[s].Cartes...),
			}})
		}
	}
	t.planifier(ctx)
}

func (t *Table) encherir(ctx context.Context, s coinche.Siege, e coinche.Enchere) error {
	issue, err := t.partie.Encherir(s, e)
	if err != nil {
		return err
	}
	m := t.partie.MeneCourante()

	sources := []any{m.Encheres[len(m.Encheres)-1]}
	types := []TypeEvenement{EvtEnchere}
	if issue.EncheresTerminees {
		sources = append(sources, ContratFixe{Contrat: *m.Contrat, Preneur: m.Preneur, Multiplicateur: m.Multiplicateur})
		types = append(types, EvtContrat)
	}
	if issue.Redistribution {
		sources = append(sources, Redistribution{Mene: m.Numero})
		types = append(types, EvtRedistribution)
	}
	return t.enregistrerEtDiffuser(ctx, types, sources)
}

func (t *Table) jouer(ctx context.Context, s coinche.Siege, c coinche.Carte) error {
	issue, err := t.partie.Jouer(s, c)
	if err != nil {
		return err
	}
	m := t.partie.MeneCourante()

	sources := []any{CarteJouee{Siege: s, Carte: c, Belote: issue.Belote}}
	types := []TypeEvenement{EvtCarte}
	if issue.Pli != nil {
		sources = append(sources, PliRemporte{Pli: *issue.Pli, Points: issue.Pli.Score(m.Contrat.Atout)})
		types = append(types, EvtPli)
	}
	if issue.Mene != nil {
		sources = append(sources, MeneTerminee{Mene: m.Numero, Resultat: *issue.Mene, Scores: t.partie.Scores})
		types = append(types, EvtMene)
	}
	return t.enregistrerEtDiffuser(ctx, types, sources)
}

func (t *Table) enregistrerEtDiffuser(ctx context.Context, types []TypeEvenement, donnees []any) error {
	evts := make([]Evenement, 0, len(types))
	for i, typ := range types {
		evt, err := t.evenement(typ, donnees[i])
		if err != nil {
			t.panne(err)
			return ErrStockageIndisponible
		}
		evts = append(evts, evt)
	}
	if err := t.persister(ctx, evts...); err != nil {
		return err
	}
	t.diffuserEvenements(ctx, evts)
	t.planifier(ctx)
	return nil
}

// jouerAutomatiquement fait agir le bot ; une décision illégale (qui ne devrait pas arriver)
// est remplacée par la première action autorisée.
func (t *Table) jouerAutomatiquement(ctx context.Context, s coinche.Siege) {
	tour, phase, ok := t.partie.Tour()
	if !ok || tour != s {
		return
	}
	v := t.partie.Vue(s)

	var err error
	switch phase {
	case coinche.PhaseEncheres:
		if err = t.encherir(ctx, s, t.deps.Decideur.Encherir(v)); err != nil && !t.ferme {
			t.log.Warn("enchère du bot refusée", "siege", s, "erreur", err)
			err = t.encherir(ctx, s, coinche.NouvellePasse())
		}
	case coinche.PhaseJeu:
		if err = t.jouer(ctx, s, t.deps.Decideur.Jouer(v)); err != nil && !t.ferme {
			t.log.Warn("carte du bot refusée", "siege", s, "erreur", err)
			err = t.jouer(ctx, s, v.Mene.CartesJouables[0])
		}
	}
	if err != nil {
		t.log.Error("action automatique impossible", "siege", s, "erreur", err)
	}
}

// planifier détermine la prochaine étape : donne, tour d'un joueur ou fin de partie.
// Le jeton invalide tous les délais programmés auparavant.
func (t *Table) planifier(ctx context.Context) {
	t.jeton++
	t.echeance = time.Time{}
	if t.statut != StatutEnCours || t.ferme {
		return
	}
	if t.partie.Terminee {
		cause := CauseScore
		if t.partie.Forfait {
			cause = CauseForfait
		}
		t.terminer(ctx, cause)
		return
	}
	if t.partie.AttendDonne() {
		delai := t.cfg.DelaiEntreMenes
		if len(t.partie.Menes) == 0 {
			delai = t.cfg.DelaiDemarrage
		}
		t.apres(delai, interne{typ: interneDonne, jeton: t.jeton})
		return
	}

	s, phase, _ := t.partie.Tour()
	o := t.sieges[s]
	switch {
	case o.automatique():
		t.apres(t.cfg.DelaiBot, interne{typ: interneBot, jeton: t.jeton, siege: s})
	case t.options.DelaiAction > 0:
		t.echeance = t.maintenant().Add(t.options.Delai())
		t.apres(t.options.Delai(), interne{typ: interneDelai, jeton: t.jeton, siege: s})
	}
	t.diffuserTour(ctx, s, phase)
}

func (t *Table) diffuserTour(ctx context.Context, s coinche.Siege, phase coinche.Phase) {
	base := MessageTour{Entete: Entete{Table: t.id, Seq: t.seq}, Siege: s, Phase: phase}
	if !t.echeance.IsZero() {
		e := t.echeance
		base.Echeance = &e
	}
	for siege, o := range t.sieges {
		if !o.humain() || o.abandon {
			continue
		}
		m := base
		if coinche.Siege(siege) == s {
			m.CartesJouables = t.partie.CartesJouables(s)
			if opts, ok := t.partie.OptionsEnchere(s); ok {
				m.OptionsEnchere = &opts
			}
		}
		t.envoyer(ctx, o.joueur.ID, protocole.Message{Type: MsgTour, Data: m})
	}
}

// abandonner : forfait immédiat en partie classée ; en partie privée le bot reprend le siège.
func (t *Table) abandonner(ctx context.Context, s coinche.Siege) error {
	o := t.sieges[s]
	evt, err := t.evenement(EvtAbandon, Abandon{Siege: s})
	if err != nil {
		return err
	}
	if err := t.persister(ctx, evt); err != nil {
		return err
	}
	o.abandon = true
	t.diffuserStatut(ctx, s)
	t.log.Info("abandon", "siege", s, "joueur", o.joueur.ID)

	if t.typ == Classee {
		t.partie.Abandonner(s.Equipe())
		t.terminer(ctx, CauseForfait)
		return nil
	}

	t.dissocier(ctx, o)
	if len(t.joueursRestants()) == 0 {
		t.log.Info("plus aucun joueur humain, la partie est arrêtée")
		t.partie.Abandonner(s.Equipe())
		t.terminer(ctx, CauseForfait)
		return nil
	}
	if tour, _, ok := t.partie.Tour(); ok && tour == s {
		t.planifier(ctx)
	}
	return nil
}

func (t *Table) joueursRestants() []*occupant {
	var res []*occupant
	for _, o := range t.humains() {
		if !o.abandon {
			res = append(res, o)
		}
	}
	return res
}

func (t *Table) terminer(ctx context.Context, cause Cause) {
	t.jeton++
	t.echeance = time.Time{}
	fin := FinPartie{Gagnante: t.partie.Gagnante, Cause: cause, Scores: t.partie.Scores}
	evt, err := t.evenement(EvtPartieTerminee, fin)
	if err != nil {
		t.panne(err)
		return
	}

	archive := Fin{
		PartieID:  t.id,
		Type:      t.typ,
		Gagnante:  fin.Gagnante,
		Cause:     cause,
		Scores:    fin.Scores,
		Evenement: evt,
		Date:      t.maintenant(),
	}
	for s, o := range t.sieges {
		sf := SiegeFinal{Bot: o.bot, Abandon: o.abandon}
		if o.humain() {
			j := o.joueur
			sf.Joueur = &j
		}
		archive.Sieges[s] = sf
	}

	stockage, annuler := t.contexteStockage(ctx)
	defer annuler()
	variations, err := t.deps.Archiviste.Terminer(stockage, archive)
	if err != nil {
		t.panne(err)
		return
	}
	fin.Elo = variations
	t.fin = &fin
	t.statut = StatutTerminee
	t.log.Info("partie terminée", "gagnante", fin.Gagnante, "cause", cause, "scores", fin.Scores)
	if t.deps.Metriques != nil {
		t.deps.Metriques.PartieTerminee(t.typ, cause)
	}

	m := protocole.Message{Type: MsgTerminee, Data: struct {
		Entete
		FinPartie
	}{Entete{Table: t.id, Seq: evt.Sequence}, fin}}
	for _, o := range t.humains() {
		t.envoyer(ctx, o.joueur.ID, m)
		t.dissocier(ctx, o)
	}
	t.apres(t.cfg.DelaiFermeture, interne{typ: interneFermeture})
}
