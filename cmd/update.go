package cmd

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const githubReleasesURL = "https://api.github.com/repos/usemindex/cli/releases/latest"

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update the mindex CLI to the latest version",
	Long:  `Checks GitHub Releases for the latest version and replaces the current binary if a newer version is available.`,
	RunE:  runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	return runUpdateWithURL(cmd, args, githubReleasesURL)
}

// runUpdateWithURL executa a lógica de atualização com uma URL configurável (facilita testes).
func runUpdateWithURL(cmd *cobra.Command, args []string, releasesURL string) error {
	w := cmd.OutOrStdout()

	fmt.Fprintln(w, "  Checking for updates...")

	release, err := fetchLatestRelease(releasesURL)
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	latestTag, ok := release["tag_name"].(string)
	if !ok || latestTag == "" {
		return fmt.Errorf("failed to check for updates: invalid response from GitHub")
	}

	currentVersion := normalizeVersion(Version)
	latestVersion := normalizeVersion(latestTag)

	if currentVersion == latestVersion {
		fmt.Fprintf(w, "  Already up to date (%s)\n", latestTag)
		return nil
	}

	assetURL, assetName, err := findAsset(release)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "  Downloading %s...\n", latestTag)

	tmpFile, err := downloadAsset(assetURL)
	if err != nil {
		return fmt.Errorf("failed to download update: %w", err)
	}
	defer os.Remove(tmpFile)

	newBinary, err := extractBinary(tmpFile, assetName)
	if err != nil {
		return fmt.Errorf("failed to extract binary: %w", err)
	}
	defer os.Remove(newBinary)

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to determine current binary path: %w", err)
	}

	if err := replaceBinary(execPath, newBinary); err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("update downloaded but failed to replace binary. Run with sudo: sudo mindex update")
		}
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	fmt.Fprintf(w, "  \u2713 Updated to %s (was %s)\n", latestTag, Version)
	return nil
}

// fetchLatestRelease consulta a API do GitHub e retorna o release mais recente.
func fetchLatestRelease(url string) (map[string]any, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "mindex-cli/"+Version)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// findAsset localiza o asset correto para o OS/ARCH atual.
func findAsset(release map[string]any) (downloadURL, assetName string, err error) {
	return findAssetForPlatform(release, runtime.GOOS, runtime.GOARCH)
}

// findAssetForPlatform localiza o asset para um OS/ARCH específico (testável).
func findAssetForPlatform(release map[string]any, goos, goarch string) (downloadURL, assetName string, err error) {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}

	// padrão: mindex_{os}_{arch}.tar.gz
	wanted := fmt.Sprintf("mindex_%s_%s%s", goos, goarch, ext)

	assets, ok := release["assets"].([]any)
	if !ok {
		return "", "", fmt.Errorf("no binary available for %s/%s", goos, goarch)
	}

	for _, a := range assets {
		asset, ok := a.(map[string]any)
		if !ok {
			continue
		}
		name, _ := asset["name"].(string)
		if name == wanted {
			url, _ := asset["browser_download_url"].(string)
			return url, name, nil
		}
	}

	return "", "", fmt.Errorf("no binary available for %s/%s", goos, goarch)
}

// downloadAsset faz o download do asset para um arquivo temporário e retorna seu caminho.
func downloadAsset(url string) (string, error) {
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url) //nolint:noctx
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "mindex-update-*")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}

// extractBinary extrai o binário mindex do arquivo comprimido e retorna o caminho do arquivo extraído.
func extractBinary(archivePath, assetName string) (string, error) {
	if strings.HasSuffix(assetName, ".zip") {
		return extractFromZip(archivePath)
	}
	return extractFromTarGz(archivePath)
}

// extractFromTarGz extrai o binário de um .tar.gz.
func extractFromTarGz(archivePath string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		// procura pelo binário mindex (com ou sem extensão)
		base := filepath.Base(hdr.Name)
		if base == "mindex" || base == "mindex.exe" {
			tmpFile, err := os.CreateTemp("", "mindex-bin-*")
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(tmpFile, tr); err != nil {
				tmpFile.Close()
				os.Remove(tmpFile.Name())
				return "", err
			}
			tmpFile.Close()
			if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
				os.Remove(tmpFile.Name())
				return "", err
			}
			return tmpFile.Name(), nil
		}
	}

	return "", fmt.Errorf("binary 'mindex' not found in archive")
}

// extractFromZip extrai o binário de um .zip (Windows).
func extractFromZip(archivePath string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer r.Close()

	for _, f := range r.File {
		base := filepath.Base(f.Name)
		if base == "mindex" || base == "mindex.exe" {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			defer rc.Close()

			tmpFile, err := os.CreateTemp("", "mindex-bin-*")
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(tmpFile, rc); err != nil {
				tmpFile.Close()
				os.Remove(tmpFile.Name())
				return "", err
			}
			tmpFile.Close()
			if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
				os.Remove(tmpFile.Name())
				return "", err
			}
			return tmpFile.Name(), nil
		}
	}

	return "", fmt.Errorf("binary 'mindex' not found in zip archive")
}

// replaceBinary substitui o binário atual pelo novo.
// Estratégia: renomeia o atual para .bak, move o novo para o lugar, remove o .bak.
func replaceBinary(execPath, newBinary string) error {
	bakPath := execPath + ".bak"

	// remove .bak anterior se existir
	os.Remove(bakPath)

	if err := os.Rename(execPath, bakPath); err != nil {
		return err
	}

	if err := os.Rename(newBinary, execPath); err != nil {
		// tenta restaurar o original
		os.Rename(bakPath, execPath)
		return err
	}

	os.Remove(bakPath)
	return nil
}

// normalizeVersion remove o prefixo "v" para comparação uniforme.
func normalizeVersion(v string) string {
	return strings.TrimPrefix(v, "v")
}
