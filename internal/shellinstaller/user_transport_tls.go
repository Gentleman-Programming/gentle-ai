package shellinstaller

import (
	"crypto/tls"
	"net/http"
	"net/url"
)

func gentleShellStableResponseMatches(response *http.Response, root *url.URL,
	archivePath string) bool {
	if response == nil || response.Request == nil || response.Request.URL == nil || response.TLS == nil {
		return false
	}
	requestURL := response.Request.URL
	if response.Request.Method != http.MethodGet || requestURL.Scheme != "https" || requestURL.Host != root.Host ||
		requestURL.User != nil || requestURL.Path != archivePath ||
		requestURL.RawPath != "" || requestURL.RawQuery != "" || requestURL.ForceQuery ||
		requestURL.Fragment != "" || (response.Request.Host != "" && response.Request.Host != root.Host) {
		return false
	}
	state := response.TLS
	if state.Version < tls.VersionTLS12 || state.ServerName != root.Hostname() ||
		len(state.VerifiedChains) == 0 || len(state.VerifiedChains[0]) == 0 ||
		len(state.PeerCertificates) == 0 ||
		!state.VerifiedChains[0][0].Equal(state.PeerCertificates[0]) {
		return false
	}
	return state.PeerCertificates[0].VerifyHostname(root.Hostname()) == nil
}
