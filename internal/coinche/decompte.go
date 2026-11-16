package coinche

const (
	PointsDixDeDer = 10
	PointsBelote   = 20
	// PointsCapot remplace les 162 points de la mène pour l'équipe qui fait tous les plis.
	PointsCapot = 250
	// PointsChute est la base marquée par l'équipe qui gagne un contrat chuté ou coinché.
	PointsChute = 160
)

type ResultatMene struct {
	Redistribuee   bool     `json:"redistribuee"`
	Contrat        *Contrat `json:"contrat,omitempty"`
	Preneur        Siege    `json:"preneur"`
	Multiplicateur int      `json:"multiplicateur"`
	PlisGagnes     [2]int   `json:"plis_gagnes"`
	PointsFaits    [2]int   `json:"points_faits"`
	Belote         *Equipe  `json:"belote,omitempty"`
	Capot          *Equipe  `json:"capot,omitempty"`
	Reussi         bool     `json:"reussi"`
	Points         [2]int   `json:"points"`
}

// Decompter calcule les points marqués par chaque équipe à la fin d'une mène jouée.
func Decompter(d Decompte, m *Mene) ResultatMene {
	contrat := *m.Contrat
	atout := contrat.Atout
	r := ResultatMene{
		Contrat:        &contrat,
		Preneur:        m.Preneur,
		Multiplicateur: m.Multiplicateur,
	}

	for i, pli := range m.Plis {
		e := pli.Gagnant.Equipe()
		r.PlisGagnes[e]++
		r.PointsFaits[e] += pli.Score(atout)
		if i == len(m.Plis)-1 {
			r.PointsFaits[e] += PointsDixDeDer
		}
	}

	for _, e := range [...]Equipe{EquipeA, EquipeB} {
		if r.PlisGagnes[e] == NbPlis {
			r.Capot = &e
			r.PointsFaits[e] = PointsCapot
			r.PointsFaits[e.Adverse()] = 0
		}
	}

	var belote [2]int
	if m.Belote != nil {
		e := m.Belote.Equipe()
		r.Belote = &e
		belote[e] = PointsBelote
	}

	attaque := m.Preneur.Equipe()
	defense := attaque.Adverse()

	switch contrat.Valeur {
	case Capot:
		r.Reussi = r.PlisGagnes[attaque] == NbPlis
	case Generale:
		r.Reussi = tousLesPlisPour(m.Plis, m.Preneur)
	default:
		// La belote compte pour remplir le contrat, et il faut faire plus que la défense.
		attaquePoints := r.PointsFaits[attaque] + belote[attaque]
		defensePoints := r.PointsFaits[defense] + belote[defense]
		r.Reussi = attaquePoints >= int(contrat.Valeur) && attaquePoints > defensePoints
	}

	gagnante := attaque
	if !r.Reussi {
		gagnante = defense
	}
	valeur := int(contrat.Valeur) * m.Multiplicateur

	switch {
	case d == DecompteContratSeul:
		r.Points[gagnante] = valeur
	case r.Reussi && m.Multiplicateur == 1:
		r.Points[attaque] = r.PointsFaits[attaque] + valeur
		r.Points[defense] = r.PointsFaits[defense]
	default:
		base := PointsChute
		if r.Capot != nil && *r.Capot == gagnante {
			base = PointsCapot
		}
		r.Points[gagnante] = base + valeur
	}

	r.Points[EquipeA] += belote[EquipeA]
	r.Points[EquipeB] += belote[EquipeB]
	return r
}

func tousLesPlisPour(plis []Pli, s Siege) bool {
	if len(plis) != NbPlis {
		return false
	}
	for _, p := range plis {
		if p.Gagnant != s {
			return false
		}
	}
	return true
}
