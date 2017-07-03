package main

import (
	"bufio"
	"bytes"
	"crypto"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
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
	// public key to verify signed updates
	publicKey []byte
)

type SignedUpdate struct {
	Patch     []byte `json:"patch"`
	Signature []byte `json:"signature"`
	Digest    []byte `json:"sha256"`
}

func main() {
	var (
		publicKeyFDptr **os.File
		debug          = kingpin.Flag("debug", "enable debug mode").Default("false").Bool()
		_              = kingpin.Command("version", "show version")
		update         = kingpin.Command("update", "update binary")
		baseUpdateURL  = update.Flag("url", "Update URL").Default("http://localhost:8080/update").String()
		patchUpdate    = kingpin.Command("patch-update", "update binary diff")

		basePatchUpdateURL = patchUpdate.Flag("url", "Update URL").Default("http://localhost:8080/patch-update").String()

		signed              = kingpin.Command("signed", "Signed updates")
		signedUpdate        = signed.Command("update", "Verify signature and update binary")
		baseSignedUpdateURL = signedUpdate.Flag("url", "Update URL").Default("http://localhost:8080/signed-update").String()

		signedPatchUpdate        = signed.Command("patch-update", "Verify signature and update binary diff")
		baseSignedPatchUpdateURL = signedPatchUpdate.Flag("url", "Update URL").Default("http://localhost:8080/signed-patch-update").String()
	)
	publicKeyFDptr = signed.Flag("public-key", "File path containing the public Key used to verify signed updates.").File()

	cmd := kingpin.Parse()
	if *debug {
		log.Print("TODO configure debug mode")
	}
	if publicKeyFDptr != nil && *publicKeyFDptr != nil {
		fd := *publicKeyFDptr
		if buf, err := ioutil.ReadAll(fd); err == nil {
			publicKey = buf
		} else {
			log.Fatalf("Failed to read %s: %v", fd.Name(), err)
		}
	}

	switch cmd {
	case "version":
		fmt.Printf(`%s Version: %s
================================
    Buildtime: %s
    GitHash: %s
`, path.Base(os.Args[0]), Version, Buildstamp, Githash)
		os.Exit(0)
	case update.FullCommand():
		rc, err := GetUpdate(*baseUpdateURL, Version)
		if err != nil {
			log.Fatalf("Failed to get update: %v", err)
		}
		if err := ApplyUpdate(rc); err != nil {
			log.Fatalf("Failed to apply update: %v", err)
		}
		rc.Close()

	case patchUpdate.FullCommand():
		log.Printf("use %s", *basePatchUpdateURL)

		rc, err := GetUpdate(*basePatchUpdateURL, Version)
		if err != nil {
			log.Fatalf("Failed to get update: %v", err)
		}
		if err := ApplyUpdateWithPatch(rc); err != nil {
			log.Fatalf("Failed to apply update: %v", err)
		}
		rc.Close()

	case signedUpdate.FullCommand():
		log.Printf("use %s", *baseSignedUpdateURL)
		rc, err := GetUpdate(*baseSignedUpdateURL, Version)
		if err != nil {
			log.Fatalf("Failed to get update: %v", err)
		}

		jsonUpdateData, err := ioutil.ReadAll(rc)
		if err != nil {
			log.Fatalf("Failed to read json: %v", err)
		}
		rc.Close()

		var data SignedUpdate
		err = json.Unmarshal(jsonUpdateData, &data)
		if err != nil {
			log.Fatalf("Failed to unmarshal json: %v", err)
		}

		buf := bytes.NewBuffer(data.Patch)
		r := bufio.NewReader(buf)
		rcPatch := ioutil.NopCloser(r)
		sDigest := strings.TrimSpace(string(data.Digest))
		sSign := hex.EncodeToString(data.Signature)
		err = ApplyVerifiedUpdate(rcPatch, sDigest, sSign)
		if err != nil {
			log.Fatalf("Failed verified update: %v", err)
		}

	case signedPatchUpdate.FullCommand():
		log.Printf("use %s", *baseSignedPatchUpdateURL)
		rc, err := GetUpdate(*baseSignedPatchUpdateURL, Version)
		if err != nil {
			log.Fatalf("Failed to get update: %v", err)
		}

		jsonUpdateData, err := ioutil.ReadAll(rc)
		if err != nil {
			log.Fatalf("Failed to read json: %v", err)
		}
		rc.Close()

		var data SignedUpdate
		err = json.Unmarshal(jsonUpdateData, &data)
		if err != nil {
			log.Fatalf("Failed to unmarshal json: %v", err)
		}

		buf := bytes.NewBuffer(data.Patch)
		r := bufio.NewReader(buf)
		rcPatch := ioutil.NopCloser(r)

		digest := strings.TrimSpace(string(data.Digest))
		sSign := hex.EncodeToString(data.Signature)
		err = ApplyVerifiedPatchUpdate(rcPatch, digest, sSign)
		if err != nil {
			log.Fatalf("Failed verified update: %v", err)
		}
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

// ApplyVerifiedUpdate applies a signed binary update and checks the checksum.
func ApplyVerifiedUpdate(binary io.ReadCloser, hexChecksum, hexSignature string) error {
	defer binary.Close()
	checksum, err := hex.DecodeString(hexChecksum)
	if err != nil {
		return fmt.Errorf("failed checksum: %v", err)
	}
	signature, err := hex.DecodeString(hexSignature)
	if err != nil {
		return fmt.Errorf("failed signature: %v", err)
	}
	opts := update.Options{
		Checksum:  checksum,
		Signature: signature,
		Hash:      crypto.SHA256,
		Verifier:  update.NewECDSAVerifier(),
	}
	err = opts.SetPublicKeyPEM(publicKey)
	if err != nil {
		return fmt.Errorf("failed set opts: %v", err)
	}
	err = update.Apply(binary, opts)
	if err != nil {
		if rerr := update.RollbackError(err); rerr != nil {
			return fmt.Errorf("failed to rollback from bad signed update: %v", rerr)
		}
		log.Println("Rolled back signed patch Update")
		return fmt.Errorf("successfully rolled back signed update with options %v: %v", opts, err)
	}
	return nil
}

// ApplyVerifiedPatchUpdate applies a signed binary patch and checks the checksum.
func ApplyVerifiedPatchUpdate(binary io.ReadCloser, hexChecksum, hexSignature string) error {
	defer binary.Close()
	checksum, err := hex.DecodeString(hexChecksum)
	if err != nil {
		return fmt.Errorf("failed to decode checksum: %v", err)
	}
	signature, err := hex.DecodeString(hexSignature)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %v", err)
	}
	opts := update.Options{
		Patcher:   update.NewBSDiffPatcher(),
		Checksum:  checksum,
		Signature: signature,
		Hash:      crypto.SHA256,
		Verifier:  update.NewECDSAVerifier(),
	}
	err = opts.SetPublicKeyPEM(publicKey)
	if err != nil {
		return fmt.Errorf("failed set opts: %v", err)
	}
	err = update.Apply(binary, opts)
	if err != nil {
		if rerr := update.RollbackError(err); rerr != nil {
			return fmt.Errorf("failed to rollback from bad signed patch update: %v", rerr)
		}
		log.Println("Rolled back signed patch Update")
		return fmt.Errorf("successfully rolled back signed patch update with options %v: %v", opts, err)
	}
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
