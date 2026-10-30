package shellinstaller

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestGentleShellStableResponseMatches(t *testing.T) {
	root := &url.URL{Scheme: "https", Host: "registry.npmjs.org", Path: "/"}
	const archivePath = "/gentle-pi/-/gentle-pi-1.0.0.tgz"
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		DNSNames:     []string{root.Hostname()},
		NotBefore:    time.Unix(0, 0),
		NotAfter:     time.Unix(4102444800, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*http.Response)
		want   bool
	}{
		{name: "verified exact HTTPS origin path and certificate", want: true},
		{name: "nil TLS", mutate: func(r *http.Response) { r.TLS = nil }},
		{name: "wrong method", mutate: func(r *http.Response) { r.Request.Method = http.MethodPost }},
		{name: "non HTTPS", mutate: func(r *http.Response) { r.Request.URL.Scheme = "http" }},
		{name: "wrong URL host", mutate: func(r *http.Response) { r.Request.URL.Host = "other.example" }},
		{name: "wrong request host", mutate: func(r *http.Response) { r.Request.Host = "other.example" }},
		{name: "wrong path", mutate: func(r *http.Response) { r.Request.URL.Path = "/other.tgz" }},
		{name: "query present", mutate: func(r *http.Response) { r.Request.URL.RawQuery = "download=1" }},
		{name: "no verified chain", mutate: func(r *http.Response) { r.TLS.VerifiedChains = nil }},
		{name: "empty verified chain", mutate: func(r *http.Response) { r.TLS.VerifiedChains[0] = nil }},
		{name: "no peer certificate", mutate: func(r *http.Response) { r.TLS.PeerCertificates = nil }},
		{name: "chain leaf differs", mutate: func(r *http.Response) {
			r.TLS.VerifiedChains[0][0] = &x509.Certificate{Raw: []byte("other leaf")}
		}},
		{name: "certificate hostname differs", mutate: func(r *http.Response) {
			other := *certificate
			other.DNSNames = []string{"other.example"}
			r.TLS.PeerCertificates = []*x509.Certificate{&other}
			r.TLS.VerifiedChains = [][]*x509.Certificate{{&other}}
		}},
		{name: "wrong TLS server name", mutate: func(r *http.Response) { r.TLS.ServerName = "other.example" }},
		{name: "old TLS version", mutate: func(r *http.Response) { r.TLS.Version = tls.VersionTLS11 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Model an already verified handshake; this boundary makes no network request.
			response := &http.Response{
				Request: &http.Request{
					Method: http.MethodGet,
					URL:    &url.URL{Scheme: root.Scheme, Host: root.Host, Path: archivePath},
				},
				TLS: &tls.ConnectionState{
					Version:          tls.VersionTLS12,
					ServerName:       root.Hostname(),
					PeerCertificates: []*x509.Certificate{certificate},
					VerifiedChains:   [][]*x509.Certificate{{certificate}},
				},
			}
			if tt.mutate != nil {
				tt.mutate(response)
			}
			if got := gentleShellStableResponseMatches(response, root, archivePath); got != tt.want {
				t.Fatalf("gentleShellStableResponseMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}
