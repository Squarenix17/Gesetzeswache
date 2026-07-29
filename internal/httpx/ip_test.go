package httpx

import (
	"net"
	"testing"
)

func TestIsBlockedIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"192.168.1.1", true},
		{"169.254.169.254", true},
		{"100.64.0.1", true},
		{"100.127.255.254", true},
		{"::1", true},
		{"::ffff:127.0.0.1", true},
		{"64:ff9b::127.0.0.1", true},
		{"64:ff9b::169.254.169.254", true},
		{"8.8.8.8", false},
		{"93.184.216.34", false},
		{"100.63.255.254", false},
		{"100.128.0.1", false},
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			if got := IsBlockedIP(net.ParseIP(tc.ip)); got != tc.want {
				t.Fatalf("IsBlockedIP(%q)=%v want %v", tc.ip, got, tc.want)
			}
		})
	}
}

func TestValidateURL(t *testing.T) {
	if err := ValidateURL("https://www.gesetze-im-internet.de/gii-toc.xml"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateURL("http://169.254.169.254/x"); err == nil {
		t.Fatal("expected literal IP rejection")
	}
	if err := ValidateURL("https://evil.example/x"); err == nil {
		t.Fatal("expected allowlist rejection")
	}
}
