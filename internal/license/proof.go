package license

import (
	"crypto/sha256"
	"encoding/hex"
)

// ClusterProof binds a license to a cluster without revealing the
// kube-system namespace UID: hex sha256(uid + ":" + lid).
func ClusterProof(kubeSystemUID, lid string) string {
	sum := sha256.Sum256([]byte(kubeSystemUID + ":" + lid))
	return hex.EncodeToString(sum[:])
}
