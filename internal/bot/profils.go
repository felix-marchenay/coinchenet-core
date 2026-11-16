package bot

import "github.com/felix-marchenay/coinchenet-core/internal/coinche"

// Jamais désactive un seuil : aucune main ne l'atteint.
const Jamais = 1000

// Profil donne une personnalité à un bot : son goût du risque aux enchères et dans le jeu de la carte.
// Les seuils s'expriment en points de mène (162 au total, hors belote).
type Profil struct {
	ID          string `json:"id"`
	Nom         string `json:"nom"`
	Style       string `json:"style"`
	Description string `json:"description"`

	// Optimisme : points que le bot compte recevoir de son partenaire, en attaque comme en défense.
	Optimisme int `json:"-"`
	// SoutienMin : apport minimal de sa main pour monter sur l'annonce de son partenaire.
	SoutienMin int `json:"-"`
	// AudaceCapot : estimation à partir de laquelle il annonce un capot sans tenir toutes ses levées.
	AudaceCapot int `json:"-"`
	// MargeCoinche : chute attendue du contrat adverse à partir de laquelle il coinche.
	MargeCoinche int `json:"-"`
	// MargeSurcoinche : avance attendue sur son contrat coinché à partir de laquelle il surcoinche.
	MargeSurcoinche int `json:"-"`
	// ChargeRisquee : il charge le pli de son partenaire dès que sa carte est maîtresse dans la
	// couleur, sans attendre qu'elle soit à l'abri d'une coupe.
	ChargeRisquee bool `json:"-"`
}

var (
	Bernard = Profil{
		ID:          "bernard",
		Nom:         "Bernard",
		Style:       "prudent",
		Description: "N'annonce que ce qu'il tient et soutient peu, mais ne laisse rien passer en défense : un contrat trop gourmand, il le coinche.",
		Optimisme:   20, SoutienMin: 20, AudaceCapot: Jamais, MargeCoinche: 10, MargeSurcoinche: Jamais,
	}
	Martine = Profil{
		ID:          "martine",
		Nom:         "Martine",
		Style:       "équilibrée",
		Description: "Joue juste et sans excès : annonce ce que sa main vaut, coinche quand la chute est probable.",
		Optimisme:   30, SoutienMin: 10, AudaceCapot: Jamais, MargeCoinche: 20, MargeSurcoinche: 40,
	}
	Nadia = Profil{
		ID:          "nadia",
		Nom:         "Nadia",
		Style:       "agressive",
		Description: "Veut la main à chaque donne : compte sur son partenaire, surenchérit plutôt que de coincher, charge les plis sans attendre.",
		Optimisme:   35, SoutienMin: 10, AudaceCapot: 170, MargeCoinche: 30, MargeSurcoinche: 20, ChargeRisquee: true,
	}
	Leo = Profil{
		ID:          "leo",
		Nom:         "Léo",
		Style:       "casse-cou",
		Description: "Ose tout : capots à l'intuition, coinches au culot, surcoinches pour le panache. Ça passe… souvent.",
		Optimisme:   30, SoutienMin: 10, AudaceCapot: 150, MargeCoinche: 10, MargeSurcoinche: 0, ChargeRisquee: true,
	}

	// Profils, du plus prudent au plus téméraire. Réglés au tournoi (coinchenet tournoi) pour rester
	// de force comparable : leur personnalité change leur façon de gagner, pas leurs chances.
	Profils = []Profil{Bernard, Martine, Nadia, Leo}
	// Defaut joue pour les joueurs absents et les profils inconnus.
	Defaut = Martine
)

// Pour renvoie le profil d'identifiant id, ou Defaut.
func Pour(id string) Profil {
	if p, ok := Trouver(id); ok {
		return p
	}
	return Defaut
}

func Trouver(id string) (Profil, bool) {
	for _, p := range Profils {
		if p.ID == id {
			return p, true
		}
	}
	return Profil{}, false
}

// Bots fait jouer chaque siège selon son profil (table.Decideur).
type Bots struct{}

func (Bots) Encherir(profil string, v coinche.Vue) coinche.Enchere {
	return Bot{Profil: Pour(profil)}.Encherir(v)
}

func (Bots) Jouer(profil string, v coinche.Vue) coinche.Carte {
	return Bot{Profil: Pour(profil)}.Jouer(v)
}

func (Bots) Profils() []string {
	ids := make([]string, len(Profils))
	for i, p := range Profils {
		ids[i] = p.ID
	}
	return ids
}

func (Bots) Nom(profil string) (string, bool) {
	p, ok := Trouver(profil)
	return p.Nom, ok
}
