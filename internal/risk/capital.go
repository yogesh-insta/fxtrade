package risk

// EffectiveCapital returns allocated capital when configured, otherwise the live account balance.
func EffectiveCapital(allocated, accountBalance float64) float64 {
	if allocated > 0 {
		return allocated
	}
	return accountBalance
}
