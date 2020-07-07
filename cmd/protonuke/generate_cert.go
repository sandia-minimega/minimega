// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Generate a self-signed X.509 certificate for a TLS server.

// modified by fritz for generating temporary certs for serving tls https in
// protonuke.

package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io/ioutil"
	"math/big"
	"os"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var (
	validFor = time.Duration(365 * 24 * time.Hour)
	isCA     = false
	rsaBits  = 1024
)

func generateCerts() (string, string) {
	if *f_httpTLSCert != "" && *f_httpTLSKey != "" {
		return *f_httpTLSCert, *f_httpTLSKey
	}
	host, err := os.Hostname()
	if err != nil {
		log.Fatalln(err)
	}

	priv, err := rsa.GenerateKey(rand.Reader, rsaBits)
	if err != nil {
		log.Fatal("failed to generate private key: %s", err)
	}

	var notBefore time.Time
	notBefore = time.Now()
	notAfter := notBefore.Add(validFor)

	// end of ASN.1 time
	endOfTime := time.Date(2049, 12, 31, 23, 59, 59, 0, time.UTC)
	if notAfter.After(endOfTime) {
		notAfter = endOfTime
	}

	template := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(0),
		Subject: pkix.Name{
			Organization: []string{"protonuke"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	template.DNSNames = append(template.DNSNames, host)

	if isCA {
		template.IsCA = true
		template.KeyUsage |= x509.KeyUsageCertSign
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		log.Fatal("Failed to create certificate: %s", err)
	}

	certOut, err := ioutil.TempFile("", "protonuke_cert_")
	if err != nil {
		log.Fatal("failed to open cert for writing: %s", err)
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	certOut.Close()
	log.Debugln("wrote cert to: ", certOut.Name())

	keyOut, err := ioutil.TempFile("", "protonuke_key_")
	if err != nil {
		log.Fatal("failed to open key.pem for writing: %v", err)
	}
	pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	keyOut.Close()
	log.Debugln("wrote key to: ", keyOut.Name())

	return certOut.Name(), keyOut.Name()
}
