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
		patchUpdate   = kingpin.Command("patch-update", "update binary diff")

		basePatchUpdateURL  = patchUpdate.Flag("url", "Update URL").Default("http://localhost:8080/patch-update").String()
		signedUpdate        = kingpin.Command("signed-update", "Verify signature and update binary")
		baseSignedUpdateURL = signedUpdate.Flag("url", "Update URL").Default("http://localhost:8080/signed-update").String()
	)

	cmd := kingpin.Parse()
	if *debug {
		log.Print("TODO configure debug mode")
	}

	switch cmd {
	case "version":
		fmt.Printf(`%s Version: %s
================================
    Buildtime: %s
    GitHash: %s
`, path.Base(os.Args[0]), Version, Buildstamp, Githash)
		os.Exit(0)
	case "update":
		rc, err := GetUpdate(*baseUpdateURL, Version)
		if err != nil {
			log.Fatalf("Failed to get update: %v", err)
		}
		if err := ApplyUpdate(rc); err != nil {
			log.Fatalf("Failed to apply update: %v", err)
		}
		rc.Close()

	case "patch-update":
		log.Printf("use %s", *basePatchUpdateURL)
		log.Fatal("TODO")

		rc, err := GetUpdate(*basePatchUpdateURL, Version)
		if err != nil {
			log.Fatalf("Failed to get update: %v", err)
		}
		if err := ApplyUpdateWithPatch(rc); err != nil {
			log.Fatalf("Failed to apply update: %v", err)
		}
		rc.Close()

	case "signed-update":
		log.Printf("use %s", *baseSignedUpdateURL)
		log.Fatal("TODO")
	}

}

// GetUpdate returns an open io.ReadCloser, if error is not
// nil. Caller has to close the io.ReadCloser.
func GetUpdate(baseUpdateURL, version string) (io.ReadCloser, error) {
	binary := getLocalBinaryName()
	updateURL := getUpdateURL(baseUpdateURL, binary, version)
	rc, err := getUpdate(updateURL.String())
	if err != nil {
		return nil, fmt.Errorf("failed to getUpdate: %v", err)
	}
	return rc, nil
}

// ApplyUpdate is the simplest version of applying an update. It
// patches a full binary without checking a signature.
func ApplyUpdate(rc io.ReadCloser) error {
	defer rc.Close()
	err := update.Apply(rc, update.Options{})
	if err != nil {
		if rerr := update.RollbackError(err); rerr != nil {
			return fmt.Errorf("failed to rollback from bad update: %v", rerr)
		}
		log.Println("Rolled back update")
		return err
	}

	log.Println("Update complete")
	return nil
}

// ApplyUpdateWithPatch applies a not signed binary patch.
func ApplyUpdateWithPatch(patch io.Reader) error {
	err := update.Apply(patch, update.Options{
		Patcher: update.NewBSDiffPatcher(),
	})
	if err != nil {
		if rerr := update.RollbackError(err); rerr != nil {
			return fmt.Errorf("Failed to rollback from bad patch update: %v", rerr)
		}
		log.Println("Rolled back patch Update")
		return err
	}

	log.Println("Patch Update complete")
	return nil
}

func getLocalBinaryName() string {
	binSlice := strings.Split(os.Args[0], "/")
	return binSlice[len(binSlice)-1]
}

func getUpdateURL(baseUpdateURL, binary, version string) *url.URL {
	updateURL, err := url.Parse(fmt.Sprintf("%s/%s?version=%s&arch=%s&os=%s", baseUpdateURL, binary, version, runtime.GOARCH, runtime.GOOS))
	if err != nil {
		log.Fatalf("Could not parse URL, caused by: %v", err)
	}
	return updateURL
}

// getUpdate returns an open io.ReadCloser, if error is not
// nil. Caller has to close the io.ReadCloser.
func getUpdate(url string) (io.ReadCloser, error) {
	// request new file
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("failed to get update with status code: %d", resp.StatusCode)
	} else if resp.StatusCode == http.StatusNotModified {
		resp.Body.Close()
		return nil, fmt.Errorf("you already have the latest version")
	}
	return resp.Body, nil
}

// TODO

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

var publicKey = []byte(`
-----BEGIN PUBLIC KEY-----
MFYwEAYHKoZIzj0CAQYFK4EEAAoDQgAEtrVmBxQvheRArXjg2vG1xIprWGuCyESx
MMY8pjmjepSy2kuz+nl9aFLqmr+rDNdYvEBqQaZrYMc6k29gjvoQnQ==
-----END PUBLIC KEY-----
`)

func verifiedUpdate(binary io.ReadCloser, hexChecksum, hexSignature string) error {
	defer binary.Close()
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
		fmt.Errorf("failed to apply update with options %v: %v", opts, err)
	}
	return nil
}
