package sso

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"encoding/xml"
	"errors"
	"net/http"
	"net/url"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"
)

type SAMLConfig struct {
	Enabled        bool
	EntityID       string // SP entity id (usually the metadata URL)
	ACSURL         string // https://host/api/auth/saml/acs
	MetadataURL    string // https://host/api/auth/saml/metadata
	IDPMetadataXML string // inline IdP metadata XML (preferred — no startup network)
	IDPMetadataURL string // or fetch IdP metadata from URL
	CertPEM        string // optional SP signing cert
	KeyPEM         string // optional SP signing key
	AttrUsername   string // SAML attribute name for username ("" or "nameid" → use NameID)
	AttrEmail      string
	AttrName       string
	AttrGroups     string
	// AllowIDPInitiated accepts unsolicited IdP-initiated SAMLResponses (no prior
	// SP AuthnRequest). Off by default: IdP-initiated flows carry no request-bound
	// state, which is a CSRF/replay vector. Enable only if your IdP requires it.
	AllowIDPInitiated bool
}

type SAML struct {
	cfg SAMLConfig
	sp  *saml.ServiceProvider
}

func NewSAML(cfg SAMLConfig) (*SAML, error) {
	if !cfg.Enabled {
		return &SAML{cfg: cfg}, nil
	}
	var idp *saml.EntityDescriptor
	switch {
	case cfg.IDPMetadataXML != "":
		md, err := samlsp.ParseMetadata([]byte(cfg.IDPMetadataXML))
		if err != nil {
			return nil, err
		}
		idp = md
	case cfg.IDPMetadataURL != "":
		u, err := url.Parse(cfg.IDPMetadataURL)
		if err != nil {
			return nil, err
		}
		md, err := samlsp.FetchMetadata(context.Background(), http.DefaultClient, *u)
		if err != nil {
			return nil, err
		}
		idp = md
	default:
		return nil, errors.New("saml: no IdP metadata (set ALERTHUB_SAML_IDP_METADATA[_URL])")
	}
	acs, err := url.Parse(cfg.ACSURL)
	if err != nil {
		return nil, err
	}
	meta, err := url.Parse(cfg.MetadataURL)
	if err != nil {
		return nil, err
	}
	sp := &saml.ServiceProvider{
		EntityID:          cfg.EntityID,
		AcsURL:            *acs,
		MetadataURL:       *meta,
		IDPMetadata:       idp,
		AllowIDPInitiated: cfg.AllowIDPInitiated,
	}
	if cfg.CertPEM != "" && cfg.KeyPEM != "" {
		pair, err := tls.X509KeyPair([]byte(cfg.CertPEM), []byte(cfg.KeyPEM))
		if err != nil {
			return nil, err
		}
		leaf, err := x509.ParseCertificate(pair.Certificate[0])
		if err != nil {
			return nil, err
		}
		signer, ok := pair.PrivateKey.(crypto.Signer)
		if !ok {
			return nil, errors.New("saml: SP key is not a crypto.Signer")
		}
		sp.Certificate = leaf
		sp.Key = signer
	}
	return &SAML{cfg: cfg, sp: sp}, nil
}

func (s *SAML) Enabled() bool { return s.sp != nil }

// RedirectURL builds the IdP SSO redirect (HTTP-Redirect binding).
func (s *SAML) RedirectURL(relayState string) (string, error) {
	u, err := s.sp.MakeRedirectAuthenticationRequest(relayState)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func (s *SAML) MetadataXML() ([]byte, error) {
	return xml.MarshalIndent(s.sp.Metadata(), "", "  ")
}

// ParseResponse validates the SAMLResponse (signature/conditions/audience via
// crewjam/saml) and maps the assertion to normalized Claims.
func (s *SAML) ParseResponse(r *http.Request) (*Claims, error) {
	a, err := s.sp.ParseResponse(r, nil) // AllowIDPInitiated covers nil requestIDs
	if err != nil {
		return nil, err
	}
	return mapAssertion(a, s.cfg), nil
}

func mapAssertion(a *saml.Assertion, cfg SAMLConfig) *Claims {
	attr := func(name string) string {
		if name == "" {
			return ""
		}
		for _, st := range a.AttributeStatements {
			for _, at := range st.Attributes {
				if at.Name == name || at.FriendlyName == name {
					if len(at.Values) > 0 {
						return at.Values[0].Value
					}
				}
			}
		}
		return ""
	}
	c := &Claims{}
	if a.Subject != nil && a.Subject.NameID != nil {
		c.Subject = a.Subject.NameID.Value
	}
	c.Email = attr(cfg.AttrEmail)
	c.Name = attr(cfg.AttrName)
	username := attr(cfg.AttrUsername)
	if cfg.AttrUsername == "" || cfg.AttrUsername == "nameid" {
		username = c.Subject
	}
	if username == "" {
		username = c.Email
	}
	if username == "" {
		username = c.Subject
	}
	c.Username = username
	if cfg.AttrGroups != "" {
		for _, st := range a.AttributeStatements {
			for _, at := range st.Attributes {
				if at.Name == cfg.AttrGroups || at.FriendlyName == cfg.AttrGroups {
					for _, v := range at.Values {
						c.Groups = append(c.Groups, v.Value)
					}
				}
			}
		}
	}
	return c
}
