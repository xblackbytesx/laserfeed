package scraper

import (
	"net"
	"testing"
)

func TestIsPrivateIP_Blocked(t *testing.T) {
	blocked := []string{
		"127.0.0.1",          // loopback
		"::1",                // IPv6 loopback
		"10.0.0.1",           // RFC1918
		"192.168.1.1",        // RFC1918
		"172.16.0.1",         // RFC1918
		"169.254.169.254",    // link-local / cloud metadata
		"fe80::1",            // IPv6 link-local
		"100.64.0.1",         // CGNAT
		"100.127.255.254",    // CGNAT high end
		"0.0.0.0",            // unspecified
		"::",                 // IPv6 unspecified
		"224.0.0.1",          // IPv4 multicast
		"fd00::1",            // RFC4193 unique-local IPv6
	}
	for _, s := range blocked {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("invalid test fixture %q", s)
		}
		if !isPrivateIP(ip) {
			t.Errorf("%s should be blocked", s)
		}
	}
}

func TestIsPrivateIP_Allowed(t *testing.T) {
	allowed := []string{
		"8.8.8.8",
		"1.1.1.1",
		"93.184.216.34",       // example.com
		"100.63.255.255",      // just below CGNAT range
		"100.128.0.1",         // just above CGNAT range
		"2606:4700:4700::1111", // public IPv6
	}
	for _, s := range allowed {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("invalid test fixture %q", s)
		}
		if isPrivateIP(ip) {
			t.Errorf("%s should be allowed", s)
		}
	}
}
