package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"golang.org/x/time/rate"

	"github.com/felix-marchenay/coinchenet-core/internal/matchmaking"
	"github.com/felix-marchenay/coinchenet-core/internal/protocole"
	"github.com/felix-marchenay/coinchenet-core/internal/table"
)

// Types des messages envoyés par le client.
const (
	CliPing           = "ping"
	CliFileRejoindre  = "file.rejoindre"
	CliFileQuitter    = "file.quitter"
	CliFileAccepter   = "file.accepter"
	CliFileRefuser    = "file.refuser"
	CliDuoInviter     = "duo.inviter"
	CliDuoRepondre    = "duo.repondre"
	CliTableRejoindre = "table.rejoindre"
	CliTableEtat      = "table.etat"
	CliTableSiege     = "table.siege"
	CliTableBot       = "table.bot"
	CliTableLiberer   = "table.liberer"
	CliTableLancer    = "table.lancer"
	CliTableQuitter   = "table.quitter"
	CliEncherir       = "partie.encherir"
	CliJouer          = "partie.jouer"
	CliAbandonner     = "partie.abandonner"
	CliPresent        = "partie.present"
	CliChat           = "partie.chat"
)

const (
	MsgBienvenue = "bienvenue"
	MsgPong      = "pong"
)

var commandesTable = map[string]table.TypeCommande{
	CliTableEtat:    table.CmdEtat,
	CliTableSiege:   table.CmdSiege,
	CliTableBot:     table.CmdBot,
	CliTableLiberer: table.CmdLiberer,
	CliTableLancer:  table.CmdLancer,
	CliTableQuitter: table.CmdQuitter,
	CliEncherir:     table.CmdEnchere,
	CliJouer:        table.CmdJouer,
	CliAbandonner:   table.CmdAbandonner,
	CliPresent:      table.CmdPresent,
	CliChat:         table.CmdChat,
}

const (
	tailleMaxMessage = 8 << 10
	// connexionsMaxParJoueur borne les onglets d'un même joueur sur une instance.
	connexionsMaxParJoueur = 5
	intervallePing         = 25 * time.Second
	delaiEcriture          = 10 * time.Second
	tampon                 = 256
)

type passerelle struct {
	d   Dependances
	log *slog.Logger

	mu         sync.Mutex
	connexions map[*connexion]struct{}
	parJoueur  map[uuid.UUID]int
	fermee     bool
}

func nouvellePasserelle(d Dependances, log *slog.Logger) *passerelle {
	return &passerelle{
		d:          d,
		log:        log.With("composant", "ws"),
		connexions: map[*connexion]struct{}{},
		parJoueur:  map[uuid.UUID]int{},
	}
}

func (p *passerelle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	joueurID, ok := p.d.Auth.AuthentifierWebSocket(r)
	if !ok {
		ecrireErreur(w, http.StatusUnauthorized, protocole.CodeNonAuthentifie, "connexion requise")
		return
	}
	profil, err := p.d.Joueurs.JoueurParID(r.Context(), joueurID)
	if err != nil {
		ecrireErreur(w, http.StatusUnauthorized, protocole.CodeNonAuthentifie, "compte introuvable")
		return
	}
	p.mu.Lock()
	trop := p.parJoueur[joueurID] >= connexionsMaxParJoueur
	p.mu.Unlock()
	if trop {
		ecrireErreur(w, http.StatusTooManyRequests, protocole.CodeTropDeRequetes, "trop de connexions ouvertes pour ce compte")
		return
	}
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: p.d.HotesWS})
	if err != nil {
		return
	}
	ws.SetReadLimit(tailleMaxMessage)

	c := &connexion{
		p:          p,
		id:         uuid.NewString(),
		joueur:     table.Joueur{ID: profil.ID, Pseudo: profil.Pseudo, Elo: profil.Elo},
		ws:         ws,
		entrees:    make(chan protocole.Enveloppe, 16),
		evenements: make(chan []byte, tampon),
		limite:     rate.NewLimiter(p.d.DebitWS, 2*int(p.d.DebitWS)),
	}
	if !p.ajouter(c) {
		_ = ws.Close(websocket.StatusGoingAway, "arrêt du serveur")
		return
	}
	c.executer(context.WithoutCancel(r.Context()))
	if p.retirer(c) == 0 && !p.estFermee() {
		// Plus aucun onglet ouvert sur cette instance : le joueur quitte la file classée.
		ctx, annuler := context.WithTimeout(context.Background(), 3*time.Second)
		defer annuler()
		if err := p.d.File.Envoyer(ctx, matchmaking.Commande{Type: matchmaking.CmdDeconnexion, Joueur: c.joueur}); err != nil {
			p.log.Warn("sortie de file impossible", "erreur", err)
		}
	}
}

func (p *passerelle) ajouter(c *connexion) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fermee {
		return false
	}
	p.connexions[c] = struct{}{}
	p.parJoueur[c.joueur.ID]++
	if p.d.Metriques != nil {
		p.d.Metriques.ConnexionOuverte()
	}
	return true
}

// retirer renvoie le nombre de connexions restantes du joueur sur cette instance.
func (p *passerelle) retirer(c *connexion) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.connexions, c)
	p.parJoueur[c.joueur.ID]--
	restantes := p.parJoueur[c.joueur.ID]
	if restantes <= 0 {
		delete(p.parJoueur, c.joueur.ID)
	}
	if p.d.Metriques != nil {
		p.d.Metriques.ConnexionFermee()
	}
	return restantes
}

// estFermee : à l'arrêt du serveur, les joueurs gardent leur place en file et se reconnectent ailleurs.
func (p *passerelle) estFermee() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fermee
}

func (p *passerelle) fermer() {
	p.mu.Lock()
	p.fermee = true
	cs := make([]*connexion, 0, len(p.connexions))
	for c := range p.connexions {
		cs = append(cs, c)
	}
	p.mu.Unlock()
	for _, c := range cs {
		_ = c.ws.Close(websocket.StatusGoingAway, "redémarrage du serveur")
	}
}

type connexion struct {
	p          *passerelle
	id         string
	joueur     table.Joueur
	ws         *websocket.Conn
	entrees    chan protocole.Enveloppe
	evenements chan []byte
	limite     *rate.Limiter
	table      uuid.UUID
	saturee    chan struct{}
	saturation sync.Once
}

func (c *connexion) executer(ctx context.Context) {
	ctx, annuler := context.WithCancel(ctx)
	defer annuler()
	c.saturee = make(chan struct{})

	desabonner, err := c.p.d.Bus.AbonnerJoueur(ctx, c.joueur.ID, c.recevoirDuBus)
	if err != nil {
		c.p.log.Error("abonnement du joueur impossible", "erreur", err)
		_ = c.ws.Close(websocket.StatusTryAgainLater, "service indisponible")
		return
	}
	defer desabonner()

	go c.lire(ctx, annuler)

	c.envoyer(ctx, protocole.Message{Type: MsgBienvenue, Data: map[string]any{"joueur": c.joueur, "connexion": c.id}})
	if t, ok := tableActuelle(ctx, c.p.d, c.joueur.ID); ok {
		c.rejoindre(ctx, t, "")
	}
	if err := c.p.d.File.Envoyer(ctx, matchmaking.Commande{Type: matchmaking.CmdEtat, Joueur: c.joueur}); err != nil {
		c.p.log.Warn("état de la file indisponible", "erreur", err)
	}

	ping := time.NewTicker(intervallePing)
	defer ping.Stop()
	defer c.quitterTable()

	for {
		select {
		case <-ctx.Done():
			_ = c.ws.Close(websocket.StatusNormalClosure, "")
			return
		case <-c.saturee:
			c.p.log.Warn("client trop lent, connexion fermée", "joueur", c.joueur.ID)
			_ = c.ws.Close(websocket.StatusPolicyViolation, "client trop lent")
			return
		case e := <-c.entrees:
			c.traiter(ctx, e)
		case b := <-c.evenements:
			c.relayer(ctx, b)
		case <-ping.C:
			ctxPing, annulerPing := context.WithTimeout(ctx, delaiEcriture)
			err := c.ws.Ping(ctxPing)
			annulerPing()
			if err != nil {
				return
			}
		}
	}
}

func (c *connexion) lire(ctx context.Context, annuler context.CancelFunc) {
	defer annuler()
	for {
		_, data, err := c.ws.Read(ctx)
		if err != nil {
			return
		}
		var e protocole.Enveloppe
		if err := json.Unmarshal(data, &e); err != nil || e.Type == "" {
			c.erreurAsync(protocole.NouvelleErreur("", protocole.CodeRequeteInvalide, "message JSON {type, data} attendu"))
			continue
		}
		select {
		case c.entrees <- e:
		case <-ctx.Done():
			return
		}
	}
}

// recevoirDuBus est appelé par la distribution du bus : il ne doit jamais bloquer.
func (c *connexion) recevoirDuBus(b []byte) {
	select {
	case c.evenements <- b:
	default:
		c.saturation.Do(func() { close(c.saturee) })
	}
}

func (c *connexion) erreurAsync(m protocole.Message) {
	b, err := json.Marshal(m)
	if err == nil {
		c.recevoirDuBus(b)
	}
}

var marqueurCommencee = []byte(`"` + table.MsgCommencee + `"`)

// relayer transmet un message du bus ; l'annonce d'une nouvelle partie rattache la connexion à sa table.
func (c *connexion) relayer(ctx context.Context, b []byte) {
	if bytes.Contains(b, marqueurCommencee) {
		var m struct {
			Type string                 `json:"type"`
			Data table.MessageCommencee `json:"data"`
		}
		if json.Unmarshal(b, &m) == nil && m.Type == table.MsgCommencee {
			c.ecrire(ctx, b)
			c.rejoindre(ctx, m.Data.Table, "")
			return
		}
	}
	c.ecrire(ctx, b)
}

func (c *connexion) ecrire(ctx context.Context, b []byte) {
	ctx, annuler := context.WithTimeout(ctx, delaiEcriture)
	defer annuler()
	if err := c.ws.Write(ctx, websocket.MessageText, b); err != nil {
		c.saturation.Do(func() { close(c.saturee) })
	}
}

func (c *connexion) envoyer(ctx context.Context, m protocole.Message) {
	b, err := json.Marshal(m)
	if err != nil {
		c.p.log.Error("sérialisation impossible", "type", m.Type, "erreur", err)
		return
	}
	c.ecrire(ctx, b)
}

func (c *connexion) erreur(ctx context.Context, ref, code, message string) {
	c.envoyer(ctx, protocole.NouvelleErreur(ref, code, message))
}

func (c *connexion) traiter(ctx context.Context, e protocole.Enveloppe) {
	if !c.limite.Allow() {
		c.erreur(ctx, e.Ref, protocole.CodeTropDeRequetes, "trop de messages, ralentissez")
		return
	}

	if typ, ok := commandesTable[e.Type]; ok {
		if c.table == uuid.Nil {
			c.erreur(ctx, e.Ref, "aucune_table", "vous n'êtes à aucune table")
			return
		}
		c.envoyerTable(ctx, c.table, table.Commande{Type: typ, Ref: e.Ref, Donnees: e.Data})
		return
	}

	switch e.Type {
	case CliPing:
		c.envoyer(ctx, protocole.Message{Type: MsgPong, Ref: e.Ref})
	case CliTableRejoindre:
		c.rejoindreDemande(ctx, e)
	case CliFileRejoindre, CliFileQuitter, CliFileAccepter, CliFileRefuser, CliDuoInviter, CliDuoRepondre:
		c.file(ctx, e)
	default:
		c.erreur(ctx, e.Ref, protocole.CodeRequeteInvalide, "type de message inconnu : "+e.Type)
	}
}

func (c *connexion) rejoindreDemande(ctx context.Context, e protocole.Enveloppe) {
	var d struct {
		Table *uuid.UUID `json:"table"`
		Code  string     `json:"code"`
	}
	if err := decoderDonnees(e.Data, &d); err != nil || (d.Table == nil && d.Code == "") {
		c.erreur(ctx, e.Ref, protocole.CodeRequeteInvalide, "table ou code attendu")
		return
	}
	var id uuid.UUID
	if d.Table != nil {
		id = *d.Table
	} else {
		t, ok, err := c.p.d.Tables.ResoudreCode(ctx, d.Code)
		switch {
		case err != nil:
			c.erreur(ctx, e.Ref, protocole.CodeIndisponible, "service momentanément indisponible")
			return
		case !ok:
			c.erreur(ctx, e.Ref, protocole.CodeIntrouvable, "aucune table avec ce code")
			return
		}
		id = t
	}
	if actuelle, ok := tableActuelle(ctx, c.p.d, c.joueur.ID); ok && actuelle != id {
		c.erreur(ctx, e.Ref, "deja_a_une_table", "vous êtes déjà à une autre table")
		return
	}
	// Rejoindre une table privée fait quitter la file classée.
	if err := c.p.d.File.Envoyer(ctx, matchmaking.Commande{Type: matchmaking.CmdQuitter, Joueur: c.joueur}); err != nil {
		c.p.log.Warn("sortie de file impossible", "erreur", err)
	}
	c.rejoindre(ctx, id, e.Ref)
}

func (c *connexion) rejoindre(ctx context.Context, t uuid.UUID, ref string) {
	if c.table != uuid.Nil && c.table != t {
		c.quitterTable()
	}
	c.table = t
	c.envoyerTable(ctx, t, table.Commande{Type: table.CmdConnexion, Ref: ref})
}

func (c *connexion) quitterTable() {
	if c.table == uuid.Nil {
		return
	}
	ctx, annuler := context.WithTimeout(context.Background(), 3*time.Second)
	defer annuler()
	c.envoyerTable(ctx, c.table, table.Commande{Type: table.CmdDeconnexion})
	c.table = uuid.Nil
}

func (c *connexion) envoyerTable(ctx context.Context, t uuid.UUID, cmd table.Commande) {
	cmd.Joueur = c.joueur
	cmd.Connexion = c.id
	if err := c.p.d.Tables.Envoyer(ctx, t, cmd); err != nil {
		c.p.log.Warn("commande de table non transmise", "table", t, "erreur", err)
		c.erreur(ctx, cmd.Ref, protocole.CodeIndisponible, "table momentanément injoignable")
	}
}

func (c *connexion) file(ctx context.Context, e protocole.Enveloppe) {
	var d struct {
		Proposition string `json:"proposition"`
		Pseudo      string `json:"pseudo"`
		Invitation  string `json:"invitation"`
		Accepte     bool   `json:"accepte"`
	}
	if len(e.Data) > 0 {
		if err := decoderDonnees(e.Data, &d); err != nil {
			c.erreur(ctx, e.Ref, protocole.CodeRequeteInvalide, "données invalides")
			return
		}
	}
	cmd := matchmaking.Commande{Ref: e.Ref, Proposition: d.Proposition, Invitation: d.Invitation, Accepte: d.Accepte}

	switch e.Type {
	case CliFileQuitter:
		cmd.Type = matchmaking.CmdQuitter
	case CliFileAccepter:
		if _, ok := tableActuelle(ctx, c.p.d, c.joueur.ID); ok {
			c.erreur(ctx, e.Ref, "deja_a_une_table", "terminez ou quittez votre table avant de jouer en classé")
			return
		}
		cmd.Type = matchmaking.CmdAccepter
	case CliFileRefuser:
		cmd.Type = matchmaking.CmdRefuser
	case CliFileRejoindre, CliDuoRepondre, CliDuoInviter:
		if _, ok := tableActuelle(ctx, c.p.d, c.joueur.ID); ok {
			c.erreur(ctx, e.Ref, "deja_a_une_table", "terminez ou quittez votre table avant de rejoindre la file")
			return
		}
		if !c.rafraichirProfil(ctx, e.Ref) {
			return
		}
		cmd.Type = map[string]matchmaking.TypeCommande{
			CliFileRejoindre: matchmaking.CmdRejoindre,
			CliDuoRepondre:   matchmaking.CmdRepondre,
			CliDuoInviter:    matchmaking.CmdInviter,
		}[e.Type]
		if e.Type == CliDuoInviter {
			invite, err := c.p.d.Joueurs.JoueurParPseudo(ctx, d.Pseudo)
			if err != nil {
				c.erreur(ctx, e.Ref, protocole.CodeIntrouvable, "joueur introuvable")
				return
			}
			cmd.Invite = &table.Joueur{ID: invite.ID, Pseudo: invite.Pseudo, Elo: invite.Elo}
		}
	}

	cmd.Joueur = c.joueur
	if err := c.p.d.File.Envoyer(ctx, cmd); err != nil {
		c.p.log.Warn("commande de file non transmise", "erreur", err)
		c.erreur(ctx, e.Ref, protocole.CodeIndisponible, "file momentanément indisponible")
	}
}

// rafraichirProfil recharge l'ELO avant d'entrer en file, il a pu changer depuis la connexion.
func (c *connexion) rafraichirProfil(ctx context.Context, ref string) bool {
	j, err := c.p.d.Joueurs.JoueurParID(ctx, c.joueur.ID)
	if err != nil {
		c.erreur(ctx, ref, protocole.CodeIndisponible, "profil momentanément indisponible")
		return false
	}
	c.joueur = table.Joueur{ID: j.ID, Pseudo: j.Pseudo, Elo: j.Elo}
	return true
}

func decoderDonnees(data json.RawMessage, v any) error {
	if len(data) == 0 {
		return errors.New("données absentes")
	}
	return json.Unmarshal(data, v)
}
