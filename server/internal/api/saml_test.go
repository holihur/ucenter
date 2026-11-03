package api

import (
	"strings"
	"testing"

	"github.com/crewjam/saml"
	"ucenter/internal/config"
	"ucenter/internal/idp"
	"ucenter/internal/models"
)

func TestSAMLIdentityMapping(t *testing.T) {
	assertion := &saml.Assertion{
		Subject: &saml.Subject{NameID: &saml.NameID{Value: "user@example.com"}},
		AttributeStatements: []saml.AttributeStatement{{
			Attributes: []saml.Attribute{
				{Name: "email", Values: []saml.AttributeValue{{Value: "user@example.com"}}},
				{Name: "displayName", Values: []saml.AttributeValue{{Value: "SAML User"}}},
			},
		}},
	}
	identity := samlIdentity(assertion, idp.Config{"emailAttribute": "email", "nameAttribute": "displayName"}, true)
	if identity.Subject != "user@example.com" || identity.Email != "user@example.com" || identity.Name != "SAML User" || !identity.EmailVerified {
		t.Fatalf("unexpected identity: %#v", identity)
	}
}

func TestSAMLServiceProviderMetadata(t *testing.T) {
	tenant := &models.Tenant{Slug: "acme"}
	keyPEM, certPEM, err := generateSAMLKeyPair(tenant)
	if err != nil {
		t.Fatal(err)
	}
	idpMetadata := `<md:EntityDescriptor xmlns:md="urn:oasis:names:tc:SAML:2.0:metadata" entityID="https://idp.test/metadata">
<md:IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
<md:SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="https://idp.test/sso"/>
</md:IDPSSODescriptor></md:EntityDescriptor>`
	a := &API{Cfg: config.Config{BaseURL: "https://id.example.com"}}
	sp, err := a.samlServiceProvider(tenant, idp.Config{"idpMetadataXML": idpMetadata, "spKey": keyPEM, "spCert": certPEM})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(sp.AcsURL.Path, "/t/acme/saml/acs") {
		t.Fatalf("unexpected ACS URL: %s", sp.AcsURL.String())
	}
	meta := sp.Metadata()
	if meta.EntityID != "https://id.example.com/t/acme/saml/metadata" {
		t.Fatalf("unexpected entity ID: %s", meta.EntityID)
	}
	if len(meta.SPSSODescriptors) == 0 || len(meta.SPSSODescriptors[0].AssertionConsumerServices) == 0 {
		t.Fatal("metadata is missing an ACS endpoint")
	}
	req, err := sp.MakeAuthenticationRequest(sp.GetSSOBindingLocation(saml.HTTPRedirectBinding), saml.HTTPRedirectBinding, saml.HTTPPostBinding)
	if err != nil || req.ID == "" {
		t.Fatalf("authentication request failed: %v", err)
	}
}

func TestSAMLBadMetadataRejected(t *testing.T) {
	tenant := &models.Tenant{Slug: "acme"}
	keyPEM, certPEM, _ := generateSAMLKeyPair(tenant)
	a := &API{Cfg: config.Config{BaseURL: "https://id.example.com"}}
	if _, err := a.samlServiceProvider(tenant, idp.Config{"idpMetadataXML": "not-xml", "spKey": keyPEM, "spCert": certPEM}); err == nil {
		t.Fatal("invalid metadata must be rejected")
	}
}
