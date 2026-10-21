package app

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// licenseVersionField mirrors the reported kwatch version in the instance
// Secret, so a lost data volume does not renew early at the same version.
const licenseVersionField = "reported-version"

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

// LoadReportedVersion implements license.VersionStore. The state file
// wins; the Secret copy fills in when the state file has none.
func (s *mirroredLeaseStore) LoadReportedVersion() (string, error) {
	v, localErr := s.local.LoadReportedVersion()
	if localErr == nil && v != "" {
		return v, nil
	}
	ctx, cancel := context.WithTimeout(
		context.Background(), leaseMirrorTimeout)
	defer cancel()
	secret, err := s.secret.secrets().Get(
		ctx, licenseInstanceSecret, metav1.GetOptions{})
	if err != nil {
		return v, localErr
	}
	return string(secret.Data[licenseVersionField]), nil
}

// SaveReportedVersion implements license.VersionStore. Like SaveLease, a
// failed Secret write is logged and only the state file's error returned.
func (s *mirroredLeaseStore) SaveReportedVersion(v string) error {
	localErr := s.local.SaveReportedVersion(v)
	ctx, cancel := context.WithTimeout(
		context.Background(), leaseMirrorTimeout)
	defer cancel()
	if err := s.secret.saveField(ctx, licenseVersionField, v); err != nil {
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
