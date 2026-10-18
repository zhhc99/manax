package service

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zhhc99/manax/internal/config"
	"github.com/zhhc99/manax/internal/locale"
)

const geoDataRepo = "MetaCubeX/meta-rules-dat"

const githubConnectivityTimeout = 10 * time.Second

// HasInstallResources reports whether the kernel and all GeoData needed by
// Manax are already installed system-wide.
func HasInstallResources() bool {
	info, err := os.Stat(config.KernelPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return false
	}
	for _, name := range config.GeoDataFiles {
		info, err := os.Stat(filepath.Join(config.DataDir, name))
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return false
		}
	}
	return true
}

const resourceKernelName = "kernel"

type releaseAsset struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
}

type releaseInfo struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

// BuildGitHubResourcePackage downloads and verifies the kernel and GeoData.
func BuildGitHubResourcePackage(ctx context.Context, dst string) error {
	work, err := os.MkdirTemp("", "manax-install-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	p := newProgress(5)
	defer p.finish()
	p.startFile(fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", KernelRepo))

	kernel, err := latestKernelRelease(ctx)
	if err != nil {
		return err
	}
	if kernel.SHA256 == "" {
		return errors.New("missing kernel SHA256")
	}
	compressed := filepath.Join(work, kernel.Asset)
	p.startFile(kernel.URL)
	if err := downloadResource(ctx, kernel.URL, compressed, kernel.SHA256, p); err != nil {
		return err
	}
	p.completeFile()
	binaryPath := filepath.Join(work, resourceKernelName)
	if err := unpackKernel(compressed, binaryPath); err != nil {
		return err
	}
	files := map[string]string{resourceKernelName: binaryPath}
	p.startFile(fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", geoDataRepo))
	assets, err := latestGeoDataAssets(ctx)
	if err != nil {
		return err
	}
	for _, name := range config.GeoDataFiles {
		asset, ok := assets[name]
		if !ok || asset.SHA256 == "" {
			return fmt.Errorf("missing checksum for %s", name)
		}
		path := filepath.Join(work, name)
		p.startFile(asset.URL)
		if err := downloadResource(ctx, asset.URL, path, asset.SHA256, p); err != nil {
			return err
		}
		p.completeFile()
		files[name] = path
	}
	if err := writeResourcePackage(dst, files); err != nil {
		return err
	}
	return nil
}

// CheckGitHubConnectivity checks the release sources needed for this install.
func CheckGitHubConnectivity(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, githubConnectivityTimeout)
	defer cancel()
	if _, err := fetchRelease(ctx, KernelRepo, "latest"); err != nil {
		return err
	}
	_, err := fetchRelease(ctx, geoDataRepo, "latest")
	return err
}

// DownloadResourcePackage downloads a user-selected ZIP URL into dst.
func DownloadResourcePackage(ctx context.Context, url, dst string) error {
	p := newProgress(1)
	defer p.finish()
	p.startFile(url)
	if err := downloadResource(ctx, url, dst, "", p); err != nil {
		return err
	}
	p.completeFile()
	return nil
}

// ValidateResourcePackage checks for the platform kernel and all four GeoData files.
func ValidateResourcePackage(path string) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("%s: %w", locale.T("err.resource_zip"), err)
	}
	defer zr.Close()
	seen := map[string]bool{}
	kernels := 0
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if contains(config.GeoDataFiles, f.Name) {
			if seen[f.Name] {
				return fmt.Errorf("%s: duplicate %s", locale.T("err.resource_zip"), f.Name)
			}
			seen[f.Name] = true
		} else {
			kernels++
		}
	}
	if kernels != 1 {
		return errors.New(locale.T("err.resource_missing_kernel"))
	}
	for _, name := range config.GeoDataFiles {
		if !seen[name] {
			return fmt.Errorf("%s: %s", locale.T("err.resource_missing_file"), name)
		}
	}
	return nil
}

// InstallKernelPackage installs the platform kernel from a ZIP.
func InstallKernelPackage(path string) error {
	if err := ValidateResourcePackage(path); err != nil {
		return err
	}
	if err := os.MkdirAll(config.InstallDir, 0755); err != nil {
		return err
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || contains(config.GeoDataFiles, f.Name) {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		dst := config.KernelPath
		err = writeAtomicMode(dst, 0755, func(out *os.File) error {
			_, copyErr := io.Copy(out, r)
			return copyErr
		})
		_ = r.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// InstallGeoDataPackage writes the shared GeoData into the system data
// directory, where ordinary users can read but not modify it.
func InstallGeoDataPackage(path, destination string) error {
	if err := ValidateResourcePackage(path); err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0755); err != nil {
		return err
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if !contains(config.GeoDataFiles, f.Name) {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		err = writeAtomicMode(filepath.Join(destination, f.Name), 0644, func(out *os.File) error {
			_, copyErr := io.Copy(out, r)
			return copyErr
		})
		_ = r.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func latestKernelRelease(ctx context.Context) (KernelRelease, error) {
	info, err := fetchRelease(ctx, KernelRepo, "latest")
	if err != nil {
		return KernelRelease{}, fmt.Errorf(locale.T("err.kernel_release"), err)
	}
	assetName, err := assetName(info.TagName)
	if err != nil {
		return KernelRelease{}, err
	}
	for _, a := range info.Assets {
		if a.Name == assetName {
			return KernelRelease{
				Asset:  a.Name,
				URL:    fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", KernelRepo, info.TagName, a.Name),
				SHA256: digestValue(a.Digest),
			}, nil
		}
	}
	return KernelRelease{}, fmt.Errorf("release asset not found: %s", assetName)
}

type geoAsset struct {
	URL    string
	SHA256 string
}

func latestGeoDataAssets(ctx context.Context) (map[string]geoAsset, error) {
	info, err := fetchRelease(ctx, geoDataRepo, "latest")
	if err != nil {
		return nil, err
	}
	out := make(map[string]geoAsset, len(config.GeoDataFiles))
	for _, asset := range info.Assets {
		if contains(config.GeoDataFiles, asset.Name) {
			out[asset.Name] = geoAsset{
				URL:    fmt.Sprintf("https://github.com/%s/releases/latest/download/%s", geoDataRepo, asset.Name),
				SHA256: digestValue(asset.Digest),
			}
		}
	}
	return out, nil
}

func fetchRelease(ctx context.Context, repo, tag string) (releaseInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/%s", repo, tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return releaseInfo{}, err
	}
	req.Header.Set("User-Agent", "manax")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return releaseInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return releaseInfo{}, fmt.Errorf("http %d", resp.StatusCode)
	}
	var out releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return releaseInfo{}, err
	}
	return out, nil
}

func digestValue(digest string) string {
	return strings.TrimPrefix(digest, "sha256:")
}

func downloadResource(ctx context.Context, url, dst, expectedSHA256 string, p *progressBody) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "manax")
	client := http.DefaultClient
	if parsed, parseErr := neturl.Parse(url); parseErr == nil && parsed.Hostname() == "home.ustc.edu.cn" {
		client = &http.Client{Transport: &http.Transport{Proxy: nil}}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	p.attach(resp.Body, resp.ContentLength)
	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(f, h), p)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(dst)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(dst)
		return closeErr
	}
	if expectedSHA256 != "" && !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), expectedSHA256) {
		_ = os.Remove(dst)
		return errors.New("SHA256 mismatch")
	}
	return nil
}

func unpackKernel(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if strings.HasSuffix(src, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return err
		}
		for _, f := range zr.File {
			if !f.FileInfo().IsDir() && strings.HasSuffix(strings.ToLower(f.Name), ".exe") {
				r, err := f.Open()
				if err != nil {
					return err
				}
				err = writeBinary(dst, r)
				_ = r.Close()
				return err
			}
		}
		return errors.New("kernel ZIP contains no executable")
	}
	var r io.Reader = bytes.NewReader(data)
	if strings.HasSuffix(src, ".gz") {
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return err
		}
		defer gz.Close()
		r = gz
	}
	return writeBinary(dst, r)
}

func writeResourcePackage(dst string, files map[string]string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	for name, path := range files {
		in, err := os.Open(path)
		if err != nil {
			zw.Close()
			f.Close()
			return err
		}
		out, err := zw.Create(name)
		if err == nil {
			_, err = io.Copy(out, in)
		}
		_ = in.Close()
		if err != nil {
			zw.Close()
			f.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func contains(items []string, s string) bool {
	for _, item := range items {
		if item == s {
			return true
		}
	}
	return false
}
