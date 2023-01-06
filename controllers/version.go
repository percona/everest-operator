package controllers

import (
	"fmt"
	"strings"

	goversion "github.com/hashicorp/go-version"
)

type Version struct {
	version *goversion.Version
}

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
