package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"

	"github.com/szuecs/binary-patch/patchclient"

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
		pc := patchclient.NewInsecurePatchClient(*baseUpdateURL, Version)
		err := pc.UnsignedNotVerifiedUpdate()
		if err != nil {
			log.Fatalf("Failed to update: %v", err)
		}

	case patchUpdate.FullCommand():
		pc := patchclient.NewInsecurePatchClient(*basePatchUpdateURL, Version)
		err := pc.UnsignedNotVerifiedPatchUpdate()
		if err != nil {
			log.Fatalf("Failed to update: %v", err)
		}

	case signedUpdate.FullCommand():
		pc := patchclient.NewPatchClient(*baseSignedUpdateURL, Version, publicKey)
		err := pc.SignedVerifiedUpdate()
		if err != nil {
			log.Fatalf("Failed to update: %v", err)
		}

	case signedPatchUpdate.FullCommand():
		pc := patchclient.NewPatchClient(*baseSignedPatchUpdateURL, Version, publicKey)
		err := pc.SignedVerifiedPatchUpdate()
		if err != nil {
			log.Fatalf("Failed to update: %v", err)
		}
	}
}
