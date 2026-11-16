package coinche

import (
	"errors"
	"fmt"
)

type Decompte int

const (
	// DecompteFaitsEtContrat : le preneur qui réussit marque ses points + le contrat.
	DecompteFaitsEtContrat Decompte = iota
	// DecompteContratSeul : seule la valeur du contrat est marquée.
	DecompteContratSeul
)

var nomsDecomptes = [...]string{DecompteFaitsEtContrat: "faits-et-contrat", DecompteContratSeul: "contrat-seul"}

func (d Decompte) Valide() bool {
	return d == DecompteFaitsEtContrat || d == DecompteContratSeul
}

func (d Decompte) String() string {
	if !d.Valide() {
		return fmt.Sprintf("decompte(%d)", int(d))
	}
	return nomsDecomptes[d]
}

func (d Decompte) MarshalText() ([]byte, error) {
	if !d.Valide() {
		return nil, fmt.Errorf("décompte invalide %d", int(d))
	}
	return []byte(d.String()), nil
}

func (d *Decompte) UnmarshalText(b []byte) error {
	for v, nom := range nomsDecomptes {
		if nom == string(b) {
			*d = Decompte(v)
			return nil
		}
	}
	return fmt.Errorf("décompte inconnu %q", string(b))
}

type Distribution int

const (
	// DistributionTraditionnelle : on ramasse les plis, on coupe et on redistribue sans mélanger.
	DistributionTraditionnelle Distribution = iota
	// DistributionAleatoire : le paquet est entièrement mélangé à chaque mène.
	DistributionAleatoire
)

var nomsDistributions = [...]string{DistributionTraditionnelle: "traditionnelle", DistributionAleatoire: "aleatoire"}

func (d Distribution) Valide() bool {
	return d == DistributionTraditionnelle || d == DistributionAleatoire
}

func (d Distribution) String() string {
	if !d.Valide() {
		return fmt.Sprintf("distribution(%d)", int(d))
	}
	return nomsDistributions[d]
}

func (d Distribution) MarshalText() ([]byte, error) {
	if !d.Valide() {
		return nil, fmt.Errorf("distribution invalide %d", int(d))
	}
	return []byte(d.String()), nil
}

func (d *Distribution) UnmarshalText(b []byte) error {
	for v, nom := range nomsDistributions {
		if nom == string(b) {
			*d = Distribution(v)
			return nil
		}
	}
	return fmt.Errorf("distribution inconnue %q", string(b))
}

type Regles struct {
	Objectif     int          `json:"objectif"`
	Decompte     Decompte     `json:"decompte"`
	Distribution Distribution `json:"distribution"`
}

const (
	ObjectifMin = 500
	ObjectifMax = 3000
	PasObjectif = 500
)

// ReglesClassees sont imposées en partie classée. Le mélange complet à chaque mène
// empêche de déduire les mains à partir de l'ordre des plis ramassés.
func ReglesClassees() Regles {
	return Regles{Objectif: 1000, Decompte: DecompteFaitsEtContrat, Distribution: DistributionAleatoire}
}

func ReglesParDefaut() Regles {
	return Regles{Objectif: 1000, Decompte: DecompteFaitsEtContrat, Distribution: DistributionTraditionnelle}
}

func (r Regles) Valider() error {
	var errs []error
	if r.Objectif < ObjectifMin || r.Objectif > ObjectifMax || r.Objectif%PasObjectif != 0 {
		errs = append(errs, fmt.Errorf("objectif %d invalide : entre %d et %d par pas de %d", r.Objectif, ObjectifMin, ObjectifMax, PasObjectif))
	}
	if !r.Decompte.Valide() {
		errs = append(errs, errors.New("décompte invalide"))
	}
	if !r.Distribution.Valide() {
		errs = append(errs, errors.New("distribution invalide"))
	}
	return errors.Join(errs...)
}
