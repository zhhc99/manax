package config

import (
	"os"
	"path/filepath"
)

// GeoDataFiles are the kernel data files manax installs system-wide.
var GeoDataFiles = []string{"geoip.dat", "geosite.dat", "country.mmdb", "GeoLite2-ASN.mmdb"}

// LinkGeoData exposes the system GeoData files in the kernel work directory.
// The work directory lives under DataDir, so the links never cross a file
// system, and replacing a GeoData file at install time breaks the link, in
// which case it is recreated here.
func LinkGeoData() error {
	for _, name := range GeoDataFiles {
		src := filepath.Join(DataDir, name)
		if _, err := os.Stat(src); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		dst := filepath.Join(WorkDir, name)
		if same, err := sameFile(src, dst); err == nil && same {
			continue
		}
		_ = os.Remove(dst)
		if err := os.Link(src, dst); err != nil {
			return err
		}
	}
	return nil
}

func sameFile(a, b string) (bool, error) {
	ai, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(ai, bi), nil
}
