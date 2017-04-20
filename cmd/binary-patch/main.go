package main

import (
	"crypto"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/inconshreveable/go-update"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	//Buildstamp is used for storing the timestamp of the build
	Buildstamp string = "Not set"
	//Githash is used for storing the commit hash of the build
	Githash string = "Not set"
	// Version is used to store the tagged version of the build
	Version string = "Not set"
)

func main() {
	var (
		debug         = kingpin.Flag("debug", "enable debug mode").Default("false").Bool()
		_             = kingpin.Command("version", "show version")
		update        = kingpin.Command("update", "update binary")
		baseUpdateURL = update.Flag("url", "Update URL").Default("http://localhost:8080/update").String()
	)

	switch kingpin.Parse() {
	case "version":
		fmt.Printf(`%s Version: %s
================================
    Buildtime: %s
    GitHash: %s
`, path.Base(os.Args[0]), Version, Buildstamp, Githash)
		os.Exit(0)
	case "update":
		binSlice := strings.Split(os.Args[0], "/")
		binary := binSlice[len(binSlice)-1]
		updateURL, err := url.Parse(fmt.Sprintf("%s/%s?version=%s&arch=%s&os=%s", *baseUpdateURL, binary, Version, runtime.GOARCH, runtime.GOOS))
		if err != nil {
			log.Fatalf("Could not parse URL, caused by: %v", err)
		}

		log.Println(*debug)
		if *debug {
			log.Printf("call %s", updateURL)
		}
		if err := doUpdate(updateURL.String()); err != nil {
			log.Printf("Update failed caused by: %v", err)
			os.Exit(1)
		}
	}

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
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Could not update, status code: %d", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusNotModified {
		log.Println("You already have the latest version")
		return nil
	}

	err = update.Apply(resp.Body, update.Options{})
	if err != nil {
		if rerr := update.RollbackError(err); rerr != nil {
			log.Printf("Failed to rollback from bad update: %v", rerr)
		}
	}
	log.Println("Update complete")

	return err
}
