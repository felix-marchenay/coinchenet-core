package bot

import "github.com/felix-marchenay/coinchenet-core/internal/coinche"

// Jamais désactive un seuil : aucune main ne l'atteint.
const Jamais = 1000

// Profil est la façon de jouer d'une famille de bots : son goût du risque aux enchères et dans le jeu
// de la carte. Les seuils s'expriment en points de mène (162 au total, hors belote).
type Profil struct {
	ID          string `json:"id"`
	Famille     string `json:"famille"`
	Style       string `json:"style"`
	Description string `json:"description"`
	// Prenoms : chaque bot de la famille en porte un, tiré au hasard.
	Prenoms []string `json:"-"`

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
	Kerfoule = Profil{
		ID:          "kerfoule",
		Famille:     "Kerfoule",
		Style:       "prudent",
		Description: "Prudents de père en fils : n'annoncent que ce qu'ils tiennent et soutiennent peu, mais coinchent sans pitié un contrat trop gourmand.",
		Prenoms:     []string{"Bernard", "Gisèle", "Yvon", "Josiane", "Gérard", "Annick", "Loïc", "Maryvonne", "Hervé", "Odile"},
		Optimisme:   20, SoutienMin: 20, AudaceCapot: Jamais, MargeCoinche: 10, MargeSurcoinche: Jamais,
	}
	McBalance = Profil{
		ID:          "mcbalance",
		Famille:     "McBalance",
		Style:       "équilibré",
		Description: "Ni trop, ni trop peu : annoncent ce que vaut leur main, coinchent quand la chute est probable.",
		Prenoms:     []string{"Martine", "Douglas", "Fiona", "Paul", "Claire", "Ewan", "Sophie", "Malcolm", "Hélène", "Thomas"},
		Optimisme:   30, SoutienMin: 10, AudaceCapot: Jamais, MargeCoinche: 20, MargeSurcoinche: 40,
	}
	Lashpa = Profil{
		ID:          "lashpa",
		Famille:     "Lashpa",
		Style:       "agressif",
		Description: "Veulent la main à chaque donne : comptent sur leur partenaire, surenchérissent plutôt que de coincher, chargent les plis sans attendre.",
		Prenoms:     []string{"Nadia", "Max", "Inès", "Hugo", "Sacha", "Lina", "Tony", "Yasmine", "Rémi", "Zoé"},
		Optimisme:   35, SoutienMin: 10, AudaceCapot: 170, MargeCoinche: 30, MargeSurcoinche: 20, ChargeRisquee: true,
	}
	Narvaloche = Profil{
		ID:          "narvaloche",
		Famille:     "Narvaloche",
		Style:       "casse-cou",
		Description: "Osent tout : capots à l'intuition, coinches au culot, surcoinches pour le panache. Ça passe… souvent.",
		Prenoms:     []string{"Léo", "Jojo", "Lulu", "Rico", "Gaspard", "Lola", "Django", "Capucine", "Titouan", "Sacha"},
		Optimisme:   30, SoutienMin: 10, AudaceCapot: 150, MargeCoinche: 10, MargeSurcoinche: 0, ChargeRisquee: true,
	}

	// Profils, du plus prudent au plus téméraire. Réglés au tournoi (coinchenet tournoi) pour rester
	// de force comparable : leur personnalité change leur façon de gagner, pas leurs chances.
	Profils = []Profil{Kerfoule, McBalance, Lashpa, Narvaloche}
	// Defaut joue pour les profils inconnus.
	Defaut = McBalance
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

func (Bots) Famille(profil string) (string, []string, bool) {
	p, ok := Trouver(profil)
	return p.Famille, p.Prenoms, ok
}
