// Command mock-saml-idp runs a local SAML 2.0 identity provider for development
// and end-to-end tests. It is intentionally small and must not be used in production.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"time"

	"github.com/crewjam/saml/samlidp"
	"golang.org/x/crypto/bcrypt"
)

const addr = "127.0.0.1:9091"

func main() {
	key, cert := generateKeyPair()
	baseURL, _ := url.Parse("http://" + addr)
	store := &samlidp.MemoryStore{}
	password, _ := bcrypt.GenerateFromPassword([]byte("Password123!"), bcrypt.DefaultCost)
	if err := store.Put("/users/samluser", &samlidp.User{
		Name: "SAML User", Email: "saml@acme.test", HashedPassword: password,
		Groups: []string{"users"}, CommonName: "SAML User",
	}); err != nil {
		log.Fatal(err)
	}
	server, err := samlidp.New(samlidp.Options{
		URL: *baseURL, Key: key, Signer: key, Certificate: cert, Store: store,
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("mock SAML IdP listening on %s (metadata: %s/metadata, user: samluser / Password123!)", addr, baseURL)
	log.Fatal(http.ListenAndServe(addr, server))
}

func generateKeyPair() (*rsa.PrivateKey, *x509.Certificate) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatal(err)
	}
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	now := time.Now()
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "Mock SAML IdP"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.AddDate(10, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		log.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		log.Fatal(err)
	}
	keyDER, _ := x509.MarshalPKCS8PrivateKey(key)
	_ = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return key, cert
}
