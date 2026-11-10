package awscreds

import (
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// DomainSuffix is the AWS domain of the partition region belongs to:
// amazonaws.com.cn for the China regions (cn-north-1, cn-northwest-1),
// amazonaws.com for the commercial and GovCloud regions.
func DomainSuffix(region string) string {
	if strings.HasPrefix(region, "cn-") {
		return "amazonaws.com.cn"
	}
	return "amazonaws.com"
}

// ServiceURL is the regional HTTPS endpoint of an AWS query API, such
// as ServiceURL("sns", "eu-west-1").
func ServiceURL(host, region string) string {
	return "https://" + host + "." + region + "." +
		DomainSuffix(region) + "/"
}

// Endpoint is override when it is set, otherwise the region's own
// endpoint of host. An override that is not an http or https URL is an
// error that names only its scheme and host.
func Endpoint(override, host, region string) (string, error) {
	override = strings.TrimSpace(override)
	if override == "" {
		return ServiceURL(host, region), nil
	}
	if !transport.ValidEndpoint(override) {
		return "", fmt.Errorf("endpoint %s is not an http or https URL",
			transport.LogURL(override))
	}
	return override, nil
}
