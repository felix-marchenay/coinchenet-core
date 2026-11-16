package coinche

import (
	"math/rand/v2"
	"sync"
)

// HasardDeterministe produit une suite reproductible, pour les tests et les simulations. Comme
// HasardCrypto, il peut être partagé entre les tables.
type HasardDeterministe struct {
	mu sync.Mutex
	r  *rand.Rand
}

func NouveauHasardDeterministe(graine uint64) *HasardDeterministe {
	return &HasardDeterministe{r: rand.New(rand.NewPCG(graine, graine^0x9e3779b97f4a7c15))}
}

func (h *HasardDeterministe) Intn(n int) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.r.IntN(n)
}
