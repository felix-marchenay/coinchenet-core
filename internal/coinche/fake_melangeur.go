package coinche

// FakeMelangeur renvoie Cartes si elles sont fournies, sinon le paquet tel quel.
type FakeMelangeur struct {
	Cartes []Carte
}

func (f FakeMelangeur) Melanger(cc CarteCollection) CarteCollection {
	if len(f.Cartes) > 0 {
		return CarteCollection{Cartes: append([]Carte(nil), f.Cartes...)}
	}
	return cc
}
