package pow

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const HashAlg = "sha256"

func Mine(nodeID string, k int) (nonce uint64, digestHex string) {
	if k <= 0 {
		return 0, hashHex(nodeID, 0)
	}
	prefix := strings.Repeat("0", k)
	for n := uint64(0); ; n++ {
		d := hashHex(nodeID, n)
		if strings.HasPrefix(d, prefix) {
			return n, d
		}
	}
}

func Verify(nodeID string, nonce uint64, digestHex string, k int) bool {
	if k <= 0 {
		return true
	}
	computed := hashHex(nodeID, nonce)
	if computed != digestHex {
		return false
	}
	return strings.HasPrefix(computed, strings.Repeat("0", k))
}

func hashHex(nodeID string, nonce uint64) string {
	input := fmt.Sprintf("%s%d", nodeID, nonce)
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}
