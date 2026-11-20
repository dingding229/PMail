package dkimkey

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
)

func Generate() (string, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), nil
}

func Parse(raw string) (crypto.Signer, error) {
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, errors.New("missing DKIM PEM block")
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "EDDSA PRIVATE KEY":
		if len(block.Bytes) == ed25519.PrivateKeySize {
			return ed25519.PrivateKey(block.Bytes), nil
		}
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		switch key := key.(type) {
		case *rsa.PrivateKey:
			return key, nil
		case ed25519.PrivateKey:
			return key, nil
		}
	}
	return nil, errors.New("unsupported DKIM private key")
}

func Record(key crypto.Signer) (string, error) {
	switch pub := key.Public().(type) {
	case *rsa.PublicKey:
		encoded, err := x509.MarshalPKIXPublicKey(pub)
		if err != nil {
			return "", err
		}
		return "v=DKIM1; k=rsa; p=" + base64.StdEncoding.EncodeToString(encoded), nil
	case ed25519.PublicKey:
		return "v=DKIM1; k=ed25519; p=" + base64.StdEncoding.EncodeToString(pub), nil
	default:
		return "", errors.New("unsupported DKIM public key")
	}
}
