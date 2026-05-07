package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	updateRepo         = "AgusRdz/ctxgate"
	updateAPIURL       = "https://api.github.com/repos/" + updateRepo + "/releases/latest"
	updatePublicKeyURL = "https://raw.githubusercontent.com/" + updateRepo + "/main/go/public_key.pem"
	updateHTTPTimeout  = 30 * time.Second
)

func init() {
	rootCmd.AddCommand(newUpdateCmd())
}

func newUpdateCmd() *cobra.Command {
	var checkOnly bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update ctxgate to the latest release",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(checkOnly)
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, "check for updates without installing")
	return cmd
}

func runUpdate(checkOnly bool) error {
	if version == "dev" {
		fmt.Println("[ctxgate] dev build — update not supported")
		return nil
	}

	fmt.Printf("[ctxgate] current: %s\n", version)
	fmt.Println("[ctxgate] checking for updates...")

	latest, downloadURL, err := updateFetchLatest()
	if err != nil {
		return fmt.Errorf("fetch release: %w", err)
	}

	if !updateIsNewer(latest, version) {
		fmt.Printf("[ctxgate] already up to date (%s)\n", version)
		return nil
	}

	fmt.Printf("[ctxgate] new version: %s\n", latest)
	if checkOnly {
		fmt.Println("[ctxgate] run 'ctxgate update' to install")
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	fmt.Printf("[ctxgate] downloading %s...\n", latest)
	binary, err := updateHTTPGet(downloadURL)
	if err != nil {
		return fmt.Errorf("download binary: %w", err)
	}

	checksums, err := updateHTTPGet(updateReleaseURL(latest, "checksums.txt"))
	if err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}

	sigHex, err := updateHTTPGet(updateReleaseURL(latest, "checksums.txt.sig"))
	if err != nil {
		return fmt.Errorf("download signature: %w", err)
	}

	pubKeyPEM, err := updateHTTPGet(updatePublicKeyURL)
	if err != nil {
		return fmt.Errorf("download public key: %w", err)
	}

	binaryName := updateBinaryName()

	if err := updateVerifyChecksum(binary, binaryName, checksums); err != nil {
		return fmt.Errorf("checksum verification: %w", err)
	}
	fmt.Println("[ctxgate] checksum OK")

	if err := updateVerifySignature(pubKeyPEM, checksums, sigHex); err != nil {
		return fmt.Errorf("signature verification: %w", err)
	}
	fmt.Println("[ctxgate] signature OK")

	if err := updateAtomicReplace(exe, binary); err != nil {
		return fmt.Errorf("install: %w", err)
	}

	fmt.Printf("[ctxgate] updated to %s\n", latest)
	return nil
}

func updateFetchLatest() (tag, downloadURL string, err error) {
	body, err := updateHTTPGet(updateAPIURL)
	if err != nil {
		return "", "", err
	}
	var resp struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", "", fmt.Errorf("parse response: %w", err)
	}
	tag = resp.TagName
	binaryName := updateBinaryName()
	for _, a := range resp.Assets {
		if a.Name == binaryName {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		downloadURL = updateReleaseURL(tag, binaryName)
	}
	return tag, downloadURL, nil
}

func updateBinaryName() string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return fmt.Sprintf("ctxgate-%s-%s%s", runtime.GOOS, runtime.GOARCH, ext)
}

func updateReleaseURL(tag, file string) string {
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", updateRepo, tag, file)
}

func updateHTTPGet(url string) ([]byte, error) {
	client := &http.Client{Timeout: updateHTTPTimeout}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ctxgate/"+version)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

func updateVerifyChecksum(data []byte, binaryName string, checksums []byte) error {
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == binaryName {
			if !strings.EqualFold(actual, fields[0]) {
				return fmt.Errorf("expected %s got %s", fields[0], actual)
			}
			return nil
		}
	}
	return fmt.Errorf("no entry for %s in checksums.txt", binaryName)
}

func updateVerifySignature(pubKeyPEM, message, sigHex []byte) error {
	block, _ := pem.Decode(pubKeyPEM)
	if block == nil {
		return fmt.Errorf("invalid public key PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse public key: %w", err)
	}
	edPub, ok := pub.(ed25519.PublicKey)
	if !ok {
		return fmt.Errorf("key is not Ed25519")
	}
	sig, err := hex.DecodeString(strings.TrimSpace(string(sigHex)))
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	if !ed25519.Verify(edPub, message, sig) {
		return fmt.Errorf("signature invalid")
	}
	return nil
}

func updateAtomicReplace(dst string, data []byte) error {
	tmp := fmt.Sprintf("%s.%d.tmp", dst, os.Getpid())
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		// Windows: the running binary may be locked; retry once after a brief pause.
		time.Sleep(10 * time.Millisecond)
		if err2 := os.Rename(tmp, dst); err2 != nil {
			os.Remove(tmp)
			return err2
		}
	}
	return nil
}

func updateIsNewer(latest, current string) bool {
	l := parseSemver(strings.TrimPrefix(latest, "v"))
	c := parseSemver(strings.TrimPrefix(current, "v"))
	for i := range l {
		if l[i] > c[i] {
			return true
		}
		if l[i] < c[i] {
			return false
		}
	}
	return false
}

func parseSemver(s string) [3]int {
	parts := strings.SplitN(s, ".", 3)
	var v [3]int
	for i, p := range parts {
		if i >= 3 {
			break
		}
		v[i], _ = strconv.Atoi(p)
	}
	return v
}
