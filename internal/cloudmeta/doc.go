// Package cloudmeta names the cloud instance-metadata addresses and host
// names. Such an endpoint hands out the node's cloud credentials, so both
// the delivery transport and KwatchConfig probes refuse them. In: an
// address or host name. Out: whether it reaches a metadata service. It is
// a leaf package, so both guards share one list.
package cloudmeta
