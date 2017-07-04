// Package patchclient implements all you need to easily do updates
// for your application.
package patchclient

import (
	"bufio"
	"bytes"
	"crypto"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"

	update "github.com/inconshreveable/go-update"
)

var (
	ErrGetUpdate     = errors.New("patchclient: failed to get update")
	ErrApplyUpdate   = errors.New("patchclient: failed to apply update")
	ErrUnmarshalJSON = errors.New("patchclient: failed to unmarshal json")
	ErrReadJSON      = errors.New("patchclient: failed to read json")
)

type PatchClient struct {
	URL       string
	Version   string
	PublicKey []byte
}

// NewInsecurePatchClient is not able to verify the signature of your update.
func NewInsecurePatchClient(url, version string) *PatchClient {
	return &PatchClient{
		URL:     url,
		Version: version,
	}
}

// NewPatchClient is able to verify updates.
func NewPatchClient(url, version string, pubKey []byte) *PatchClient {
	return &PatchClient{
		URL:       url,
		Version:   version,
		PublicKey: pubKey,
	}
}

func (pc *PatchClient) UnsignedNotVerifiedUpdate() error {
	rc, err := GetUpdate(pc.URL, pc.Version)
	if err != nil {
		return fmt.Errorf("%s: %v", ErrGetUpdate, err)
	}
	if err := pc.ApplyUpdate(rc); err != nil {
		return fmt.Errorf("%s: %v", ErrApplyUpdate, err)
	}
	return rc.Close()
}

func (pc *PatchClient) UnsignedNotVerifiedPatchUpdate() error {
	rc, err := GetUpdate(pc.URL, pc.Version)
	if err != nil {
		return fmt.Errorf("%s: %v", ErrGetUpdate, err)
	}
	if err := pc.ApplyUpdateWithPatch(rc); err != nil {
		return fmt.Errorf("%s: %v", ErrApplyUpdate, err)
	}
	return rc.Close()
}

func (pc *PatchClient) SignedVerifiedUpdate() error {
	rc, err := GetUpdate(pc.URL, pc.Version)
	if err != nil {
		return fmt.Errorf("%s: %v", ErrGetUpdate, err)
	}
	jsonUpdateData, err := ioutil.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("%s: %v", ErrReadJSON, err)
	}
	err = rc.Close()
	if err != nil {
		return err
	}

	var data SignedUpdate
	err = json.Unmarshal(jsonUpdateData, &data)
	if err != nil {
		return fmt.Errorf("%s: %v", ErrUnmarshalJSON, err)
	}

	buf := bytes.NewBuffer(data.Patch)
	r := bufio.NewReader(buf)
	rcPatch := ioutil.NopCloser(r)
	sDigest := strings.TrimSpace(string(data.Digest))
	sSign := hex.EncodeToString(data.Signature)
	err = pc.ApplyVerifiedUpdate(rcPatch, sDigest, sSign)
	if err != nil {
		return fmt.Errorf("%s: %v", ErrApplyUpdate, err)
	}
	return rcPatch.Close()
}

func (pc *PatchClient) SignedVerifiedPatchUpdate() error {
	rc, err := GetUpdate(pc.URL, pc.Version)
	if err != nil {
		return fmt.Errorf("%s: %v", ErrGetUpdate, err)
	}

	jsonUpdateData, err := ioutil.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("%s: %v", ErrReadJSON, err)
	}
	rc.Close()

	var data SignedUpdate
	err = json.Unmarshal(jsonUpdateData, &data)
	if err != nil {
		return fmt.Errorf("%s: %v", ErrUnmarshalJSON, err)
	}

	buf := bytes.NewBuffer(data.Patch)
	r := bufio.NewReader(buf)
	rcPatch := ioutil.NopCloser(r)

	digest := strings.TrimSpace(string(data.Digest))
	sSign := hex.EncodeToString(data.Signature)
	err = pc.ApplyVerifiedPatchUpdate(rcPatch, digest, sSign)
	if err != nil {
		return fmt.Errorf("%s: %v", ErrApplyUpdate, err)
	}
	return rcPatch.Close()
}

// SignedUpdate contains data required to validate and verify patch
// the applied patch. If the Digest is not correct, the Patch should
// not be applied, if the Signature can not be verified, the patch
// will return an error and you can rollback the patch.
type SignedUpdate struct {
	// Patch contains the binary diff of current to next version
	Patch []byte `json:"patch"`
	// Signature contains the signature of the next version binary
	// to verify, if the resulting binary patch is correct.
	Signature []byte `json:"signature"`
	// Digest is the SHA256 of the Patch
	Digest []byte `json:"sha256"`
}

func GetLocalBinaryName() string {
	binSlice := strings.Split(os.Args[0], "/")
	return binSlice[len(binSlice)-1]
}

// GetUpdate returns an open io.ReadCloser, if error is not
// nil. Caller has to close the io.ReadCloser.
func GetUpdate(baseUpdateURL, version string) (io.ReadCloser, error) {
	binary := GetLocalBinaryName()
	updateURL := getUpdateURL(baseUpdateURL, binary, version)
	rc, err := getUpdate(updateURL.String())
	if err != nil {
		return nil, fmt.Errorf("failed to getUpdate: %v", err)
	}
	return rc, nil
}

// ApplyUpdate is the simplest version of applying an update. It
// patches a full binary without checking a signature.
func (pc *PatchClient) ApplyUpdate(rc io.ReadCloser) error {
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
func (pc *PatchClient) ApplyUpdateWithPatch(patch io.Reader) error {
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
func (pc *PatchClient) ApplyVerifiedUpdate(binary io.ReadCloser, hexChecksum, hexSignature string) error {
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
	err = opts.SetPublicKeyPEM(pc.PublicKey)
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
func (pc *PatchClient) ApplyVerifiedPatchUpdate(binary io.ReadCloser, hexChecksum, hexSignature string) error {
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
	err = opts.SetPublicKeyPEM(pc.PublicKey)
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
