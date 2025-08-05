package test

import (
	"coinchenetcore/src"
)

type FakeMelangeur struct {
	Cartes []src.Carte
}

func (f *FakeMelangeur) Melanger(cc src.CarteCollection) src.CarteCollection {
	return cc
}
