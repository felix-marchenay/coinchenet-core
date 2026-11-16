package table

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/felix-marchenay/coinchenet-core/internal/coinche"
	"github.com/felix-marchenay/coinchenet-core/internal/protocole"
)

type occupant struct {
	joueur Joueur
	bot    bool
	// profil du bot assis ; vide pour un humain.
	profil string
	// remplacant : profil du bot qui joue pour un humain absent, tiré au hasard et gardé jusqu'à la fin.
	remplacant    string
	abandon       bool
	absent        bool
	delaisExpires int
	connexions    map[string]struct{}
	jetonRetour   int64
	retourAvant   time.Time
}

func (o *occupant) humain() bool {
	return !o.bot
}

func (o *occupant) connecte() bool {
	return len(o.connexions) > 0
}

// automatique : le bot joue pour ce siège.
func (o *occupant) automatique() bool {
	return o.bot || o.abandon || o.absent || !o.connecte()
}

func (o *occupant) statut() string {
	switch {
	case o.bot:
		return SiegeBot
	case o.abandon:
		return SiegeAbandon
	case !o.connecte():
		return SiegeDeconnecte
	case o.absent:
		return SiegeAbsent
	default:
		return SiegeConnecte
	}
}

type typeInterne int

const (
	interneDonne typeInterne = iota
	interneBot
	interneDelai
	interneRetour
	interneFermeture
	interneLobby
)

type interne struct {
	typ   typeInterne
	jeton int64
	siege coinche.Siege
}

type Table struct {
	id        uuid.UUID
	typ       Type
	options   Options
	code      string
	createur  uuid.UUID
	statut    Statut
	sieges    [coinche.NbSieges]*occupant
	partie    *coinche.Partie
	fin       *FinPartie
	seq       int64
	jeton     int64
	echeance  time.Time
	restauree bool
	ferme     bool
	chats     map[uuid.UUID][]time.Time
	// exclus : joueurs retirés par le créateur, qui ne peuvent plus revenir au salon.
	exclus map[uuid.UUID]bool
	// demarree reçoit le résultat du lancement d'une partie classée.
	demarree chan error

	externes chan Commande
	internes chan interne
	arret    chan struct{}

	deps Dependances
	cfg  Config
	log  *slog.Logger
}

func nouvelleTable(id uuid.UUID, typ Type, options Options, deps Dependances, cfg Config) *Table {
	deps = deps.completer()
	return &Table{
		id:       id,
		typ:      typ,
		options:  options,
		statut:   StatutAttente,
		chats:    map[uuid.UUID][]time.Time{},
		exclus:   map[uuid.UUID]bool{},
		demarree: make(chan error, 1),
		externes: make(chan Commande, 128),
		internes: make(chan interne, 32),
		arret:    make(chan struct{}),
		deps:     deps,
		cfg:      cfg,
		log:      deps.Logger.With("table", id, "type", typ),
	}
}

// NouvelleTablePrivee crée une table en attente de joueurs ; le créateur occupe le siège 0.
func NouvelleTablePrivee(id uuid.UUID, createur Joueur, code string, options Options, deps Dependances, cfg Config) (*Table, error) {
	if err := options.Valider(); err != nil {
		return nil, err
	}
	t := nouvelleTable(id, Privee, options, deps, cfg)
	t.code = code
	t.createur = createur.ID
	t.sieges[0] = &occupant{joueur: createur, connexions: map[string]struct{}{}}
	return t, nil
}

// NouvelleTableClassee crée la table d'une partie de la file : l'équipe A aux sièges 0 et 2, l'équipe B
// aux sièges 1 et 3. Un Joueur zéro laisse son siège à un bot, et la partie est alors AvecBots.
func NouvelleTableClassee(id uuid.UUID, equipeA, equipeB [2]Joueur, options Options, deps Dependances, cfg Config) (*Table, error) {
	if err := options.Valider(); err != nil {
		return nil, err
	}
	joueurs := [...]Joueur{equipeA[0], equipeB[0], equipeA[1], equipeB[1]}
	typ := Classee
	if slices.ContainsFunc(joueurs[:], func(j Joueur) bool { return j.ID == uuid.Nil }) {
		typ = AvecBots
	}
	t := nouvelleTable(id, typ, options, deps, cfg)
	for i, j := range joueurs {
		if j.ID != uuid.Nil {
			t.sieges[i] = &occupant{joueur: j, connexions: map[string]struct{}{}}
		}
	}
	for i, o := range t.sieges {
		if o != nil {
			continue
		}
		profil, nom, err := t.nouveauBot("")
		if err != nil {
			return nil, err
		}
		t.sieges[i] = &occupant{bot: true, profil: profil, joueur: Joueur{Pseudo: nom}}
	}
	return t, nil
}

func (t *Table) ID() uuid.UUID {
	return t.id
}

func (t *Table) Type() Type {
	return t.typ
}

func (t *Table) Code() string {
	return t.code
}

// Arretee est fermé quand l'acteur s'est arrêté.
func (t *Table) Arretee() <-chan struct{} {
	return t.arret
}

// Envoyer transmet une commande à l'acteur sans bloquer.
func (t *Table) Envoyer(c Commande) bool {
	select {
	case t.externes <- c:
		return true
	case <-t.arret:
		return false
	default:
		t.log.Warn("file de commandes saturée", "commande", c.Type)
		return false
	}
}

// Executer fait tourner l'acteur jusqu'à la fermeture de la table ou l'annulation du contexte.
func (t *Table) Executer(ctx context.Context) {
	defer close(t.arret)
	t.demarrer(ctx)
	for !t.ferme {
		select {
		case <-ctx.Done():
			// Un salon n'est pas journalisé : il disparaît avec l'instance, ses joueurs sont libérés.
			// Une partie en cours, elle, sera reprise par une autre instance.
			if t.statut == StatutAttente {
				t.fermerLobby(context.WithoutCancel(ctx), "arret_serveur")
			}
			return
		case c := <-t.externes:
			debut := time.Now()
			if err := t.executer(ctx, c); err != nil {
				t.signalerErreur(ctx, c, err)
			}
			if t.deps.Metriques != nil {
				t.deps.Metriques.ActionTraitee(string(c.Type), time.Since(debut))
			}
		case i := <-t.internes:
			t.traiterInterne(ctx, i)
		}
	}
}

func (t *Table) demarrer(ctx context.Context) {
	switch {
	case t.statut == StatutAttente && t.typ == Privee:
		t.associer(ctx, t.sieges[0])
		t.apres(t.cfg.DelaiLobby, interne{typ: interneLobby})
	case t.statut == StatutAttente:
		err := t.commencer(ctx)
		t.demarree <- err
		if err != nil {
			t.log.Error("lancement de la partie classée impossible", "erreur", err)
			t.ferme = true
		}
	case t.restauree:
		t.reprendre(ctx)
	}
}

func (t *Table) executer(ctx context.Context, c Commande) error {
	switch c.Type {
	case CmdConnexion:
		return t.connexion(ctx, c)
	case CmdDeconnexion:
		t.deconnexion(ctx, c)
		return nil
	case CmdEtat:
		if _, ok := t.siegeDe(c.Joueur.ID); !ok {
			return ErrNonAssis
		}
		t.envoyerEtat(ctx, c.Joueur.ID)
		return nil
	}

	// Une commande venant d'une connexion inconnue (après une reprise, par exemple) la rattache.
	if s, ok := t.siegeDe(c.Joueur.ID); ok && !t.sieges[s].abandon && t.rattacher(s, c.Connexion) {
		t.apresRetour(ctx, s)
	}

	if c.Type == CmdChat {
		return t.chat(ctx, c)
	}

	switch t.statut {
	case StatutAttente:
		return t.executerLobby(ctx, c)
	case StatutEnCours:
		return t.executerJeu(ctx, c)
	default:
		return ErrPartieFinie
	}
}

func (t *Table) traiterInterne(ctx context.Context, i interne) {
	switch i.typ {
	case interneFermeture:
		t.ferme = true
	case interneLobby:
		if t.statut == StatutAttente {
			t.fermerLobby(ctx, "expiree")
		}
	case interneRetour:
		o := t.sieges[i.siege]
		if t.statut == StatutEnCours && o != nil && o.jetonRetour == i.jeton && !o.connecte() && !o.abandon {
			t.log.Info("joueur non revenu à temps", "siege", i.siege, "joueur", o.joueur.ID)
			if err := t.abandonner(ctx, i.siege); err != nil {
				t.log.Error("forfait impossible", "erreur", err)
			}
		}
	default:
		if i.jeton != t.jeton || t.statut != StatutEnCours {
			return
		}
		t.traiterInterneJeu(ctx, i)
	}
}

func (t *Table) signalerErreur(ctx context.Context, c Commande, err error) {
	code, message := codeErreur(err)
	if code == protocole.CodeInterne {
		t.log.Error("commande en échec", "commande", c.Type, "joueur", c.Joueur.ID, "erreur", err)
	}
	if c.Joueur.ID != uuid.Nil {
		t.deps.Diffuseur.Envoyer(ctx, c.Joueur.ID, protocole.NouvelleErreur(c.Ref, code, message))
	}
}

// apres programme un événement interne ; il est ignoré s'il arrive après l'arrêt de l'acteur.
func (t *Table) apres(d time.Duration, i interne) {
	time.AfterFunc(d, func() {
		select {
		case t.internes <- i:
		case <-t.arret:
		}
	})
}

func (t *Table) maintenant() time.Time {
	return t.deps.Maintenant()
}

func (t *Table) siegeDe(joueur uuid.UUID) (coinche.Siege, bool) {
	for _, s := range coinche.Sieges {
		if o := t.sieges[s]; o != nil && o.humain() && o.joueur.ID == joueur {
			return s, true
		}
	}
	return 0, false
}

func (t *Table) humains() []*occupant {
	var hs []*occupant
	for _, o := range t.sieges {
		if o != nil && o.humain() {
			hs = append(hs, o)
		}
	}
	return hs
}

func (t *Table) contexteStockage(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), t.cfg.TimeoutStockage)
}

func (t *Table) evenement(typ TypeEvenement, donnees any) (Evenement, error) {
	t.seq++
	return nouvelEvenement(t.seq, typ, donnees, t.maintenant())
}

// persister journalise les événements. En cas d'échec l'état en mémoire n'est plus fiable :
// la table s'arrête et sera reconstruite depuis le journal.
func (t *Table) persister(ctx context.Context, evts ...Evenement) error {
	ctx, annuler := t.contexteStockage(ctx)
	defer annuler()
	if err := t.deps.Journal.Ajouter(ctx, t.id, evts); err != nil {
		t.panne(fmt.Errorf("journalisation : %w", err))
		return ErrStockageIndisponible
	}
	return nil
}

func (t *Table) panne(err error) {
	t.log.Error("table arrêtée sur erreur, elle sera reprise depuis le journal", "erreur", err)
	t.ferme = true
}

func (t *Table) envoyer(ctx context.Context, joueur uuid.UUID, m protocole.Message) {
	t.deps.Diffuseur.Envoyer(ctx, joueur, m)
}

// diffuser envoie un message à tous les joueurs humains encore à la table.
func (t *Table) diffuser(ctx context.Context, m protocole.Message) {
	for _, o := range t.humains() {
		if !o.abandon {
			t.envoyer(ctx, o.joueur.ID, m)
		}
	}
}

func (t *Table) diffuserEvenements(ctx context.Context, evts []Evenement) {
	for _, e := range evts {
		if m, ok := messagePublic(t.id, e); ok {
			t.diffuser(ctx, m)
		}
	}
}

func (t *Table) diffuserStatut(ctx context.Context, s coinche.Siege) {
	o := t.sieges[s]
	m := MessageStatut{Entete: Entete{Table: t.id, Seq: t.seq}, Siege: s, Statut: o.statut()}
	if !o.retourAvant.IsZero() && !o.connecte() {
		r := o.retourAvant
		m.RetourAvant = &r
	}
	t.diffuser(ctx, protocole.Message{Type: MsgStatut, Data: m})
}

func (t *Table) associer(ctx context.Context, o *occupant) {
	if o == nil || !o.humain() {
		return
	}
	ctx, annuler := t.contexteStockage(ctx)
	defer annuler()
	if err := t.deps.Annuaire.Associer(ctx, o.joueur.ID, t.id); err != nil {
		t.log.Warn("annuaire indisponible", "erreur", err)
	}
}

func (t *Table) dissocier(ctx context.Context, o *occupant) {
	if o == nil || !o.humain() {
		return
	}
	ctx, annuler := t.contexteStockage(ctx)
	defer annuler()
	if err := t.deps.Annuaire.Dissocier(ctx, o.joueur.ID, t.id); err != nil {
		t.log.Warn("annuaire indisponible", "erreur", err)
	}
}

// Etat construit l'instantané destiné à un joueur.
func (t *Table) etatPour(joueur uuid.UUID) EtatTable {
	e := EtatTable{
		Entete:  Entete{Table: t.id, Seq: t.seq},
		Type:    t.typ,
		Statut:  t.statut,
		Code:    t.code,
		Options: t.options,
		Fin:     t.fin,
	}
	if t.createur != uuid.Nil {
		c := t.createur
		e.Createur = &c
	}
	if !t.echeance.IsZero() {
		ech := t.echeance
		e.Echeance = &ech
	}
	for s, o := range t.sieges {
		if o == nil {
			continue
		}
		es := &EtatSiege{Bot: o.bot, Profil: o.profil, Statut: o.statut()}
		if o.humain() {
			j := o.joueur
			es.Joueur = &j
		} else {
			es.Joueur = &Joueur{Pseudo: o.joueur.Pseudo}
		}
		if !o.retourAvant.IsZero() && !o.connecte() {
			r := o.retourAvant
			es.RetourAvant = &r
		}
		e.Sieges[s] = es
	}
	if s, ok := t.siegeDe(joueur); ok {
		e.Moi = &s
		if t.partie != nil {
			v := t.partie.Vue(s)
			e.Jeu = &v
		}
	}
	return e
}

func (t *Table) envoyerEtat(ctx context.Context, joueur uuid.UUID) {
	t.envoyer(ctx, joueur, protocole.Message{Type: MsgEtat, Data: t.etatPour(joueur)})
}

func (t *Table) diffuserEtat(ctx context.Context) {
	for _, o := range t.humains() {
		if !o.abandon {
			t.envoyerEtat(ctx, o.joueur.ID)
		}
	}
}

func (t *Table) connexion(ctx context.Context, c Commande) error {
	s, assis := t.siegeDe(c.Joueur.ID)
	if !assis {
		if t.statut != StatutAttente {
			return ErrNonAssis
		}
		if t.exclus[c.Joueur.ID] {
			return ErrExclu
		}
		libre, ok := t.siegeLibre()
		if !ok {
			return ErrTableComplete
		}
		t.sieges[libre] = &occupant{joueur: c.Joueur, connexions: map[string]struct{}{}}
		t.associer(ctx, t.sieges[libre])
		s = libre
	}

	if t.sieges[s].abandon {
		return ErrNonAssis
	}
	revenu := t.rattacher(s, c.Connexion)

	if t.statut != StatutEnCours {
		t.diffuserEtat(ctx)
		return nil
	}
	t.envoyerEtat(ctx, c.Joueur.ID)
	if revenu {
		t.apresRetour(ctx, s)
	}
	return nil
}

// rattacher enregistre une connexion du joueur assis en s et indique s'il était déconnecté.
func (t *Table) rattacher(s coinche.Siege, connexion string) bool {
	o := t.sieges[s]
	if connexion == "" {
		return false
	}
	if _, connue := o.connexions[connexion]; connue {
		return false
	}
	revenu := !o.connecte()
	o.connexions[connexion] = struct{}{}
	if revenu {
		o.jetonRetour++
		o.retourAvant = time.Time{}
	}
	return revenu
}

func (t *Table) apresRetour(ctx context.Context, s coinche.Siege) {
	if t.statut != StatutEnCours {
		return
	}
	t.diffuserStatut(ctx, s)
	if tour, _, ok := t.partie.Tour(); ok && tour == s && !t.sieges[s].automatique() {
		t.planifier(ctx)
	}
}

func (t *Table) deconnexion(ctx context.Context, c Commande) {
	s, ok := t.siegeDe(c.Joueur.ID)
	if !ok {
		return
	}
	o := t.sieges[s]
	if _, existe := o.connexions[c.Connexion]; !existe {
		return
	}
	delete(o.connexions, c.Connexion)
	if o.connecte() {
		return
	}

	switch t.statut {
	case StatutAttente:
		t.diffuserEtat(ctx)
	case StatutEnCours:
		if o.abandon {
			return
		}
		t.lancerDelaiRetour(s)
		t.diffuserStatut(ctx, s)
		if tour, _, ok := t.partie.Tour(); ok && tour == s {
			t.planifier(ctx)
		}
	}
}

// lancerDelaiRetour : en partie classée, un joueur déconnecté a un temps limité pour revenir.
func (t *Table) lancerDelaiRetour(s coinche.Siege) {
	if t.typ != Classee {
		return
	}
	o := t.sieges[s]
	o.jetonRetour++
	o.retourAvant = t.maintenant().Add(t.cfg.DelaiRetour)
	t.apres(t.cfg.DelaiRetour, interne{typ: interneRetour, jeton: o.jetonRetour, siege: s})
}

func (t *Table) siegeLibre() (coinche.Siege, bool) {
	for _, s := range coinche.Sieges {
		if t.sieges[s] == nil {
			return s, true
		}
	}
	return 0, false
}

func (t *Table) chat(ctx context.Context, c Commande) error {
	s, ok := t.siegeDe(c.Joueur.ID)
	if !ok {
		return ErrNonAssis
	}
	d, err := decoder[DonneesChat](c)
	if err != nil {
		return err
	}
	msg, err := validerChat(t.typ, d)
	if err != nil {
		return err
	}
	if !t.autoriserChat(c.Joueur.ID) {
		return ErrChatTropRapide
	}

	msg.Siege = s
	msg.Pseudo = t.sieges[s].joueur.Pseudo
	if t.statut == StatutAttente {
		t.diffuser(ctx, protocole.Message{Type: MsgChat, Data: struct {
			Table uuid.UUID `json:"table"`
			Chat
		}{t.id, msg}})
		return nil
	}

	evt, err := t.evenement(EvtChat, msg)
	if err != nil {
		return err
	}
	if err := t.persister(ctx, evt); err != nil {
		return err
	}
	t.diffuserEvenements(ctx, []Evenement{evt})
	return nil
}

func (t *Table) autoriserChat(joueur uuid.UUID) bool {
	limite := t.maintenant().Add(-t.cfg.ChatFenetre)
	recents := t.chats[joueur][:0]
	for _, d := range t.chats[joueur] {
		if d.After(limite) {
			recents = append(recents, d)
		}
	}
	if len(recents) >= t.cfg.ChatMax {
		t.chats[joueur] = recents
		return false
	}
	t.chats[joueur] = append(recents, t.maintenant())
	return true
}

func validerChat(typ Type, d DonneesChat) (Chat, error) {
	if d.Message != "" {
		for _, m := range MessagesPredefinis {
			if m == d.Message {
				return Chat{Message: d.Message}, nil
			}
		}
		return Chat{}, ErrChatInvalide
	}
	if typ != Privee {
		return Chat{}, erreur("chat_restreint", "seuls les messages prédéfinis sont autorisés dans les parties de la file")
	}
	texte := nettoyerTexte(d.Texte)
	if texte == "" || len([]rune(texte)) > 500 {
		return Chat{}, ErrChatInvalide
	}
	return Chat{Texte: texte}, nil
}

// nettoyerTexte retire les caractères de contrôle (hors retours à la ligne) et les espaces de bord.
func nettoyerTexte(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '\n' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, strings.ToValidUTF8(s, "")))
}
