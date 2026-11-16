package matchmaking

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/felix-marchenay/coinchenet-core/internal/table"
)

var t0 = time.Date(2026, 11, 16, 20, 0, 0, 0, time.UTC)

func solo(id string, elo int, attente time.Duration) Ticket {
	return Ticket{ID: id, Joueurs: []table.Joueur{{ID: uuid.New(), Pseudo: id, Elo: elo}}, EntreeLe: t0.Add(-attente)}
}

func duo(id string, elo1, elo2 int, attente time.Duration) Ticket {
	return Ticket{
		ID:       id,
		Joueurs:  []table.Joueur{{ID: uuid.New(), Pseudo: id + "1", Elo: elo1}, {ID: uuid.New(), Pseudo: id + "2", Elo: elo2}},
		EntreeLe: t0.Add(-attente),
	}
}

func ids(a Appariement) []string {
	var res []string
	for _, t := range a.Tickets {
		res = append(res, t.ID)
	}
	return res
}

func TestFenetre(t *testing.T) {
	p := ParametresParDefaut()
	assert.Equal(t, 100, p.Fenetre(0))
	assert.Equal(t, 100, p.Fenetre(14*time.Second))
	assert.Equal(t, 150, p.Fenetre(15*time.Second))
	assert.Equal(t, 300, p.Fenetre(time.Minute))
	assert.Equal(t, 400, p.Fenetre(time.Hour))
	assert.Equal(t, 100, p.Fenetre(-time.Second))
}

func TestQuatreSolosEquilibres(t *testing.T) {
	tickets := []Ticket{solo("a", 1500, 0), solo("b", 1450, 0), solo("c", 1560, 0), solo("d", 1490, 0)}

	res := Apparier(tickets, t0, ParametresParDefaut())

	require.Len(t, res, 1)
	a := res[0]
	assert.Equal(t, []string{"c", "a"}, []string{a.EquipeA[0].Pseudo, a.EquipeB[0].Pseudo})
	assert.Equal(t, "b", a.EquipeA[1].Pseudo, "le plus fort joue avec le plus faible")
	assert.Equal(t, "d", a.EquipeB[1].Pseudo)
	assert.ElementsMatch(t, []string{"a", "b", "c", "d"}, ids(a))
}

func TestPasAssezDeJoueurs(t *testing.T) {
	res := Apparier([]Ticket{solo("a", 1500, 0), solo("b", 1500, 0), solo("c", 1500, 0)}, t0, ParametresParDefaut())
	assert.Empty(t, res)
}

func TestEcartDeNiveauTropGrand(t *testing.T) {
	tickets := []Ticket{solo("a", 1000, 0), solo("b", 1000, 0), solo("c", 1000, 0), solo("d", 1400, 0)}
	assert.Empty(t, Apparier(tickets, t0, ParametresParDefaut()))

	// La fenêtre s'élargit avec l'attente.
	tickets[0] = solo("a", 1000, 2*time.Minute)
	assert.Len(t, Apparier(tickets, t0, ParametresParDefaut()), 1)
}

func TestLePlusAncienEstServiEnPremier(t *testing.T) {
	tickets := []Ticket{
		solo("recent", 1500, 0),
		solo("ancien", 1500, time.Minute),
		solo("b", 1500, 10*time.Second),
		solo("c", 1500, 10*time.Second),
		solo("d", 1500, 10*time.Second),
	}

	res := Apparier(tickets, t0, ParametresParDefaut())

	require.Len(t, res, 1)
	assert.Contains(t, ids(res[0]), "ancien")
	assert.NotContains(t, ids(res[0]), "recent")
}

func TestPlusieursTables(t *testing.T) {
	var tickets []Ticket
	for i := range 9 {
		tickets = append(tickets, solo(string(rune('a'+i)), 1500+i*10, 0))
	}
	assert.Len(t, Apparier(tickets, t0, ParametresParDefaut()), 2)
}

func TestDuoContreDuo(t *testing.T) {
	tickets := []Ticket{duo("x", 1500, 1600, 0), solo("a", 1550, 0), solo("b", 1550, 0), duo("y", 1520, 1580, 0)}

	res := Apparier(tickets, t0, ParametresParDefaut())

	require.Len(t, res, 1)
	assert.ElementsMatch(t, []string{"x", "y"}, ids(res[0]), "un duo affronte de préférence un duo")
	assert.Equal(t, "x1", res[0].EquipeA[0].Pseudo)
	assert.Equal(t, "x2", res[0].EquipeA[1].Pseudo, "les partenaires d'un duo jouent ensemble")
}

func TestDuoContreSolosApresAttente(t *testing.T) {
	tickets := []Ticket{duo("x", 1500, 1500, 0), solo("a", 1500, 0), solo("b", 1500, 0)}
	assert.Empty(t, Apparier(tickets, t0, ParametresParDefaut()), "un duo attend d'abord un autre duo")

	tickets[0] = duo("x", 1500, 1500, 31*time.Second)
	res := Apparier(tickets, t0, ParametresParDefaut())
	require.Len(t, res, 1)
	assert.Equal(t, "x1", res[0].EquipeA[0].Pseudo)
	assert.ElementsMatch(t, []string{"a", "b"}, []string{res[0].EquipeB[0].Pseudo, res[0].EquipeB[1].Pseudo})
}

func TestSoloPivotAvecDuoPatient(t *testing.T) {
	tickets := []Ticket{solo("a", 1500, time.Minute), duo("x", 1500, 1500, 40*time.Second), solo("b", 1500, 0)}

	res := Apparier(tickets, t0, ParametresParDefaut())

	require.Len(t, res, 1)
	assert.ElementsMatch(t, []string{"a", "x", "b"}, ids(res[0]))
}

func TestEloDUnTicket(t *testing.T) {
	assert.InDelta(t, 1550, duo("x", 1500, 1600, 0).Elo(), 1e-9)
	assert.True(t, duo("x", 1, 1, 0).Duo())
	assert.False(t, solo("a", 1, 0).Duo())
}
