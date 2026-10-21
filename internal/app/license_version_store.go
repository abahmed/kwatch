package app

import (
	"context"
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/license"
)

// licenseVersionField mirrors the reported kwatch version in the instance
// Secret, so a lost data volume does not renew early at the same version.
// licenseRenewAttemptField mirrors a pending early renewal the same way,
// so a crash loop on a fresh data volume still respects its window.
const (
	licenseVersionField      = "reported-version"
	licenseRenewAttemptField = "renew-attempt"
)

// LoadReportedVersion implements license.VersionStore.
func (s boltLeaseStore) LoadReportedVersion() (string, error) {
	var v string
	_, err := s.disk.get(stateLicenseVersion, &v)
	return v, err
}

// SaveReportedVersion implements license.VersionStore.
func (s boltLeaseStore) SaveReportedVersion(v string) error {
	return s.disk.put(stateLicenseVersion, v)
}

// LoadRenewAttempt implements license.VersionStore.
func (s boltLeaseStore) LoadRenewAttempt() (license.RenewAttempt, error) {
	var a license.RenewAttempt
	_, err := s.disk.get(stateLicenseRenewAttempt, &a)
	return a, err
}

// SaveRenewAttempt implements license.VersionStore.
func (s boltLeaseStore) SaveRenewAttempt(a license.RenewAttempt) error {
	return s.disk.put(stateLicenseRenewAttempt, a)
}

// LoadRenewAttempt implements license.VersionStore. The state file wins;
// the Secret copy fills in when the state file has none.
func (s *mirroredLeaseStore) LoadRenewAttempt() (
	license.RenewAttempt, error,
) {
	a, localErr := s.local.LoadRenewAttempt()
	if localErr == nil && a.Version != "" {
		return a, nil
	}
	raw, err := s.loadField(licenseRenewAttemptField)
	if err != nil || len(raw) == 0 {
		return a, localErr
	}
	var remote license.RenewAttempt
	if json.Unmarshal(raw, &remote) != nil {
		return a, localErr
	}
	return remote, nil
}

// SaveRenewAttempt implements license.VersionStore. A failed Secret
// write is logged and only the state file's error returned.
func (s *mirroredLeaseStore) SaveRenewAttempt(a license.RenewAttempt) error {
	localErr := s.local.SaveRenewAttempt(a)
	raw, err := json.Marshal(a)
	if err == nil {
		err = s.saveMirrored(licenseRenewAttemptField, string(raw))
	}
	if err != nil {
		klog.ErrorS(err, "mirror license renew attempt to its Secret",
			"component", "license")
	}
	return localErr
}

// LoadReportedVersion implements license.VersionStore. The state file
// wins; the Secret copy fills in when the state file has none.
func (s *mirroredLeaseStore) LoadReportedVersion() (string, error) {
	v, localErr := s.local.LoadReportedVersion()
	if localErr == nil && v != "" {
		return v, nil
	}
	raw, err := s.loadField(licenseVersionField)
	if err != nil {
		return v, localErr
	}
	return string(raw), nil
}

// loadField reads one field of the instance Secret.
func (s *mirroredLeaseStore) loadField(field string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(
		context.Background(), leaseMirrorTimeout)
	defer cancel()
	secret, err := s.secret.secrets().Get(
		ctx, licenseInstanceSecret, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return secret.Data[field], nil
}

// saveMirrored writes one field of the instance Secret when it differs.
func (s *mirroredLeaseStore) saveMirrored(field, value string) error {
	ctx, cancel := context.WithTimeout(
		context.Background(), leaseMirrorTimeout)
	defer cancel()
	return s.secret.saveField(ctx, field, value)
}

// SaveReportedVersion implements license.VersionStore. Like SaveLease, a
// failed Secret write is logged and only the state file's error returned.
func (s *mirroredLeaseStore) SaveReportedVersion(v string) error {
	localErr := s.local.SaveReportedVersion(v)
	if err := s.saveMirrored(licenseVersionField, v); err != nil {
		klog.ErrorS(err, "mirror reported kwatch version to its Secret",
			"component", "license")
	}
	return localErr
}

// saveField sets one field of the instance Secret when it differs.
func (s kubeInstanceStore) saveField(
	ctx context.Context, field, value string,
) error {
	existing, err := s.secrets().Get(
		ctx, licenseInstanceSecret, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if string(existing.Data[field]) == value {
		return nil
	}
	if existing.Data == nil {
		existing.Data = map[string][]byte{}
	}
	existing.Data[field] = []byte(value)
	_, err = s.secrets().Update(ctx, existing, metav1.UpdateOptions{})
	return err
}
