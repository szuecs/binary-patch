package main

import (
	"crypto"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/inconshreveable/go-update"
)

var (
	version     string = "v0.0.1"
	flagVersion bool
)

func init() {
	flag.BoolVar(&flagVersion, "version", false, "show version")
}

func updateWithChecksum(binary io.Reader, hexChecksum string) error {
	checksum, err := hex.DecodeString(hexChecksum)
	if err != nil {
		return err
	}
	err = update.Apply(binary, update.Options{
		Hash:     crypto.SHA256,
		Checksum: checksum,
	})
	if err != nil {
		// error handling
	}
	return err
}

func updateWithPatch(patch io.Reader) error {
	err := update.Apply(patch, update.Options{
		Patcher: update.NewBSDiffPatcher(),
	})
	if err != nil {
		// error handling
	}
	return err
}

var publicKey = []byte(`
-----BEGIN PUBLIC KEY-----
MFYwEAYHKoZIzj0CAQYFK4EEAAoDQgAEtrVmBxQvheRArXjg2vG1xIprWGuCyESx
MMY8pjmjepSy2kuz+nl9aFLqmr+rDNdYvEBqQaZrYMc6k29gjvoQnQ==
-----END PUBLIC KEY-----
`)

func verifiedUpdate(binary io.Reader, hexChecksum, hexSignature string) error {
	checksum, err := hex.DecodeString(hexChecksum)
	if err != nil {
		return err
	}
	signature, err := hex.DecodeString(hexSignature)
	if err != nil {
		return err
	}
	opts := update.Options{
		Checksum:  checksum,
		Signature: signature,
		Hash:      crypto.SHA256,             // this is the default, you don't need to specify it
		Verifier:  update.NewECDSAVerifier(), // this is the default, you don't need to specify it
	}
	err = opts.SetPublicKeyPEM(publicKey)
	if err != nil {
		return err
	}
	err = update.Apply(binary, opts)
	if err != nil {
		// error handling
	}
	return err
}

func doUpdate(url string) error {
	// request the new file
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	err = update.Apply(resp.Body, update.Options{})
	if err != nil {
		if rerr := update.RollbackError(err); rerr != nil {
			fmt.Printf("Failed to rollback from bad update: %v", rerr)
		}
	}
	return err
}

func main() {
	flag.Parse()
	if flagVersion {
		fmt.Println(version)
		os.Exit(0)
	}
	if err := doUpdate("http://localhost:8080/"); err != nil {
		fmt.Printf("Update failed caused by: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Update complete")
}
