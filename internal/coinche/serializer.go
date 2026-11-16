package coinche

import (
	"fmt"
	"strings"
)

var nomsLongsCouleurs = [...]string{Coeur: "Coeur", Carreau: "Carreau", Trefle: "Trefle", Pique: "Pique"}

var nomsLongsValeurs = [...]string{Sept: "Sept", Huit: "Huit", Neuf: "Neuf", Dix: "Dix", Valet: "Valet", Dame: "Dame", Roi: "Roi", As: "As"}

func SerializePaquet(p CarteCollection) string {
	var b strings.Builder
	for _, carte := range p.Cartes {
		b.WriteString(SerializeCarte(carte) + ", ")
	}
	return "Paquet{" + b.String() + "}"
}

func SerializeCarte(c Carte) string {
	if !c.Valide() {
		return "Inconnu -"
	}
	return nomsLongsValeurs[c.Valeur] + " " + nomsLongsCouleurs[c.Couleur]
}

// SerializePartie produit un résumé lisible, utile pour déboguer une partie.
func SerializePartie(p *Partie) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Objectif %d, scores A %d - B %d, %d mènes", p.Regles.Objectif, p.Scores[EquipeA], p.Scores[EquipeB], len(p.Menes))
	for _, m := range p.Menes {
		fmt.Fprintf(&b, "\nMène %d (donneur %d)", m.Numero, m.Donneur)
		if m.Contrat != nil {
			fmt.Fprintf(&b, " : %s par le siège %d ×%d", m.Contrat, m.Preneur, m.Multiplicateur)
		}
		for _, pli := range m.Plis {
			fmt.Fprintf(&b, "\n  %s -> siège %d, %d points", SerializePaquet(pli.SCartes()), pli.Gagnant, pli.Score(m.Contrat.Atout))
		}
		if m.Resultat != nil {
			fmt.Fprintf(&b, "\n  résultat : A %d - B %d", m.Resultat.Points[EquipeA], m.Resultat.Points[EquipeB])
		}
	}
	if p.Terminee {
		fmt.Fprintf(&b, "\nVictoire de l'équipe %s", p.Gagnante)
	}
	return b.String()
}
