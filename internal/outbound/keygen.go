package outbound

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"fmt"
)

func GenerateKeyDER() ([]byte, error) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate rsa key: %w", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("marshal pkcs8: %w", err)
	}
	return der, nil
}

func DERToDNSValue(der []byte) (string, error) {
	key, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return "", err
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("expected RSA key, got %T", key)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&rsaKey.PublicKey)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("v=DKIM1; k=rsa; p=%s", base64.StdEncoding.EncodeToString(pubDER)), nil
}
