package coinche

import "math/rand/v2"

// HasardDeterministe produit une suite reproductible, pour les tests et les simulations.
type HasardDeterministe struct {
	r *rand.Rand
}

func NouveauHasardDeterministe(graine uint64) *HasardDeterministe {
	return &HasardDeterministe{r: rand.New(rand.NewPCG(graine, graine^0x9e3779b97f4a7c15))}
}

func (h *HasardDeterministe) Intn(n int) int {
	return h.r.IntN(n)
}
