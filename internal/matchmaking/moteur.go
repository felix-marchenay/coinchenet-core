package matchmaking

import (
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/felix-marchenay/coinchenet-core/internal/protocole"
	"github.com/felix-marchenay/coinchenet-core/internal/table"
)

// Messages envoyés aux joueurs.
const (
	MsgRejointe    = "file.rejointe"
	MsgQuittee     = "file.quittee"
	MsgTrouvee     = "file.trouvee"
	MsgAcceptation = "file.acceptation"
	MsgAnnulee     = "file.annulee"
	MsgInvitation  = "duo.invitation"
	MsgEnvoyee     = "duo.envoyee"
	MsgRefusee     = "duo.refusee"
	MsgExpiree     = "duo.expiree"
)

type TypeCommande string

const (
	CmdRejoindre   TypeCommande = "rejoindre"
	CmdQuitter     TypeCommande = "quitter"
	CmdInviter     TypeCommande = "inviter"
	CmdRepondre    TypeCommande = "repondre"
	CmdAccepter    TypeCommande = "accepter"
	CmdRefuser     TypeCommande = "refuser"
	CmdDeconnexion TypeCommande = "deconnexion"
	// CmdEtat renvoie au joueur sa situation dans la file, par exemple à sa reconnexion.
	CmdEtat TypeCommande = "etat"
)

type Commande struct {
	Type        TypeCommande  `json:"type"`
	Joueur      table.Joueur  `json:"joueur"`
	Invite      *table.Joueur `json:"invite,omitempty"`
	Invitation  string        `json:"invitation,omitempty"`
	Proposition string        `json:"proposition,omitempty"`
	Accepte     bool          `json:"accepte,omitempty"`
	Ref         string        `json:"ref,omitempty"`
}

// Proposition : comme dans Appariement, un Joueur zéro est un siège tenu par un bot.
type Proposition struct {
	ID       string             `json:"id"`
	EquipeA  [2]table.Joueur    `json:"equipe_a"`
	EquipeB  [2]table.Joueur    `json:"equipe_b"`
	Tickets  []Ticket           `json:"tickets"`
	Acceptes map[uuid.UUID]bool `json:"acceptes"`
	ExpireLe time.Time          `json:"expire_le"`
}

func (p *Proposition) humains() []table.Joueur {
	var res []table.Joueur
	for _, j := range [...]table.Joueur{p.EquipeA[0], p.EquipeB[0], p.EquipeA[1], p.EquipeB[1]} {
		if j.ID != uuid.Nil {
			res = append(res, j)
		}
	}
	return res
}

func (p *Proposition) contient(id uuid.UUID) bool {
	return slices.ContainsFunc(p.humains(), func(j table.Joueur) bool { return j.ID == id })
}

// acceptes compte les bots, qui acceptent d'office.
func (p *Proposition) acceptes() int {
	return len(p.Acceptes) + 4 - len(p.humains())
}

func (p *Proposition) trouvee() MessageTrouvee {
	return MessageTrouvee{Proposition: p.ID, ExpireLe: p.ExpireLe, EquipeA: equipe(p.EquipeA), EquipeB: equipe(p.EquipeB)}
}

func equipe(e [2]table.Joueur) [2]*table.Joueur {
	var res [2]*table.Joueur
	for i := range e {
		if e[i].ID != uuid.Nil {
			res[i] = &e[i]
		}
	}
	return res
}

type Invitation struct {
	ID       string       `json:"id"`
	De       table.Joueur `json:"de"`
	A        table.Joueur `json:"a"`
	ExpireLe time.Time    `json:"expire_le"`
}

// Etat est entièrement sérialisable, pour qu'une autre instance reprenne la file.
type Etat struct {
	Tickets      map[string]*Ticket      `json:"tickets"`
	Propositions map[string]*Proposition `json:"propositions"`
	Invitations  map[string]*Invitation  `json:"invitations"`
}

func NouvelEtat() Etat {
	return Etat{Tickets: map[string]*Ticket{}, Propositions: map[string]*Proposition{}, Invitations: map[string]*Invitation{}}
}

type Notification struct {
	Joueur  uuid.UUID
	Message protocole.Message
}

type CreationTable struct {
	Proposition string
	EquipeA     [2]table.Joueur
	EquipeB     [2]table.Joueur
	Attentes    []time.Duration
}

type Effets struct {
	Notifications []Notification
	Tables        []CreationTable
	Modifie       bool
}

func (e *Effets) notifier(j uuid.UUID, typ string, data any) {
	e.Notifications = append(e.Notifications, Notification{Joueur: j, Message: protocole.Message{Type: typ, Data: data}})
}

func (e *Effets) erreur(c Commande, code, message string) {
	e.Notifications = append(e.Notifications, Notification{Joueur: c.Joueur.ID, Message: protocole.NouvelleErreur(c.Ref, code, message)})
}

// Moteur applique les commandes de la file ; il n'est utilisé que par l'instance leader.
type Moteur struct {
	Etat       Etat
	Parametres Parametres
	NouvelID   func() string
}

func NouveauMoteur(e Etat, p Parametres) *Moteur {
	if e.Tickets == nil {
		e = NouvelEtat()
	}
	return &Moteur{Etat: e, Parametres: p, NouvelID: uuid.NewString}
}

type MessageRejointe struct {
	Mode       string        `json:"mode"`
	Depuis     time.Time     `json:"depuis"`
	Partenaire *table.Joueur `json:"partenaire,omitempty"`
}

type MessageQuittee struct {
	Raison string `json:"raison"`
}

// MessageTrouvee : un siège nul est tenu par un bot, et la partie n'est alors pas classée.
type MessageTrouvee struct {
	Proposition string           `json:"proposition"`
	ExpireLe    time.Time        `json:"expire_le"`
	EquipeA     [2]*table.Joueur `json:"equipe_a"`
	EquipeB     [2]*table.Joueur `json:"equipe_b"`
}

type MessageAcceptation struct {
	Proposition string `json:"proposition"`
	Acceptes    int    `json:"acceptes"`
}

type MessageAnnulee struct {
	Proposition string `json:"proposition"`
	Raison      string `json:"raison"`
	EnFile      bool   `json:"en_file"`
}

type MessageInvitation struct {
	Invitation string       `json:"invitation"`
	De         table.Joueur `json:"de"`
	A          table.Joueur `json:"a"`
	ExpireLe   time.Time    `json:"expire_le"`
}

func (m *Moteur) Appliquer(c Commande, maintenant time.Time) Effets {
	var e Effets
	switch c.Type {
	case CmdRejoindre:
		if m.occupe(c.Joueur.ID) {
			e.erreur(c, "deja_en_file", "vous êtes déjà dans la file")
			return e
		}
		t := &Ticket{ID: m.NouvelID(), Joueurs: []table.Joueur{c.Joueur}, EntreeLe: maintenant}
		m.Etat.Tickets[t.ID] = t
		m.annulerInvitationsDe(c.Joueur.ID, &e)
		e.notifier(c.Joueur.ID, MsgRejointe, MessageRejointe{Mode: "solo", Depuis: maintenant})
		e.Modifie = true

	case CmdQuitter, CmdDeconnexion:
		raison := "demande"
		if c.Type == CmdDeconnexion {
			raison = "deconnexion"
			m.annulerInvitationsDe(c.Joueur.ID, &e)
		}
		if t := m.ticketDe(c.Joueur.ID); t != nil {
			m.retirerTicket(t, raison, c.Joueur.ID, &e)
		}
		if p := m.propositionDe(c.Joueur.ID); p != nil {
			m.annulerProposition(p, map[uuid.UUID]bool{c.Joueur.ID: true}, "refus", &e)
		}

	case CmdEtat:
		m.etat(c.Joueur, &e)

	case CmdInviter:
		m.inviter(c, maintenant, &e)

	case CmdRepondre:
		m.repondre(c, maintenant, &e)

	case CmdAccepter:
		p := m.Etat.Propositions[c.Proposition]
		if p == nil || !p.contient(c.Joueur.ID) {
			e.erreur(c, protocole.CodeIntrouvable, "proposition introuvable ou expirée")
			return e
		}
		p.Acceptes[c.Joueur.ID] = true
		e.Modifie = true
		if p.acceptes() < 4 {
			for _, j := range p.humains() {
				e.notifier(j.ID, MsgAcceptation, MessageAcceptation{Proposition: p.ID, Acceptes: p.acceptes()})
			}
			return e
		}
		delete(m.Etat.Propositions, p.ID)
		creation := CreationTable{Proposition: p.ID, EquipeA: p.EquipeA, EquipeB: p.EquipeB}
		for _, t := range p.Tickets {
			for range t.Joueurs {
				creation.Attentes = append(creation.Attentes, maintenant.Sub(t.EntreeLe))
			}
		}
		e.Tables = append(e.Tables, creation)

	case CmdRefuser:
		p := m.Etat.Propositions[c.Proposition]
		if p == nil || !p.contient(c.Joueur.ID) {
			e.erreur(c, protocole.CodeIntrouvable, "proposition introuvable ou expirée")
			return e
		}
		m.annulerProposition(p, map[uuid.UUID]bool{c.Joueur.ID: true}, "refus", &e)

	default:
		e.erreur(c, protocole.CodeRequeteInvalide, "commande inconnue")
	}
	return e
}

// Tick expire les propositions et invitations, puis forme de nouvelles tables.
func (m *Moteur) Tick(maintenant time.Time) Effets {
	var e Effets
	for _, p := range m.Etat.Propositions {
		if maintenant.Before(p.ExpireLe) {
			continue
		}
		fautifs := map[uuid.UUID]bool{}
		for _, j := range p.humains() {
			if !p.Acceptes[j.ID] {
				fautifs[j.ID] = true
			}
		}
		m.annulerProposition(p, fautifs, "delai", &e)
	}
	for id, inv := range m.Etat.Invitations {
		if maintenant.Before(inv.ExpireLe) {
			continue
		}
		delete(m.Etat.Invitations, id)
		e.notifier(inv.De.ID, MsgExpiree, MessageInvitation{Invitation: id, De: inv.De, A: inv.A, ExpireLe: inv.ExpireLe})
		e.Modifie = true
	}

	tickets := make([]Ticket, 0, len(m.Etat.Tickets))
	for _, t := range m.Etat.Tickets {
		tickets = append(tickets, *t)
	}
	for _, a := range Apparier(tickets, maintenant, m.Parametres) {
		p := &Proposition{
			ID:       m.NouvelID(),
			EquipeA:  a.EquipeA,
			EquipeB:  a.EquipeB,
			Tickets:  a.Tickets,
			Acceptes: map[uuid.UUID]bool{},
			ExpireLe: maintenant.Add(m.Parametres.DelaiAcceptation),
		}
		for _, t := range a.Tickets {
			delete(m.Etat.Tickets, t.ID)
		}
		m.Etat.Propositions[p.ID] = p
		for _, j := range p.humains() {
			e.notifier(j.ID, MsgTrouvee, p.trouvee())
		}
		e.Modifie = true
	}
	return e
}

// NbEnFile compte les joueurs en attente (hors propositions).
func (m *Moteur) NbEnFile() int {
	n := 0
	for _, t := range m.Etat.Tickets {
		n += len(t.Joueurs)
	}
	return n
}

// TableEchouee remet en file les joueurs d'une proposition dont la table n'a pas pu être créée.
func (m *Moteur) TableEchouee(c CreationTable) Effets {
	var e Effets
	for _, j := range [...]table.Joueur{c.EquipeA[0], c.EquipeA[1], c.EquipeB[0], c.EquipeB[1]} {
		if j.ID != uuid.Nil {
			e.notifier(j.ID, MsgAnnulee, MessageAnnulee{Proposition: c.Proposition, Raison: "erreur", EnFile: false})
		}
	}
	return e
}

func (m *Moteur) etat(j table.Joueur, e *Effets) {
	if t := m.ticketDe(j.ID); t != nil {
		msg := MessageRejointe{Mode: "solo", Depuis: t.EntreeLe}
		for i := range t.Joueurs {
			if t.Joueurs[i].ID != j.ID && t.Duo() {
				msg.Mode = "duo"
				msg.Partenaire = &t.Joueurs[i]
			}
		}
		e.notifier(j.ID, MsgRejointe, msg)
		return
	}
	if p := m.propositionDe(j.ID); p != nil {
		e.notifier(j.ID, MsgTrouvee, p.trouvee())
		e.notifier(j.ID, MsgAcceptation, MessageAcceptation{Proposition: p.ID, Acceptes: p.acceptes()})
	}
}

func (m *Moteur) inviter(c Commande, maintenant time.Time, e *Effets) {
	invite := c.Invite
	switch {
	case invite == nil || invite.ID == c.Joueur.ID:
		e.erreur(c, protocole.CodeRequeteInvalide, "invitation invalide")
		return
	case m.occupe(c.Joueur.ID):
		e.erreur(c, "deja_en_file", "vous êtes déjà dans la file")
		return
	case m.occupe(invite.ID):
		e.erreur(c, "partenaire_occupe", invite.Pseudo+" est déjà dans la file")
		return
	}
	for _, inv := range m.Etat.Invitations {
		if inv.De.ID == c.Joueur.ID && inv.A.ID == invite.ID {
			e.erreur(c, protocole.CodeConflit, "invitation déjà envoyée")
			return
		}
	}
	inv := &Invitation{ID: m.NouvelID(), De: c.Joueur, A: *invite, ExpireLe: maintenant.Add(m.Parametres.DelaiInvitation)}
	m.Etat.Invitations[inv.ID] = inv
	msg := MessageInvitation{Invitation: inv.ID, De: inv.De, A: inv.A, ExpireLe: inv.ExpireLe}
	e.notifier(invite.ID, MsgInvitation, msg)
	e.notifier(c.Joueur.ID, MsgEnvoyee, msg)
	e.Modifie = true
}

func (m *Moteur) repondre(c Commande, maintenant time.Time, e *Effets) {
	inv := m.Etat.Invitations[c.Invitation]
	if inv == nil || inv.A.ID != c.Joueur.ID {
		e.erreur(c, protocole.CodeIntrouvable, "invitation introuvable ou expirée")
		return
	}
	delete(m.Etat.Invitations, inv.ID)
	e.Modifie = true
	msg := MessageInvitation{Invitation: inv.ID, De: inv.De, A: inv.A, ExpireLe: inv.ExpireLe}
	if !c.Accepte {
		e.notifier(inv.De.ID, MsgRefusee, msg)
		return
	}
	if m.occupe(inv.De.ID) || m.occupe(c.Joueur.ID) {
		e.erreur(c, "partenaire_occupe", "l'un de vous est déjà dans la file")
		return
	}
	// Elo à jour du joueur qui accepte, celui de l'invitation pour l'invitant.
	t := &Ticket{ID: m.NouvelID(), Joueurs: []table.Joueur{inv.De, c.Joueur}, EntreeLe: maintenant}
	m.Etat.Tickets[t.ID] = t
	m.annulerInvitationsDe(inv.De.ID, e)
	m.annulerInvitationsDe(c.Joueur.ID, e)
	e.notifier(inv.De.ID, MsgRejointe, MessageRejointe{Mode: "duo", Depuis: maintenant, Partenaire: &t.Joueurs[1]})
	e.notifier(c.Joueur.ID, MsgRejointe, MessageRejointe{Mode: "duo", Depuis: maintenant, Partenaire: &t.Joueurs[0]})
}

// annulerProposition retire les fautifs (et leur partenaire de duo) et remet les autres en file
// avec leur ancienneté d'origine.
func (m *Moteur) annulerProposition(p *Proposition, fautifs map[uuid.UUID]bool, raison string, e *Effets) {
	delete(m.Etat.Propositions, p.ID)
	e.Modifie = true
	for _, t := range p.Tickets {
		fautif := slices.ContainsFunc(t.Joueurs, func(j table.Joueur) bool { return fautifs[j.ID] })
		if !fautif {
			m.Etat.Tickets[t.ID] = &t
		}
		for _, j := range t.Joueurs {
			e.notifier(j.ID, MsgAnnulee, MessageAnnulee{Proposition: p.ID, Raison: raison, EnFile: !fautif})
		}
	}
}

func (m *Moteur) retirerTicket(t *Ticket, raison string, auteur uuid.UUID, e *Effets) {
	delete(m.Etat.Tickets, t.ID)
	e.Modifie = true
	for _, j := range t.Joueurs {
		r := raison
		if j.ID != auteur {
			r = "partenaire"
		}
		e.notifier(j.ID, MsgQuittee, MessageQuittee{Raison: r})
	}
}

func (m *Moteur) annulerInvitationsDe(joueur uuid.UUID, e *Effets) {
	for id, inv := range m.Etat.Invitations {
		if inv.De.ID != joueur && inv.A.ID != joueur {
			continue
		}
		delete(m.Etat.Invitations, id)
		e.Modifie = true
		autre := inv.A.ID
		if autre == joueur {
			autre = inv.De.ID
		}
		e.notifier(autre, MsgExpiree, MessageInvitation{Invitation: id, De: inv.De, A: inv.A, ExpireLe: inv.ExpireLe})
	}
}

func (m *Moteur) ticketDe(joueur uuid.UUID) *Ticket {
	for _, t := range m.Etat.Tickets {
		if slices.ContainsFunc(t.Joueurs, func(j table.Joueur) bool { return j.ID == joueur }) {
			return t
		}
	}
	return nil
}

func (m *Moteur) propositionDe(joueur uuid.UUID) *Proposition {
	for _, p := range m.Etat.Propositions {
		if p.contient(joueur) {
			return p
		}
	}
	return nil
}

func (m *Moteur) occupe(joueur uuid.UUID) bool {
	return m.ticketDe(joueur) != nil || m.propositionDe(joueur) != nil
}
