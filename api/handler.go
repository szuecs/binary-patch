package api

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"
	"github.com/kr/binarydist"
	"github.com/pkg/errors"
)

var (
	basedir                = "/tmp/bindata" // TODO: choose by cli flag
	errVersionMissing      = errors.New("Missing 'version' in query string")
	errArchitectureMissing = errors.New("Missing 'arch' in query string")
	errOSMissing           = errors.New("Missing 'os' in query string")
	errUnsupportedArchOS   = errors.New("Wrong architecture and OS combination")
	errBinaryNotFound      = errors.New("Binary not found: ")
	supported              = map[ArchAndOS]bool{
		ArchAndOS{Arch: "amd64", OS: "linux"}:  true,
		ArchAndOS{Arch: "amd64", OS: "darwin"}: true,
	}
)

type Update struct {
	Name    string
	Version string
	System  ArchAndOS
}

type ArchAndOS struct {
	Arch string // GOARCH
	OS   string // GOOS
}

func newUpdate(ginCtx *gin.Context) (*Update, error) {
	version, ok := ginCtx.GetQuery("version")
	if !ok {
		return nil, errVersionMissing
	}
	goarch, ok := ginCtx.GetQuery("arch")
	if !ok {
		return nil, errArchitectureMissing
	}
	goos, ok := ginCtx.GetQuery("os")
	if !ok {
		return nil, errOSMissing
	}
	return &Update{
		Name:    ginCtx.Param("name"),
		Version: version,
		System: ArchAndOS{
			Arch: goarch,
			OS:   goos,
		},
	}, nil
}

func (u *Update) Clone() *Update {
	return &Update{
		Name:    u.Name,
		Version: u.Version,
		System: ArchAndOS{
			Arch: u.System.Arch,
			OS:   u.System.OS,
		},
	}
}

func (u *Update) isSupported() bool {
	_, ok := supported[u.System]
	return ok
}

func (u *Update) GetFilepath() string {
	return basedir + "/" + u.Name + "_" + u.Version + "_" + u.System.Arch + u.System.OS
}

func (u *Update) GetReader(filepath string) (io.ReadCloser, error) {
	fd, err := os.Open(filepath)
	if err != nil {
		return nil, errors.Wrap(errBinaryNotFound, filepath)
	}
	return fd, nil
}

func (u *Update) String() string {
	return u.Name + "_" + u.Version + "_" + u.System.Arch + u.System.OS
}

func (u *Update) versionFromFilename(filepath string) string {
	return filepath[len(basedir)+1+len(u.Name)+1 : strings.LastIndex(filepath, "_")]
}

func (u *Update) GetLatestVersion() string {
	globStr := strings.Replace(u.String(), u.Version, "*", 1)
	glog.V(2).Infof("Use globStr: %s", globStr)
	files, err := filepath.Glob(fmt.Sprintf("%s/%s", basedir, globStr))
	if err != nil {
		glog.Errorf("Could not glob filepath '%s', caused by: %v", globStr, err)
		return ""
	}
	glog.V(2).Infof("found %d files by glob", len(files))
	latest := u.Version
	for _, fname := range files {
		v := u.versionFromFilename(fname)
		if v > latest {
			latest = v
		}
	}
	return latest
}

func newUpdateFromCtx(ginCtx *gin.Context) *Update {
	update, err := newUpdate(ginCtx)
	if err != nil {
		ginCtx.AbortWithError(http.StatusBadRequest, err)
	}

	if !update.isSupported() {
		ginCtx.AbortWithError(http.StatusBadRequest, errUnsupportedArchOS)
	}
	return update
}

// UpdateHandler handles /update/:name endpoint
func (svc *Service) UpdateHandler(ginCtx *gin.Context) {
	update := newUpdateFromCtx(ginCtx)
	curVersion := update.Version
	if latest := update.GetLatestVersion(); latest != "" {
		update.Version = latest
	}
	glog.V(2).Infof("client hast version %s, we have latest version %s", curVersion, update.Version)
	if update.Version == curVersion {
		ginCtx.String(http.StatusNotModified, "")
		return
	}
	fpath := update.GetFilepath()
	rc, err := update.GetReader(fpath)
	if err != nil {
		ginCtx.AbortWithError(http.StatusInternalServerError, err) // TODO: AbortWithError creates StackTraces, we want to have 4xx and an error log
	}
	defer rc.Close()
	n, err := io.Copy(ginCtx.Writer, rc)
	if err != nil {
		ginCtx.AbortWithError(http.StatusInternalServerError, fmt.Errorf("Could not copy %s to client, caused by: %v", update.Name, err))
	}
	glog.Infof("Copied %d bytes to client to update %s", n, update)
}

// PatchUpdateHandler handles /patch-update/:name endpoint
// TODO: use file cache to read and store binarydiff
func (svc *Service) PatchUpdateHandler(ginCtx *gin.Context) {
	newUpdate := newUpdateFromCtx(ginCtx)
	latestVersion := newUpdate.GetLatestVersion()
	glog.V(2).Infof("client has version %s, we have latest version %s", newUpdate.Version, latestVersion)
	if newUpdate.Version == latestVersion {
		ginCtx.String(http.StatusNotModified, "")
		return
	}
	oldUpdate := newUpdate.Clone()

	newUpdate.Version = latestVersion
	fpath := newUpdate.GetFilepath()
	rcNew, err := newUpdate.GetReader(fpath)
	if err != nil {
		ginCtx.AbortWithError(http.StatusInternalServerError, err) // TODO: AbortWithError creates StackTraces, we want to have 4xx and an error log
	}
	defer rcNew.Close()

	glog.Infof("old: %v, new: %v", oldUpdate, newUpdate)
	fpath = oldUpdate.GetFilepath()
	rcOld, err := oldUpdate.GetReader(fpath)
	if err != nil {
		ginCtx.AbortWithError(http.StatusInternalServerError, err) // TODO: AbortWithError creates StackTraces, we want to have 4xx and an error log
	}
	defer rcOld.Close()

	buf := bytes.NewBuffer(nil)
	rw := bufio.NewReadWriter(bufio.NewReader(buf), bufio.NewWriter(buf))

	err = binarydist.Diff(rcOld, rcNew, rw)
	if err != nil {
		ginCtx.AbortWithError(http.StatusInternalServerError, fmt.Errorf("failed to create a binary patch for %s: %v", newUpdate.Name, err))
	}
	rw.Flush()

	n, err := io.Copy(ginCtx.Writer, rw)
	if err != nil {
		ginCtx.AbortWithError(http.StatusInternalServerError, fmt.Errorf("failed to copy %s to client: %v", newUpdate.Name, err))
	}
	glog.Infof("Copied %d bytes to client to patch %s", n, newUpdate)

}

// SignedUpdateHandler handles /signed-update/:name endpoint
func (svc *Service) SignedUpdateHandler(ginCtx *gin.Context) {
	newUpdate := newUpdateFromCtx(ginCtx)
	latestVersion := newUpdate.GetLatestVersion()
	glog.V(2).Infof("client has version %s, we have latest version %s", newUpdate.Version, latestVersion)
	if newUpdate.Version == latestVersion {
		ginCtx.String(http.StatusNotModified, "")
		return
	}

	newUpdate.Version = latestVersion
	fpath := newUpdate.GetFilepath()

	binPatch, err := ioutil.ReadFile(fpath)
	if err != nil {
		ginCtx.AbortWithError(http.StatusInternalServerError, err) // TODO: AbortWithError creates StackTraces, we want to have 4xx and an error log
	}
	signature, err := ioutil.ReadFile(fpath + ".signature")
	if err != nil {
		ginCtx.AbortWithError(http.StatusInternalServerError, err) // TODO: AbortWithError creates StackTraces, we want to have 4xx and an error log
	}
	digest, err := ioutil.ReadFile(fpath + ".sha256")
	if err != nil {
		ginCtx.AbortWithError(http.StatusInternalServerError, err) // TODO: AbortWithError creates StackTraces, we want to have 4xx and an error log
	}

	ginCtx.JSON(http.StatusOK, gin.H{
		"patch":     binPatch,
		"signature": signature,
		"sha256":    digest,
	})
	glog.Infof("Copied %d bytes to client to update %s", len(binPatch), newUpdate)
}

// SignedPatchUpdateHandler handles /signed-patch-update/:name endpoint
func (svc *Service) SignedPatchUpdateHandler(ginCtx *gin.Context) {
	newUpdate := newUpdateFromCtx(ginCtx)
	latestVersion := newUpdate.GetLatestVersion()
	glog.V(2).Infof("client has version %s, we have latest version %s", newUpdate.Version, latestVersion)
	if newUpdate.Version == latestVersion {
		ginCtx.String(http.StatusNotModified, "")
		return
	}
	oldUpdate := newUpdate.Clone()

	newUpdate.Version = latestVersion
	fpath := newUpdate.GetFilepath()
	rcNew, err := newUpdate.GetReader(fpath)
	if err != nil {
		ginCtx.AbortWithError(http.StatusInternalServerError, err) // TODO: AbortWithError creates StackTraces, we want to have 4xx and an error log
	}
	defer rcNew.Close()

	glog.Infof("old: %v, new: %v", oldUpdate, newUpdate)
	oldFpath := oldUpdate.GetFilepath()
	rcOld, err := oldUpdate.GetReader(oldFpath)
	if err != nil {
		ginCtx.AbortWithError(http.StatusInternalServerError, err) // TODO: AbortWithError creates StackTraces, we want to have 4xx and an error log
	}
	defer rcOld.Close()

	buf := bytes.NewBuffer(nil)
	rw := bufio.NewReadWriter(bufio.NewReader(buf), bufio.NewWriter(buf))

	err = binarydist.Diff(rcOld, rcNew, rw)
	if err != nil {
		ginCtx.AbortWithError(http.StatusInternalServerError, fmt.Errorf("failed to create a binary patch for %s: %v", newUpdate.Name, err))
	}
	rw.Flush()

	binPatch, err := ioutil.ReadAll(rw)
	if err != nil {
		ginCtx.AbortWithError(http.StatusInternalServerError, err) // TODO: AbortWithError creates StackTraces, we want to have 4xx and an error log
	}

	signature, err := ioutil.ReadFile(fpath + ".signature")
	if err != nil {
		ginCtx.AbortWithError(http.StatusInternalServerError, err) // TODO: AbortWithError creates StackTraces, we want to have 4xx and an error log
	}

	digest, err := ioutil.ReadFile(fpath + ".sha256")
	if err != nil {
		ginCtx.AbortWithError(http.StatusInternalServerError, err) // TODO: AbortWithError creates StackTraces, we want to have 4xx and an error log
	}

	ginCtx.JSON(http.StatusOK, gin.H{
		"patch":     binPatch,
		"signature": signature,
		"sha256":    digest,
	})

	glog.Infof("Copied %d bytes patch to client to patch %s", len(binPatch), newUpdate)
}

// UploadData represents all the data needed to provide update, patch
// update, signed update or signed patch update. You need to sign your
// data if you want to provide signed updates.
type UploadData struct {
	Data          []byte `json:"data"`                     // the new binary version
	Version       string `json:"version"`                  // version string
	Architecture  string `json:"arch"`                     // architecture, p.e. amd64
	OS            string `json:"os"`                       // operating system, p.e. linux
	Signature     []byte `json:"signature,omitempty"`      // DER encoded signature
	SignatureType string `json:"signature-type,omitempty"` // ecdsa
}

func (ud *UploadData) Save(application string) error {
	up := &Update{
		Name:    application,
		Version: ud.Version,
		System: ArchAndOS{
			Arch: ud.Architecture,
			OS:   ud.OS,
		},
	}
	filepath := up.GetFilepath()
	_, err := os.Stat(filepath)
	if err == nil {
		glog.Errorf("Filepath %s already exists: %v", filepath, err)
		return fmt.Errorf("%s already exists", filepath)
	}
	if !os.IsNotExist(err) {
		glog.Errorf("Failed while doing stat(%s): %v", filepath, err)
		return fmt.Errorf("Failed to save %s", filepath)
	}

	ud.Write(filepath, ud.Data)
	// TODO make filepath executable

	// TODO: signature length is fixed size
	if len(ud.Signature) > 0 && validSignatureType(ud.SignatureType) {
		// TODO check if this works as expected
		err = ud.Write(filepath+".signature", ud.Signature)
		if err != nil {
			glog.Errorf("Failed to write %s.signature: %v", filepath, err)
			return fmt.Errorf("Failed to save %s.signature", filepath)
		}
	}

	// TODO check if this works as expected
	hash := sha256.Sum256(ud.Data)
	sum := fmt.Sprintf("%x", hash)
	glog.Infof("Wrote sha256: %s", sum)
	err = ud.Write(filepath+".sha256", []byte(sum))
	if err != nil {
		glog.Errorf("Failed to write %s.sha256: %v", filepath, err)
		return fmt.Errorf("Failed to save %s.sha256", filepath)
	}
	return nil
}

func validSignatureType(s string) bool {
	signatures := map[string]bool{
		"ecdsa": true,
	}
	_, ok := signatures[s]
	return ok
}

func (ud *UploadData) Write(filepath string, data []byte) error {
	fd, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE, 0440)
	if err != nil {
		return err
	}
	defer fd.Close()

	_, err = fd.Write(data)
	return err
}

func returnUploadErr(msg string) gin.H {
	return gin.H{
		"error": msg,
		"example": map[string]string{
			"data":           "your-binary-data",
			"version":        "v0.0.1",
			"arch":           "amd64",
			"os":             "linux",
			"signature":      "Base64-encoded-DER-signature-of-the-binary-data",
			"signature-type": "ecdsa"},
	}
}

// UploadHandler handles /upload/:name endpoint
func (svc *Service) UploadHandler(ginCtx *gin.Context) {
	name := ginCtx.Param("name")

	if ginCtx.ContentType() != "application/json" {
		ginCtx.JSON(http.StatusUnprocessableEntity, returnUploadErr(fmt.Sprintf("Content-Type: application/json required for application '%s'", name)))
		return
	}

	var upload UploadData
	if err := ginCtx.BindJSON(&upload); err != nil {
		ginCtx.JSON(http.StatusUnprocessableEntity, returnUploadErr(fmt.Sprintf("Failed to unmarshal json of application '%s': %v", name, err)))
		return
	}

	if err := upload.Save(name); err != nil {
		ginCtx.JSON(http.StatusUnprocessableEntity, returnUploadErr(fmt.Sprintf("Failed to save provided data for application '%s': %v", name, err)))
		return
	}

	if len(upload.Signature) > 0 {
		ginCtx.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("uploaded signed application '%s' version %s for OS %s and architecture %s", name, upload.Version, upload.OS, upload.Architecture)})
	} else {
		ginCtx.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("uploaded unsigned application '%s' version %s for OS %s and architecture %s", name, upload.Version, upload.OS, upload.Architecture)})
	}
}

// RootHandler handles / endpoint
func (svc *Service) RootHandler(ginCtx *gin.Context) {
	ginCtx.Redirect(http.StatusMovedPermanently, "/healthz")
}

// HealthHandler handles /healthz endpoint
func (svc *Service) HealthHandler(ginCtx *gin.Context) {
	if svc.IsHealthy() {
		ginCtx.String(http.StatusOK, "%s", "OK")
	} else {
		ginCtx.String(http.StatusServiceUnavailable, "%s", "Unavailable")
	}
}

// IsHealthy returns the health status of the running service.
func (svc *Service) IsHealthy() bool {
	return svc.Healthy
}
