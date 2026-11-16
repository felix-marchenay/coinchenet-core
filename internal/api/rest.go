package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/felix-marchenay/coinchenet-core/internal/classement"
	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
	"github.com/felix-marchenay/coinchenet-core/internal/donnees"
	"github.com/felix-marchenay/coinchenet-core/internal/matchmaking"
	"github.com/felix-marchenay/coinchenet-core/internal/protocole"
	"github.com/felix-marchenay/coinchenet-core/internal/table"
)

func (s *Serveur) sante(w http.ResponseWriter, _ *http.Request) {
	ecrireJSON(w, http.StatusOK, map[string]string{"statut": "ok"})
}

func (s *Serveur) pret(w http.ResponseWriter, r *http.Request) {
	ctx, annuler := context.WithTimeout(r.Context(), 2*time.Second)
	defer annuler()
	resultats := map[string]string{}
	statut := http.StatusOK
	for _, v := range s.d.Verifications {
		if err := v.Verifier(ctx); err != nil {
			resultats[v.Nom] = err.Error()
			statut = http.StatusServiceUnavailable
			continue
		}
		resultats[v.Nom] = "ok"
	}
	ecrireJSON(w, statut, resultats)
}

func (s *Serveur) version(w http.ResponseWriter, _ *http.Request) {
	ecrireJSON(w, http.StatusOK, map[string]string{"version": s.d.Version})
}

func (s *Serveur) regles(w http.ResponseWriter, _ *http.Request) {
	ecrireJSON(w, http.StatusOK, map[string]any{
		"defaut":              table.OptionsParDefaut(),
		"classees":            table.OptionsClassees(),
		"objectif":            map[string]int{"min": coinche.ObjectifMin, "max": coinche.ObjectifMax, "pas": coinche.PasObjectif},
		"decomptes":           []coinche.Decompte{coinche.DecompteFaitsEtContrat, coinche.DecompteContratSeul},
		"distributions":       []coinche.Distribution{coinche.DistributionTraditionnelle, coinche.DistributionAleatoire},
		"delais_action":       table.DelaisAutorises,
		"messages_predefinis": table.MessagesPredefinis,
		"elo": map[string]int{
			"initial":          classement.EloInitial,
			"seuil_classement": classement.SeuilClassement,
			"penalite_abandon": classement.PenaliteAbandon,
		},
	})
}

func (s *Serveur) moi(w http.ResponseWriter, r *http.Request) {
	j, err := s.d.Joueurs.JoueurParID(r.Context(), joueurDe(r.Context()))
	if err != nil {
		s.erreurDonnees(w, err)
		return
	}
	reponse := struct {
		donnees.Joueur
		Table *uuid.UUID `json:"table,omitempty"`
	}{Joueur: j}
	if t, ok := tableActuelle(r.Context(), s.d, j.ID); ok {
		reponse.Table = &t
	}
	ecrireJSON(w, http.StatusOK, reponse)
}

func (s *Serveur) modifierMoi(w http.ResponseWriter, r *http.Request) {
	var corps struct {
		Pseudo string `json:"pseudo"`
	}
	if err := lireJSON(w, r, &corps); err != nil {
		ecrireErreur(w, http.StatusBadRequest, protocole.CodeRequeteInvalide, err.Error())
		return
	}
	j, err := s.d.Joueurs.Renommer(r.Context(), joueurDe(r.Context()), strings.TrimSpace(corps.Pseudo))
	if err != nil {
		s.erreurDonnees(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, j)
}

func (s *Serveur) chercherJoueur(w http.ResponseWriter, r *http.Request) {
	pseudo := strings.TrimSpace(r.URL.Query().Get("pseudo"))
	if pseudo == "" {
		ecrireErreur(w, http.StatusBadRequest, protocole.CodeRequeteInvalide, "paramètre pseudo requis")
		return
	}
	j, err := s.d.Joueurs.JoueurParPseudo(r.Context(), pseudo)
	if err != nil {
		s.erreurDonnees(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, j)
}

func (s *Serveur) joueur(w http.ResponseWriter, r *http.Request) {
	id, ok := idDuChemin(w, r)
	if !ok {
		return
	}
	j, err := s.d.Joueurs.JoueurParID(r.Context(), id)
	if err != nil {
		s.erreurDonnees(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, j)
}

func (s *Serveur) partiesDuJoueur(w http.ResponseWriter, r *http.Request) {
	id, ok := idDuChemin(w, r)
	if !ok {
		return
	}
	limite, ok := entierBorne(w, r, "limite", 20, 1, 100)
	if !ok {
		return
	}
	var avant *time.Time
	if v := r.URL.Query().Get("avant"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			ecrireErreur(w, http.StatusBadRequest, protocole.CodeRequeteInvalide, "avant doit être une date RFC 3339")
			return
		}
		avant = &t
	}
	parties, err := s.d.Historique.PartiesDuJoueur(r.Context(), id, avant, limite)
	if err != nil {
		s.erreurDonnees(w, err)
		return
	}
	reponse := map[string]any{"parties": parties}
	if len(parties) == limite {
		reponse["suivant"] = parties[len(parties)-1].Debut.Format(time.RFC3339Nano)
	}
	ecrireJSON(w, http.StatusOK, reponse)
}

func (s *Serveur) evolutionElo(w http.ResponseWriter, r *http.Request) {
	id, ok := idDuChemin(w, r)
	if !ok {
		return
	}
	limite, ok := entierBorne(w, r, "limite", 100, 1, 1000)
	if !ok {
		return
	}
	points, err := s.d.Historique.EvolutionElo(r.Context(), id, limite)
	if err != nil {
		s.erreurDonnees(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, map[string]any{"points": points})
}

func (s *Serveur) classement(w http.ResponseWriter, r *http.Request) {
	page, ok := entierBorne(w, r, "page", 1, 1, 100000)
	if !ok {
		return
	}
	taille, ok := entierBorne(w, r, "taille", 50, 1, 100)
	if !ok {
		return
	}
	c, err := s.d.Historique.Classement(r.Context(), page, taille)
	if err != nil {
		s.erreurDonnees(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, c)
}

func (s *Serveur) partie(w http.ResponseWriter, r *http.Request) {
	id, ok := idDuChemin(w, r)
	if !ok {
		return
	}
	p, err := s.d.Historique.Partie(r.Context(), id)
	if err != nil {
		s.erreurDonnees(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, p)
}

// evenements renvoie le replay complet d'une partie terminée, mains comprises.
// Une partie privée n'est visible que de ses joueurs ; une partie en cours ne l'est de personne.
func (s *Serveur) evenements(w http.ResponseWriter, r *http.Request) {
	id, ok := idDuChemin(w, r)
	if !ok {
		return
	}
	p, err := s.d.Historique.Partie(r.Context(), id)
	if err != nil {
		s.erreurDonnees(w, err)
		return
	}
	if p.Statut == "en_cours" {
		ecrireErreur(w, http.StatusForbidden, protocole.CodeInterdit, "le replay n'est disponible qu'une fois la partie terminée")
		return
	}
	if p.Type == string(table.Privee) {
		joueur, connecte := s.d.Auth.Authentifier(r)
		if !connecte || !p.Participe(joueur) {
			ecrireErreur(w, http.StatusForbidden, protocole.CodeInterdit, "replay réservé aux joueurs de cette partie privée")
			return
		}
	}
	evts, err := s.d.Journal.Charger(r.Context(), id)
	if err != nil {
		s.erreurDonnees(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, map[string]any{"partie": p, "evenements": evts})
}

func (s *Serveur) creerTable(w http.ResponseWriter, r *http.Request) {
	joueur := joueurDe(r.Context())
	if !s.limiteur.autoriser("tables:" + joueur.String()) {
		ecrireErreur(w, http.StatusTooManyRequests, protocole.CodeTropDeRequetes, "trop de tables créées, patientez un peu")
		return
	}
	options := table.OptionsParDefaut()
	if err := lireJSON(w, r, &options); err != nil {
		ecrireErreur(w, http.StatusBadRequest, protocole.CodeRequeteInvalide, err.Error())
		return
	}
	if err := options.Valider(); err != nil {
		ecrireErreur(w, http.StatusBadRequest, protocole.CodeRequeteInvalide, err.Error())
		return
	}
	if t, ok := tableActuelle(r.Context(), s.d, joueur); ok {
		ecrireJSON(w, http.StatusConflict, map[string]any{"erreur": protocole.Erreur{Code: "deja_a_une_table", Message: "vous êtes déjà à une table"}, "table": t})
		return
	}
	j, err := s.d.Joueurs.JoueurParID(r.Context(), joueur)
	if err != nil {
		s.erreurDonnees(w, err)
		return
	}
	moi := table.Joueur{ID: j.ID, Pseudo: j.Pseudo, Elo: j.Elo}
	id, code, err := s.d.Tables.CreerPrivee(r.Context(), moi, options)
	if err != nil {
		s.log.Error("création de table impossible", "erreur", err)
		ecrireErreur(w, http.StatusServiceUnavailable, protocole.CodeIndisponible, "création de table momentanément impossible")
		return
	}
	// On ne peut pas être à la fois à une table privée et dans la file classée.
	if err := s.d.File.Envoyer(r.Context(), matchmaking.Commande{Type: matchmaking.CmdQuitter, Joueur: moi}); err != nil {
		s.log.Warn("sortie de file impossible", "erreur", err)
	}
	ecrireJSON(w, http.StatusCreated, map[string]any{"table": id, "code": code, "options": options})
}

func (s *Serveur) erreurDonnees(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, donnees.ErrIntrouvable):
		ecrireErreur(w, http.StatusNotFound, protocole.CodeIntrouvable, "introuvable")
	case errors.Is(err, donnees.ErrPseudoInvalide):
		ecrireErreur(w, http.StatusBadRequest, "pseudo_invalide", err.Error())
	case errors.Is(err, donnees.ErrPseudoPris):
		ecrireErreur(w, http.StatusConflict, "pseudo_pris", err.Error())
	default:
		s.log.Error("accès aux données impossible", "erreur", err)
		ecrireErreur(w, http.StatusServiceUnavailable, protocole.CodeIndisponible, "service momentanément indisponible")
	}
}

func idDuChemin(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		ecrireErreur(w, http.StatusBadRequest, protocole.CodeRequeteInvalide, "identifiant invalide")
		return uuid.Nil, false
	}
	return id, true
}

func entierBorne(w http.ResponseWriter, r *http.Request, nom string, defaut, minimum, maximum int) (int, bool) {
	v := r.URL.Query().Get(nom)
	if v == "" {
		return defaut, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < minimum || n > maximum {
		ecrireErreur(w, http.StatusBadRequest, protocole.CodeRequeteInvalide, nom+" doit être un entier entre "+strconv.Itoa(minimum)+" et "+strconv.Itoa(maximum))
		return 0, false
	}
	return n, true
}
