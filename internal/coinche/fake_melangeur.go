package coinche

type FakeMelangeur struct {
	Cartes []Carte
}

func (f FakeMelangeur) Melanger(cc CarteCollection) CarteCollection {
	return cc
}
