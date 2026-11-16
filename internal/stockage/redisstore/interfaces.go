package redisstore

import (
	"github.com/felix-marchenay/coinchenet-core/internal/matchmaking"
	"github.com/felix-marchenay/coinchenet-core/internal/table"
)

var (
	_ table.Verrous        = (*Verrous)(nil)
	_ table.BusTables      = (*Bus)(nil)
	_ table.Diffuseur      = (*Diffuseur)(nil)
	_ table.Annuaire       = (*Annuaire)(nil)
	_ table.Codes          = (*Codes)(nil)
	_ matchmaking.Source   = (*FileMatchmaking)(nil)
	_ matchmaking.Stockage = (*EtatMatchmaking)(nil)
)
