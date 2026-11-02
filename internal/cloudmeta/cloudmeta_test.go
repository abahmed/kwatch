package cloudmeta

import (
	"net"
	"testing"
)

func TestAddressBlocksOnlyMetadataRanges(t *testing.T) {
	cases := map[string]bool{
		"169.254.169.254":        true,
		"169.254.10.1":           true,
		"fd00:ec2::254":          true,
		"100.100.100.200":        true,
		"fe80::1":                true,
		"::ffff:169.254.169.254": true,
		"64:ff9b::a9fe:a9fe":     true,
		"64:ff9b::6464:64c8":     true,
		"10.0.0.5":               false,
		"192.168.1.10":           false,
		"172.16.0.1":             false,
		"127.0.0.1":              false,
		"::1":                    false,
		"100.100.100.201":        false,
		"fd00:ec2::253":          false,
		"8.8.8.8":                false,
		"2001:db8::1":            false,
		"64:ff9b::cb00:710a":     false,
		"not-an-address":         false,
	}
	for raw, want := range cases {
		if got := Address(net.ParseIP(raw)); got != want {
			t.Errorf("Address(%s) = %v, want %v", raw, got, want)
		}
	}
	if Address(nil) {
		t.Error("Address(nil) = true, want false")
	}
}

func TestHostMatchesNamesAndZonedAddresses(t *testing.T) {
	cases := map[string]bool{
		"metadata.google.internal":  true,
		"Metadata.Google.Internal.": true,
		"metadata.goog":             true,
		"fe80::1%eth0":              true,
		"100.100.100.200":           true,
		"hooks.example.com":         false,
		"10.0.0.1":                  false,
	}
	for host, want := range cases {
		if got := Host(host); got != want {
			t.Errorf("Host(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestAddressesReturnsACopy(t *testing.T) {
	got := Addresses()
	got[0][0] ^= 0xff
	if !Address(net.ParseIP("fd00:ec2::254")) {
		t.Error("mutating Addresses() changed the canonical list")
	}
	for _, ip := range Addresses() {
		if !Address(ip) {
			t.Errorf("Addresses() holds %s, which Address refuses", ip)
		}
	}
}
