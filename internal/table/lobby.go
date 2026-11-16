package table

import (
	"context"
	"fmt"

	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
	"github.com/felix-marchenay/coinchenet-core/internal/protocole"
)

func (t *Table) executerLobby(ctx context.Context, c Commande) error {
	s, assis := t.siegeDe(c.Joueur.ID)
	if !assis {
		return ErrNonAssis
	}

	switch c.Type {
	case CmdSiege:
		d, err := decoder[DonneesSiege](c)
		if err != nil {
			return err
		}
		if !d.Siege.Valide() {
			return ErrSiegeInvalide
		}
		if t.sieges[d.Siege] != nil {
			return ErrSiegeOccupe
		}
		t.sieges[d.Siege], t.sieges[s] = t.sieges[s], nil

	case CmdBot:
		if c.Joueur.ID != t.createur {
			return ErrReserveAuCreateur
		}
		d, err := decoder[DonneesBot](c)
		if err != nil {
			return err
		}
		if !d.Siege.Valide() {
			return ErrSiegeInvalide
		}
		if t.sieges[d.Siege] != nil {
			return ErrSiegeOccupe
		}
		profil, nom, err := t.choisirProfil(d.Profil)
		if err != nil {
			return err
		}
		t.sieges[d.Siege] = &occupant{bot: true, profil: profil, joueur: Joueur{Pseudo: nom}}

	case CmdLiberer:
		if c.Joueur.ID != t.createur {
			return ErrReserveAuCreateur
		}
		d, err := decoder[DonneesSiege](c)
		if err != nil {
			return err
		}
		if !d.Siege.Valide() || d.Siege == s {
			return ErrSiegeInvalide
		}
		o := t.sieges[d.Siege]
		if o == nil {
			return ErrSiegeLibre
		}
		t.sieges[d.Siege] = nil
		if o.humain() {
			t.exclus[o.joueur.ID] = true
			t.dissocier(ctx, o)
			t.envoyer(ctx, o.joueur.ID, protocole.Message{Type: MsgExclu, Data: MessageFermee{Table: t.id, Raison: "exclu"}})
		}

	case CmdQuitter:
		o := t.sieges[s]
		t.sieges[s] = nil
		t.dissocier(ctx, o)
		t.envoyer(ctx, o.joueur.ID, protocole.Message{Type: MsgFermee, Data: MessageFermee{Table: t.id, Raison: "quittee"}})
		if c.Joueur.ID == t.createur {
			restants := t.humains()
			if len(restants) == 0 {
				t.fermerLobby(ctx, "abandonnee")
				return nil
			}
			t.createur = restants[0].joueur.ID
		}

	case CmdLancer:
		if c.Joueur.ID != t.createur {
			return ErrReserveAuCreateur
		}
		if _, libre := t.siegeLibre(); libre {
			return ErrTableIncomplete
		}
		return t.commencer(ctx)

	case CmdEnchere, CmdJouer, CmdAbandonner, CmdPresent:
		return ErrPartieNonCommencee

	default:
		return ErrCommandeInconnue
	}

	t.diffuserEtat(ctx)
	return nil
}

func (t *Table) fermerLobby(ctx context.Context, raison string) {
	m := protocole.Message{Type: MsgFermee, Data: MessageFermee{Table: t.id, Raison: raison}}
	for _, o := range t.humains() {
		t.envoyer(ctx, o.joueur.ID, m)
		t.dissocier(ctx, o)
	}
	t.statut = StatutTerminee
	t.ferme = true
}

// commencer fige les sièges, enregistre la partie et programme la première donne.
func (t *Table) commencer(ctx context.Context) error {
	premier := coinche.Siege(t.deps.Hasard.Intn(coinche.NbSieges))
	partie, err := coinche.NouvellePartie(t.options.Regles, premier)
	if err != nil {
		return fmt.Errorf("création de la partie : %w", err)
	}

	creee := PartieCreee{Type: t.typ, Options: t.options, PremierDonneur: premier}
	for s, o := range t.sieges {
		if o.bot {
			creee.Sieges[s] = SiegeInitial{Bot: true, Profil: o.profil, Joueur: &Joueur{Pseudo: o.joueur.Pseudo}}
			continue
		}
		j := o.joueur
		creee.Sieges[s] = SiegeInitial{Joueur: &j}
	}

	evt, err := t.evenement(EvtPartieCreee, creee)
	if err != nil {
		return err
	}
	stockage, annuler := t.contexteStockage(ctx)
	defer annuler()
	err = t.deps.Archiviste.Commencer(stockage, Debut{
		PartieID:  t.id,
		Type:      t.typ,
		Options:   t.options,
		Sieges:    creee.Sieges,
		Evenement: evt,
		Date:      t.maintenant(),
	})
	if err != nil {
		t.seq--
		t.log.Error("enregistrement du début de partie impossible", "erreur", err)
		return ErrStockageIndisponible
	}

	t.partie = partie
	t.statut = StatutEnCours
	t.log.Info("partie commencée", "premier_donneur", premier)

	for s, o := range t.sieges {
		if !o.humain() {
			continue
		}
		t.associer(ctx, o)
		t.envoyer(ctx, o.joueur.ID, protocole.Message{Type: MsgCommencee, Data: MessageCommencee{Table: t.id, Type: t.typ}})
		if !o.connecte() {
			t.lancerDelaiRetour(coinche.Siege(s))
		}
	}
	t.diffuserEtat(ctx)
	t.planifier(ctx)
	return nil
}

// choisirProfil valide le profil demandé, ou prend le premier qui n'est pas déjà assis à la table :
// chaque bot garde ainsi un prénom distinct.
func (t *Table) choisirProfil(demande string) (string, string, error) {
	assis := make(map[string]bool, coinche.NbSieges)
	for _, o := range t.sieges {
		if o != nil && o.bot {
			assis[o.profil] = true
		}
	}
	if demande != "" {
		nom, ok := t.deps.Decideur.Nom(demande)
		if !ok {
			return "", "", ErrProfilInconnu
		}
		if assis[demande] {
			return "", "", ErrProfilPris
		}
		return demande, nom, nil
	}
	for _, p := range t.deps.Decideur.Profils() {
		if !assis[p] {
			nom, _ := t.deps.Decideur.Nom(p)
			return p, nom, nil
		}
	}
	return "", "", ErrProfilPris
}
