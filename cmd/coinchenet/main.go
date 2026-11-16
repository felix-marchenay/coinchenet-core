// Commande coinchenet : serveur de jeu, migrations, simulation et contrôle de santé.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/felix-marchenay/coinchenet-core/internal/application"
	"github.com/felix-marchenay/coinchenet-core/internal/bot"
	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
	"github.com/felix-marchenay/coinchenet-core/internal/config"
	"github.com/felix-marchenay/coinchenet-core/internal/observabilite"
	"github.com/felix-marchenay/coinchenet-core/internal/simulation"
	"github.com/felix-marchenay/coinchenet-core/internal/stockage/postgres"
)

var version = "dev"

const usage = `usage : coinchenet [commande] [options]

Commandes :
  serveur   démarre le serveur de jeu (commande par défaut)
  migrer    applique les migrations de la base puis s'arrête
  simuler   fait jouer des parties entre bots (-parties, -graine, -regles, -profils)
  tournoi   oppose les profils de bots deux à deux et estime leur ELO (-parties, -graine, -regles)
  sante     interroge /sante d'un serveur (-url), pour les healthchecks Docker
  version   affiche la version`

func main() {
	if err := executer(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "coinchenet :", err)
		os.Exit(1)
	}
}

func executer(args []string) error {
	commande := "serveur"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		commande, args = args[0], args[1:]
	}
	switch commande {
	case "serveur":
		return serveur()
	case "migrer":
		return migrer()
	case "simuler":
		return simuler(args)
	case "tournoi":
		return tournoi(args)
	case "sante":
		return sante(args)
	case "version":
		fmt.Println(version)
		return nil
	case "aide", "-h", "--help":
		fmt.Println(usage)
		return nil
	default:
		return fmt.Errorf("commande inconnue %q\n\n%s", commande, usage)
	}
}

func serveur() error {
	cfg, err := config.Charger(os.Getenv)
	if err != nil {
		return fmt.Errorf("configuration invalide :\n%w", err)
	}
	log := observabilite.NouveauLogger(os.Stdout, cfg.LogNiveau, cfg.LogFormat)

	ctx, arreter := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer arreter()

	demarrage, annuler := context.WithTimeout(ctx, time.Minute)
	app, err := application.Demarrer(demarrage, cfg, log, version)
	annuler()
	if err != nil {
		return err
	}

	<-ctx.Done()
	fin, annulerFin := context.WithTimeout(context.Background(), cfg.ArretGracieux)
	defer annulerFin()
	return app.Arreter(fin)
}

func migrer() error {
	url := os.Getenv("COINCHE_POSTGRES_URL")
	if url == "" {
		return errors.New("COINCHE_POSTGRES_URL est obligatoire")
	}
	ctx, annuler := context.WithTimeout(context.Background(), 5*time.Minute)
	defer annuler()
	base, err := postgres.Ouvrir(ctx, url)
	if err != nil {
		return err
	}
	defer base.Fermer()
	if err := base.Migrer(ctx); err != nil {
		return err
	}
	fmt.Println("migrations appliquées")
	return nil
}

func simuler(args []string) error {
	fs := flag.NewFlagSet("simuler", flag.ContinueOnError)
	parties := fs.Int("parties", 1000, "nombre de parties à jouer")
	graine := fs.Uint64("graine", uint64(time.Now().UnixNano()), "graine du hasard (reproductible)")
	regles := fs.String("regles", "classees", "classees ou traditionnelles")
	profils := fs.String("profils", bot.Defaut.ID+","+bot.Defaut.ID, "profils des équipes A et B, séparés par une virgule")
	if err := fs.Parse(args); err != nil {
		return err
	}
	r, err := reglesNommees(*regles)
	if err != nil {
		return err
	}
	a, b, ok := strings.Cut(*profils, ",")
	pa, okA := bot.Trouver(a)
	pb, okB := bot.Trouver(b)
	if !ok || !okA || !okB {
		return fmt.Errorf("profils %q : deux profils parmi %s attendus", *profils, idsProfils())
	}
	rapport, err := simulation.SimulerEntre(*parties, *graine, r, [2]bot.Profil{pa, pb})
	if err != nil {
		return err
	}
	fmt.Printf("graine %d, %s contre %s\n%s\n", *graine, pa.Famille, pb.Famille, rapport)
	return nil
}

func tournoi(args []string) error {
	fs := flag.NewFlagSet("tournoi", flag.ContinueOnError)
	parties := fs.Int("parties", 2000, "nombre de parties par rencontre")
	graine := fs.Uint64("graine", uint64(time.Now().UnixNano()), "graine du hasard (reproductible)")
	regles := fs.String("regles", "classees", "classees ou traditionnelles")
	if err := fs.Parse(args); err != nil {
		return err
	}
	r, err := reglesNommees(*regles)
	if err != nil {
		return err
	}
	lignes, err := simulation.Tournoi(*parties, *graine, r, bot.Profils)
	if err != nil {
		return err
	}
	fmt.Printf("graine %d, %d parties par rencontre\n%s", *graine, *parties, simulation.FormaterTournoi(lignes))
	return nil
}

func reglesNommees(nom string) (coinche.Regles, error) {
	switch nom {
	case "classees":
		return coinche.ReglesClassees(), nil
	case "traditionnelles":
		return coinche.ReglesParDefaut(), nil
	}
	return coinche.Regles{}, fmt.Errorf("règles %q inconnues", nom)
}

func idsProfils() string {
	ids := make([]string, len(bot.Profils))
	for i, p := range bot.Profils {
		ids[i] = p.ID
	}
	return strings.Join(ids, ", ")
}

func sante(args []string) error {
	fs := flag.NewFlagSet("sante", flag.ContinueOnError)
	url := fs.String("url", "http://127.0.0.1:8080/sante", "adresse du contrôle de santé")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, annuler := context.WithTimeout(context.Background(), 3*time.Second)
	defer annuler()
	requete, err := http.NewRequestWithContext(ctx, http.MethodGet, *url, http.NoBody)
	if err != nil {
		return err
	}
	reponse, err := http.DefaultClient.Do(requete)
	if err != nil {
		return err
	}
	defer reponse.Body.Close()
	if reponse.StatusCode != http.StatusOK {
		return fmt.Errorf("statut %d", reponse.StatusCode)
	}
	return nil
}
