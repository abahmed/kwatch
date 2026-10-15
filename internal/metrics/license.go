package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	licenseStateDesc = prometheus.NewDesc("kwatch_license_state",
		"Licensing state: 1 for the current state, 0 for the others",
		[]string{"state"}, nil)
	licenseRenewalsDesc = prometheus.NewDesc(
		"kwatch_license_renewals_total",
		"License activation and renewal attempts by result",
		[]string{"result"}, nil)

	licenseStates = [...]string{
		"community", "invalid", "pending", "active", "grace", "expired",
		"revoked", "conflict",
	}

	// licenseRenewalResults: success; rejected (the server refused the
	// license); error (network, server or rate-limit trouble).
	licenseRenewalResults = [...]string{"success", "rejected", "error"}
)

// The license fields of Registry: LicenseStateIndex indexes licenseStates
// (zero is community), LicenseExpiry is the license expiry as Unix seconds
// (0 without one) and LicenseRenewals counts by licenseRenewalResults.
func init() {
	scalarMetrics = append(scalarMetrics, gauge(
		"kwatch_license_expiry_timestamp_seconds",
		"License expiry as a Unix timestamp; 0 without a license",
		func(r *Registry) int64 { return r.LicenseExpiry.Load() }))
}

// SetLicenseState records the licensing state using a bounded label; an
// unknown state is ignored.
func (r *Registry) SetLicenseState(state string) {
	for i, allowed := range licenseStates {
		if state == allowed {
			r.LicenseStateIndex.Store(int64(i))
			return
		}
	}
}

// IncLicenseRenewal records one activation or renewal attempt using a
// bounded result label; an unknown result counts as an error.
func (r *Registry) IncLicenseRenewal(result string) {
	for i, allowed := range licenseRenewalResults {
		if result == allowed {
			r.LicenseRenewals[i].Add(1)
			return
		}
	}
	r.LicenseRenewals[len(licenseRenewalResults)-1].Add(1)
}

func (r *Registry) describeLicense(ch chan<- *prometheus.Desc) {
	ch <- licenseStateDesc
	ch <- licenseRenewalsDesc
}

func (r *Registry) collectLicense(ch chan<- prometheus.Metric) {
	current := int(r.LicenseStateIndex.Load())
	for i, state := range licenseStates {
		value := 0.0
		if i == current {
			value = 1
		}
		ch <- prometheus.MustNewConstMetric(
			licenseStateDesc, prometheus.GaugeValue, value, state,
		)
	}
	for i, result := range licenseRenewalResults {
		ch <- prometheus.MustNewConstMetric(
			licenseRenewalsDesc, prometheus.CounterValue,
			float64(r.LicenseRenewals[i].Load()), result,
		)
	}
}
