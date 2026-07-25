package uds

// KeyGenerator computes a SecurityAccess key from an ECU-supplied seed
// (ISO 14229-1 §9.4). The real algorithm is always vendor/ECU-specific and
// confidential, so production code must supply its own implementation —
// XORKeyGenerator below exists only so SecurityAccess is exercisable against
// the simulated ECU in tests.
type KeyGenerator interface {
	ComputeKey(seed []byte) ([]byte, error)
}

// XORKeyGenerator XORs every seed byte with a fixed mask. It is not a real
// security algorithm; use it only for local/simulated testing.
type XORKeyGenerator struct {
	Mask byte
}

func (g XORKeyGenerator) ComputeKey(seed []byte) ([]byte, error) {
	key := make([]byte, len(seed))
	for i, b := range seed {
		key[i] = b ^ g.Mask
	}
	return key, nil
}
