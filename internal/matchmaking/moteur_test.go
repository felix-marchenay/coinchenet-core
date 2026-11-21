package matchmaking

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/felix-marchenay/coinchenet-core/internal/protocole"
	"github.com/felix-marchenay/coinchenet-core/internal/table"
)

func moteurDeTest() *Moteur {
	m := NouveauMoteur(NouvelEtat(), ParametresParDefaut())
	n := 0
	m.NouvelID = func() string {
		n++
		return fmt.Sprintf("id%d", n)
	}
	return m
}

func joueurs(n int) []table.Joueur {
	js := make([]table.Joueur, n)
	for i := range js {
		js[i] = table.Joueur{ID: uuid.New(), Pseudo: fmt.Sprintf("J%d", i), Elo: 1500}
	}
	return js
}

func types(e Effets, j uuid.UUID) []string {
	var ts []string
	for _, n := range e.Notifications {
		if n.Joueur == j {
			ts = append(ts, n.Message.Type)
		}
	}
	return ts
}

func donnees[T any](t *testing.T, e Effets, j uuid.UUID, typ string) T {
	t.Helper()
	for _, n := range e.Notifications {
		if n.Joueur == j && n.Message.Type == typ {
			b, err := json.Marshal(n.Message.Data)
			require.NoError(t, err)
			var v T
			require.NoError(t, json.Unmarshal(b, &v))
			return v
		}
	}
	t.Fatalf("pas de message %s pour %s", typ, j)
	var zero T
	return zero
}

func rejoindre(m *Moteur, js ...table.Joueur) {
	for _, j := range js {
		m.Appliquer(Commande{Type: CmdRejoindre, Joueur: j}, t0)
	}
}

func TestRejoindreEtQuitter(t *testing.T) {
	m := moteurDeTest()
	j := joueurs(1)[0]

	e := m.Appliquer(Commande{Type: CmdRejoindre, Joueur: j}, t0)
	assert.Equal(t, []string{MsgRejointe}, types(e, j.ID))
	assert.True(t, e.Modifie)
	assert.Equal(t, 1, m.NbEnFile())

	e = m.Appliquer(Commande{Type: CmdRejoindre, Joueur: j, Ref: "r1"}, t0)
	assert.Equal(t, []string{protocole.TypeErreur}, types(e, j.ID), "pas deux fois dans la file")
	assert.Equal(t, "r1", e.Notifications[0].Message.Ref)

	e = m.Appliquer(Commande{Type: CmdQuitter, Joueur: j}, t0)
	assert.Equal(t, MessageQuittee{Raison: "demande"}, donnees[MessageQuittee](t, e, j.ID, MsgQuittee))
	assert.Equal(t, 0, m.NbEnFile())
}

func TestPropositionAccepteeParTous(t *testing.T) {
	m := moteurDeTest()
	js := joueurs(4)
	rejoindre(m, js...)

	e := m.Tick(t0.Add(time.Second))
	require.Len(t, m.Etat.Propositions, 1)
	assert.Equal(t, 0, m.NbEnFile(), "les joueurs proposés sortent de la file")
	trouvee := donnees[MessageTrouvee](t, e, js[0].ID, MsgTrouvee)
	assert.Equal(t, t0.Add(16*time.Second), trouvee.ExpireLe)

	for i, j := range js[:3] {
		e = m.Appliquer(Commande{Type: CmdAccepter, Joueur: j, Proposition: trouvee.Proposition}, t0.Add(2*time.Second))
		assert.Equal(t, i+1, donnees[MessageAcceptation](t, e, js[3].ID, MsgAcceptation).Acceptes)
		assert.Empty(t, e.Tables)
	}
	e = m.Appliquer(Commande{Type: CmdAccepter, Joueur: js[3], Proposition: trouvee.Proposition}, t0.Add(3*time.Second))

	require.Len(t, e.Tables, 1)
	assert.Equal(t, [2]table.Joueur{*trouvee.EquipeA[0], *trouvee.EquipeA[1]}, e.Tables[0].EquipeA)
	assert.Len(t, e.Tables[0].Attentes, 4)
	assert.Equal(t, 3*time.Second, e.Tables[0].Attentes[0])
	assert.Empty(t, m.Etat.Propositions)
}

func TestRefusRemetLesAutresEnFile(t *testing.T) {
	m := moteurDeTest()
	js := joueurs(4)
	rejoindre(m, js...)
	e := m.Tick(t0)
	p := donnees[MessageTrouvee](t, e, js[0].ID, MsgTrouvee).Proposition

	e = m.Appliquer(Commande{Type: CmdRefuser, Joueur: js[1], Proposition: p}, t0)

	assert.Equal(t, MessageAnnulee{Proposition: p, Raison: "refus", EnFile: false}, donnees[MessageAnnulee](t, e, js[1].ID, MsgAnnulee))
	assert.Equal(t, MessageAnnulee{Proposition: p, Raison: "refus", EnFile: true}, donnees[MessageAnnulee](t, e, js[0].ID, MsgAnnulee))
	assert.Equal(t, 3, m.NbEnFile())
	for _, tk := range m.Etat.Tickets {
		assert.Equal(t, t0, tk.EntreeLe, "l'ancienneté est conservée")
	}
}

func TestPropositionExpiree(t *testing.T) {
	m := moteurDeTest()
	m.Parametres.AttenteParBot = 0
	js := joueurs(4)
	rejoindre(m, js...)
	e := m.Tick(t0)
	p := donnees[MessageTrouvee](t, e, js[0].ID, MsgTrouvee).Proposition
	m.Appliquer(Commande{Type: CmdAccepter, Joueur: js[0], Proposition: p}, t0)
	m.Appliquer(Commande{Type: CmdAccepter, Joueur: js[1], Proposition: p}, t0)

	e = m.Tick(t0.Add(20 * time.Second))

	assert.True(t, donnees[MessageAnnulee](t, e, js[0].ID, MsgAnnulee).EnFile)
	assert.False(t, donnees[MessageAnnulee](t, e, js[2].ID, MsgAnnulee).EnFile)
	assert.Equal(t, 2, m.NbEnFile())
}

func TestDesBotsCompletentLaTableDuJoueurSeul(t *testing.T) {
	m := moteurDeTest()
	j := joueurs(1)[0]
	rejoindre(m, j)

	assert.Empty(t, m.Tick(t0.Add(14*time.Second)).Notifications, "un bot toutes les 5 s : il en faut trois")
	e := m.Tick(t0.Add(15 * time.Second))
	require.Len(t, e.Notifications, 1, "les bots ne reçoivent rien")
	trouvee := donnees[MessageTrouvee](t, e, j.ID, MsgTrouvee)
	assert.Equal(t, table.AvecBots, trouvee.Type)
	assert.Equal(t, j, *trouvee.EquipeA[0])
	assert.Nil(t, trouvee.EquipeA[1])
	assert.Equal(t, [2]*table.Joueur{}, trouvee.EquipeB)

	e = m.Appliquer(Commande{Type: CmdAccepter, Joueur: j, Proposition: trouvee.Proposition}, t0.Add(16*time.Second))
	require.Len(t, e.Tables, 1, "les bots acceptent d'office")
	assert.Equal(t, [2]table.Joueur{j, {}}, e.Tables[0].EquipeA)
	assert.Equal(t, [2]table.Joueur{}, e.Tables[0].EquipeB)
	assert.Equal(t, []time.Duration{16 * time.Second}, e.Tables[0].Attentes)
}

func TestAccepterUnePropositionInconnue(t *testing.T) {
	m := moteurDeTest()
	j := joueurs(1)[0]
	e := m.Appliquer(Commande{Type: CmdAccepter, Joueur: j, Proposition: "nope"}, t0)
	assert.Equal(t, []string{protocole.TypeErreur}, types(e, j.ID))
}

func TestDeconnexionPendantUneProposition(t *testing.T) {
	m := moteurDeTest()
	js := joueurs(4)
	rejoindre(m, js...)
	m.Tick(t0)

	e := m.Appliquer(Commande{Type: CmdDeconnexion, Joueur: js[2]}, t0)

	assert.False(t, donnees[MessageAnnulee](t, e, js[2].ID, MsgAnnulee).EnFile)
	assert.Equal(t, 3, m.NbEnFile())
}

func TestInvitationDuo(t *testing.T) {
	m := moteurDeTest()
	js := joueurs(2)
	alice, bob := js[0], js[1]

	e := m.Appliquer(Commande{Type: CmdInviter, Joueur: alice, Invite: &bob}, t0)
	inv := donnees[MessageInvitation](t, e, bob.ID, MsgInvitation)
	assert.Equal(t, alice.ID, inv.De.ID)
	assert.Equal(t, []string{MsgEnvoyee}, types(e, alice.ID))

	e = m.Appliquer(Commande{Type: CmdInviter, Joueur: alice, Invite: &bob}, t0)
	assert.Equal(t, []string{protocole.TypeErreur}, types(e, alice.ID), "pas deux invitations identiques")

	e = m.Appliquer(Commande{Type: CmdRepondre, Joueur: alice, Invitation: inv.Invitation, Accepte: true}, t0)
	assert.Equal(t, []string{protocole.TypeErreur}, types(e, alice.ID), "seul l'invité répond")

	e = m.Appliquer(Commande{Type: CmdRepondre, Joueur: bob, Invitation: inv.Invitation, Accepte: true}, t0)
	rejointe := donnees[MessageRejointe](t, e, alice.ID, MsgRejointe)
	assert.Equal(t, "duo", rejointe.Mode)
	assert.Equal(t, bob.ID, rejointe.Partenaire.ID)
	assert.Equal(t, 2, m.NbEnFile())
	assert.Empty(t, m.Etat.Invitations)

	e = m.Appliquer(Commande{Type: CmdQuitter, Joueur: bob}, t0)
	assert.Equal(t, "partenaire", donnees[MessageQuittee](t, e, alice.ID, MsgQuittee).Raison, "le duo quitte la file ensemble")
	assert.Equal(t, 0, m.NbEnFile())
}

func TestInvitationRefuseeOuExpiree(t *testing.T) {
	m := moteurDeTest()
	js := joueurs(3)
	alice, bob, carole := js[0], js[1], js[2]

	e := m.Appliquer(Commande{Type: CmdInviter, Joueur: alice, Invite: &bob}, t0)
	inv := donnees[MessageInvitation](t, e, bob.ID, MsgInvitation)
	e = m.Appliquer(Commande{Type: CmdRepondre, Joueur: bob, Invitation: inv.Invitation}, t0)
	assert.Equal(t, []string{MsgRefusee}, types(e, alice.ID))

	m.Appliquer(Commande{Type: CmdInviter, Joueur: alice, Invite: &carole}, t0)
	e = m.Tick(t0.Add(2 * time.Minute))
	assert.Equal(t, []string{MsgExpiree}, types(e, alice.ID))
	assert.Empty(t, m.Etat.Invitations)
}

func TestInvitationsInvalides(t *testing.T) {
	m := moteurDeTest()
	js := joueurs(2)
	alice, bob := js[0], js[1]

	e := m.Appliquer(Commande{Type: CmdInviter, Joueur: alice, Invite: &alice}, t0)
	assert.Equal(t, []string{protocole.TypeErreur}, types(e, alice.ID))

	rejoindre(m, bob)
	e = m.Appliquer(Commande{Type: CmdInviter, Joueur: alice, Invite: &bob}, t0)
	assert.Equal(t, []string{protocole.TypeErreur}, types(e, alice.ID), "l'invité est déjà en file")
}

func TestEtatALaReconnexion(t *testing.T) {
	m := moteurDeTest()
	js := joueurs(5)

	e := m.Appliquer(Commande{Type: CmdEtat, Joueur: js[0]}, t0)
	assert.Empty(t, e.Notifications, "rien à dire à un joueur hors file")

	m.Appliquer(Commande{Type: CmdInviter, Joueur: js[0], Invite: &js[1]}, t0)
	var inv string
	for id := range m.Etat.Invitations {
		inv = id
	}
	m.Appliquer(Commande{Type: CmdRepondre, Joueur: js[1], Invitation: inv, Accepte: true}, t0)
	e = m.Appliquer(Commande{Type: CmdEtat, Joueur: js[0]}, t0.Add(time.Minute))
	rejointe := donnees[MessageRejointe](t, e, js[0].ID, MsgRejointe)
	assert.Equal(t, "duo", rejointe.Mode)
	assert.Equal(t, js[1].ID, rejointe.Partenaire.ID)
	assert.Equal(t, t0, rejointe.Depuis)
	assert.False(t, e.Modifie)

	rejoindre(m, js[2], js[3])
	m.Tick(t0.Add(time.Hour))
	require.Len(t, m.Etat.Propositions, 1)
	e = m.Appliquer(Commande{Type: CmdEtat, Joueur: js[2]}, t0.Add(time.Hour))
	assert.Equal(t, []string{MsgTrouvee, MsgAcceptation}, types(e, js[2].ID))
}

func TestCommandeInconnue(t *testing.T) {
	m := moteurDeTest()
	j := joueurs(1)[0]
	e := m.Appliquer(Commande{Type: "danser", Joueur: j}, t0)
	assert.Equal(t, []string{protocole.TypeErreur}, types(e, j.ID))
}

func TestEtatSerialisable(t *testing.T) {
	m := moteurDeTest()
	js := joueurs(5)
	rejoindre(m, js...)
	m.Tick(t0)

	b, err := json.Marshal(m.Etat)
	require.NoError(t, err)
	var relu Etat
	require.NoError(t, json.Unmarshal(b, &relu))

	assert.Equal(t, len(m.Etat.Tickets), len(relu.Tickets))
	assert.Equal(t, len(m.Etat.Propositions), len(relu.Propositions))
}
