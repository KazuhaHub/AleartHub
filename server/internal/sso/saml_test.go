package sso

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/crewjam/saml"
)

// mapAssertion is the part of SAML we own; test it directly (internal test pkg).
func TestMapAssertion(t *testing.T) {
	a := &saml.Assertion{
		Subject: &saml.Subject{NameID: &saml.NameID{Value: "user@corp"}},
		AttributeStatements: []saml.AttributeStatement{{Attributes: []saml.Attribute{
			{Name: "email", Values: []saml.AttributeValue{{Value: "u@corp.com"}}},
			{Name: "displayName", Values: []saml.AttributeValue{{Value: "User"}}},
			{Name: "groups", Values: []saml.AttributeValue{{Value: "admins"}, {Value: "soc"}}},
		}}},
	}
	c := mapAssertion(a, SAMLConfig{AttrUsername: "nameid", AttrEmail: "email", AttrName: "displayName", AttrGroups: "groups"})
	if c.Subject != "user@corp" || c.Username != "user@corp" {
		t.Errorf("subject/username wrong: %+v", c)
	}
	if c.Email != "u@corp.com" || c.Name != "User" {
		t.Errorf("email/name wrong: %+v", c)
	}
	if len(c.Groups) != 2 {
		t.Errorf("groups = %v, want 2", c.Groups)
	}
}

func selfSignedCertB64(t *testing.T) string {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "idp.example"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(der)
}

func TestNewSAMLMetadataAndRedirect(t *testing.T) {
	idpMeta := fmt.Sprintf(`<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="https://idp.example/metadata">
  <IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    <KeyDescriptor use="signing"><KeyInfo xmlns="http://www.w3.org/2000/09/xmldsig#"><X509Data><X509Certificate>%s</X509Certificate></X509Data></KeyInfo></KeyDescriptor>
    <SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="https://idp.example/sso"/>
  </IDPSSODescriptor>
</EntityDescriptor>`, selfSignedCertB64(t))

	s, err := NewSAML(SAMLConfig{
		Enabled:        true,
		EntityID:       "https://sp.example/api/auth/saml/metadata",
		ACSURL:         "https://sp.example/api/auth/saml/acs",
		MetadataURL:    "https://sp.example/api/auth/saml/metadata",
		IDPMetadataXML: idpMeta,
		AttrUsername:   "nameid",
	})
	if err != nil {
		t.Fatalf("NewSAML: %v", err)
	}
	if !s.Enabled() {
		t.Fatal("should be enabled")
	}
	md, err := s.MetadataXML()
	if err != nil || !strings.Contains(string(md), "saml/acs") {
		t.Fatalf("metadata missing ACS: err=%v md=%s", err, md)
	}
	url, err := s.RedirectURL("relay1")
	if err != nil || !strings.Contains(url, "SAMLRequest=") || !strings.Contains(url, "idp.example") {
		t.Fatalf("redirect wrong: err=%v url=%s", err, url)
	}
}

func TestNewSAMLDisabled(t *testing.T) {
	s, err := NewSAML(SAMLConfig{Enabled: false})
	if err != nil || s.Enabled() {
		t.Fatal("disabled SAML should be inert")
	}
}
