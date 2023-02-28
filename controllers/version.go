package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	goversion "github.com/hashicorp/go-version"
	"github.com/pkg/errors"
)

const (
	defaultVersionServiceURL        = "https://check.percona.com/versions/v1/psmdb-operator/%s"
	versionServiceStatusRecommended = "recommended"
	psmdbBackupImageTmpl            = "percona/percona-server-mongodb-operator:%s-backup"
)

type (
	Version struct {
		version *goversion.Version
	}
	Image struct {
		ImagePath string `json:"imagePath"`
		ImageHash string `json:"imageHash"`
		Status    string `json:"status"`
	}
	VersionResponse struct {
		Versions []struct {
			Matrix struct {
				Backup map[string]Image `json:"backup"`
			} `json:"matrix"`
		} `json:"versions"`
	}
)

func NewVersion(v string) (*Version, error) {
	version, err := goversion.NewVersion(v)
	if err != nil {
		return nil, err
	}
	return &Version{version: version}, nil
}
func (v *Version) String() string {
	return v.version.String()
}
func (v *Version) ToCRVersion() string {
	return strings.Replace(v.String(), "v", "", -1)
}
func (v *Version) ToSemver() string {
	return fmt.Sprintf("v%s", v.String())
}
func (v *Version) ToAPIVersion(apiRoot string) string {
	ver, _ := goversion.NewVersion("v1.12.0")
	if v.version.GreaterThan(ver) {
		return fmt.Sprintf("%s/v1", apiRoot)
	}
	return fmt.Sprintf("%s/v%s", apiRoot, strings.Replace(v.String(), ".", "-", -1))
}

func (v *Version) PSMDBBackupImage() (string, error) {
	ver, _ := goversion.NewVersion("v1.11.0")
	if v.version.GreaterThan(ver) {
		resp, err := http.Get(fmt.Sprintf(defaultVersionServiceURL, v.ToCRVersion()))
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		var vr VersionResponse
		if err := json.NewDecoder(resp.Body).Decode(&vr); err != nil {
			return "", err
		}
		if len(vr.Versions) == 0 {
			return "", errors.New("no versions returned from version service")
		}

		for _, image := range vr.Versions[0].Matrix.Backup {
			if image.Status == versionServiceStatusRecommended {
				return image.ImagePath, nil
			}
		}
		return "", errors.New("no versions available for backup image")
	}
	return fmt.Sprintf(psmdbBackupImageTmpl, v.ToCRVersion()), nil
}
