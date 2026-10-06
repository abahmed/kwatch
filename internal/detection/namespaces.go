package detection

// systemNamespaces belong to the cluster itself. Their configuration
// risks are not the team's to fix, so no digest lists them.
var systemNamespaces = map[string]bool{
	"kube-system": true, "kube-public": true, "kube-node-lease": true,
}

// SystemNamespace reports a namespace the cluster owns.
func SystemNamespace(namespace string) bool {
	return systemNamespaces[namespace]
}
