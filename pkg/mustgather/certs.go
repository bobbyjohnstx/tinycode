package mustgather

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"time"
)

// CertInfo holds parsed certificate details.
type CertInfo struct {
	Subject   string    `json:"subject"`
	Issuer    string    `json:"issuer"`
	NotBefore time.Time `json:"not_before"`
	NotAfter  time.Time `json:"not_after"`
	IsCA      bool      `json:"is_ca"`
	Expired   bool      `json:"expired"`
	DaysLeft  int       `json:"days_left"`
	Source    string    `json:"source"`
}

// ParseCertificates extracts X.509 certificates from PEM-encoded data.
func ParseCertificates(pemData []byte, source string) []CertInfo {
	now := time.Now()
	var certs []CertInfo

	rest := pemData
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		daysLeft := int(cert.NotAfter.Sub(now).Hours() / 24)
		certs = append(certs, CertInfo{
			Subject:   cert.Subject.CommonName,
			Issuer:    cert.Issuer.CommonName,
			NotBefore: cert.NotBefore,
			NotAfter:  cert.NotAfter,
			IsCA:      cert.IsCA,
			Expired:   now.After(cert.NotAfter),
			DaysLeft:  daysLeft,
			Source:    source,
		})
	}
	return certs
}

// ParseCertsFromSecret extracts PEM certificates from a Kubernetes Secret
// resource's data fields (tls.crt, ca.crt, etc.).
func ParseCertsFromSecret(obj map[string]any, source string) []CertInfo {
	data := GetNestedMap(obj, "data")
	if data == nil {
		return nil
	}

	var all []CertInfo
	for key, val := range data {
		if !strings.HasSuffix(key, ".crt") && !strings.HasSuffix(key, ".pem") && key != "ca-bundle.crt" {
			continue
		}
		s, ok := val.(string)
		if !ok {
			continue
		}
		certs := ParseCertificates([]byte(s), fmt.Sprintf("%s/%s", source, key))
		all = append(all, certs...)
	}
	return all
}
