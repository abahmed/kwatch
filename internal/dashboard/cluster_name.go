package dashboard

// ClusterNameInfo is what the kwatch page says about the cluster's
// name. Name is "" when no name is known; Source then is "" too and
// Notice says how to set one. With a name, Source says where it came
// from ("configured", or "from the node label ...") and is "" only when
// the source is not known.
type ClusterNameInfo struct {
	Name   string `json:"name,omitempty"`
	Source string `json:"source,omitempty"`
	Notice string `json:"notice,omitempty"`
}

// noClusterNameNotice is the notice of a cluster without a name; the
// setting is the configuration key, not a dashboard field.
const noClusterNameNotice = "This cluster has no name. Set " +
	"app.clusterName so alerts and the dashboard say which cluster " +
	"they are about."

// SetClusterNameSource says where the cluster name came from, before
// the server serves: "configured", or the words of a detected name
// ("from the node label alpha.eksctl.io/cluster-name"). It is empty
// when there is no name. A nil server ignores it.
func (s *Server) SetClusterNameSource(source string) {
	if s == nil {
		return
	}
	s.opts.ClusterNameSource = source
}

// clusterName is the name and its source for the kwatch page, from the
// name in the engine's report.
func (s *Server) clusterName(name string) *ClusterNameInfo {
	if name == "" {
		return &ClusterNameInfo{Notice: noClusterNameNotice}
	}
	return &ClusterNameInfo{Name: name, Source: s.opts.ClusterNameSource}
}
