package coinche

type NoCoupeur struct{}

func (NoCoupeur) Couper(c CarteCollection) CarteCollection {
	return c
}
