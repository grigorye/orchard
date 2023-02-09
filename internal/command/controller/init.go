package controller

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/internal/controller"
	"github.com/spf13/cobra"
	"math/big"
	"time"
)

var ErrInitFailed = errors.New("controller initialization failed")

var controllerCertPath string
var controllerKeyPath string
var force bool

func newInitCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "init",
		Short: "Initialize the controller",
		RunE:  runInit,
	}

	command.PersistentFlags().StringVar(&controllerCertPath, "controller-cert", "",
		"do not auto-generate the controller certificate, import it from the specified path instead"+
			" (requires --controller-key)")
	command.PersistentFlags().StringVar(&controllerKeyPath, "controller-key", "",
		"do not auto-generate the controller certificate key, import it from the specified path instead"+
			" (requires --controller-cert)")
	command.PersistentFlags().BoolVar(&force, "force", false,
		"force re-initialization if the controller is already initialized")

	return command
}

func runInit(cmd *cobra.Command, args []string) (err error) {
	var controllerCert tls.Certificate

	dataDir, err := controller.NewDataDir(dataDirPath)
	if err != nil {
		return err
	}

	initialized, err := dataDir.Initialized()
	if err != nil {
		return err
	}

	if initialized && !force {
		return fmt.Errorf("%w: controller is already initialized, preventing overwrite; "+
			"please specify \"--force\" to re-initialize", ErrInitFailed)
	}

	if controllerCertPath != "" || controllerKeyPath != "" {
		if err := checkBothCertAndKeyAreSpecified(); err != nil {
			return err
		}

		controllerCert, err = tls.LoadX509KeyPair(controllerCertPath, controllerCertPath)
		if err != nil {
			return err
		}
	} else {
		controllerCert, err = generateSelfSignedControllerCertificate()
		if err != nil {
			return err
		}
	}

	if err := dataDir.SetControllerCertificate(controllerCert); err != nil {
		return err
	}

	return nil
}

func checkBothCertAndKeyAreSpecified() error {
	if controllerCertPath == "" {
		return fmt.Errorf("%w: when --controller-key is specified, --controller-cert must be specified too",
			ErrInitFailed)
	}

	if controllerKeyPath == "" {
		return fmt.Errorf("%w: when --controller-cert is specified, --controller-key must be specified too",
			ErrInitFailed)
	}

	return nil
}

func generateSelfSignedControllerCertificate() (tls.Certificate, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P384(), cryptorand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return tls.Certificate{}, err
	}

	now := time.Now()

	cert := &x509.Certificate{
		SerialNumber: big.NewInt(0),
		Subject: pkix.Name{
			CommonName: "Orchard Controller",
		},
		NotBefore:             now,
		NotAfter:              now.AddDate(10, 0, 0),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		DNSNames:              []string{controller.DefaultServerName},
	}

	certBytes, err := x509.CreateCertificate(cryptorand.Reader, cert, cert, privateKey.Public(), privateKey)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEMBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	privateKeyPEMBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	return tls.X509KeyPair(certPEMBytes, privateKeyPEMBytes)
}