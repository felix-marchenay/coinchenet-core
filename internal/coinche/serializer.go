package coinche

import (
	"fmt"
	"strconv"
)

func SerializePaquet(p CarteCollection) string {
	var result string
	for _, carte := range p.Cartes {
		result += SerializeCarte(carte) + ", "
	}
	return "Paquet{" + result + "}"
}

func SerializeCarte(c Carte) string {
	cstr := ""
	switch c.Couleur {
	case Coeur:
		cstr = "Coeur"
	case Carreau:
		cstr = "Carreau"
	case Trefle:
		cstr = "Trefle"
	case Pique:
		cstr = "Pique"
	default:
		cstr = "-"
	}

	vstr := ""
	switch c.Valeur {
	case Sept:
		vstr = "Sept"
	case Huit:
		vstr = "Huit"
	case Neuf:
		vstr = "Neuf"
	case Dix:
		vstr = "Dix"
	case Valet:
		vstr = "Valet"
	case Dame:
		vstr = "Dame"
	case Roi:
		vstr = "Roi"
	case As:
		vstr = "As"
	default:
		vstr = "Inconnu"
	}

	return fmt.Sprintf("%s %s", vstr, cstr)
}

func SerializePartie(p Partie) (str string) {
	str += "Equipe 1 : " + p.Equipe1.J1.Nom + " & " + p.Equipe1.J2.Nom
	str += "\nEquipe 2 : " + p.Equipe2.J1.Nom + " & " + p.Equipe2.J2.Nom

	str += "\n" + strconv.Itoa(len(p.Menes)) + " mènes"
	for _, m := range p.Menes {
		for e, ps := range m.Plis {
			str += "\nEquipe " + e.J1.Nom + " & " + e.J2.Nom
			for _, pli := range ps {
				str += "\n" + SerializePaquet(pli.SCartes()) + " -> " + strconv.Itoa(pli.Score(m.Contrat.Couleur)) + " points"
			}
		}
	}

	return str
}
