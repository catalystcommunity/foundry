package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// pinnedWeedVersion is the SeaweedFS release fetched when no `weed` binary is
// present. SeaweedFS ships a single static binary per platform.
const pinnedWeedVersion = "3.80"

// localS3 runs a SeaweedFS S3 server on localhost, storing data under dataDir.
// It is the off-cluster backup target reached through the reverse tunnel.
type localS3 struct {
	weedBin    string
	dataDir    string
	ip         string
	s3Port     int
	masterPort int
	volumePort int
	filerPort  int
	accessKey  string
	secretKey  string
	bucket     string

	cmd      *exec.Cmd
	s3Config string
	logFile  *os.File
}

// newLocalS3 prepares a localS3 with generated credentials. dataDir holds the
// SeaweedFS volumes (the durable local backup); s3Port is what the tunnel maps.
func newLocalS3(weedBin, dataDir, bucket string, s3Port int) (*localS3, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	return &localS3{
		weedBin:    weedBin,
		dataDir:    dataDir,
		ip:         "127.0.0.1",
		s3Port:     s3Port,
		masterPort: s3Port + 1,
		volumePort: s3Port + 2,
		filerPort:  s3Port + 3,
		accessKey:  randomHex(16),
		secretKey:  randomHex(32),
		bucket:     bucket,
	}, nil
}

func (l *localS3) Endpoint() string { return fmt.Sprintf("http://%s:%d", l.ip, l.s3Port) }

// Start writes the S3 identity config and launches `weed server -s3`, then waits
// for the S3 port to accept connections and creates the bucket.
func (l *localS3) Start(ctx context.Context) error {
	// S3 identity config granting our generated keys admin/read/write.
	cfg := map[string]interface{}{
		"identities": []map[string]interface{}{
			{
				"name":        "foundry",
				"credentials": []map[string]string{{"accessKey": l.accessKey, "secretKey": l.secretKey}},
				"actions":     []string{"Admin", "Read", "Write", "List", "Tagging"},
			},
		},
	}
	cfgBytes, _ := json.MarshalIndent(cfg, "", "  ")
	l.s3Config = filepath.Join(l.dataDir, "s3-config.json")
	if err := os.WriteFile(l.s3Config, cfgBytes, 0o600); err != nil {
		return err
	}

	// Fail fast if the port is already taken — otherwise waitForPort would happily
	// connect to a stale weed from a previous run, and Velero would hit it with
	// mismatched credentials (InvalidAccessKeyId).
	if err := ensurePortFree(l.ip, l.s3Port); err != nil {
		return err
	}

	logPath := filepath.Join(l.dataDir, "weed.log")
	var err error
	l.logFile, err = os.Create(logPath)
	if err != nil {
		return err
	}

	l.cmd = exec.Command(l.weedBin, "server",
		"-dir="+l.dataDir,
		"-ip="+l.ip,
		"-ip.bind="+l.ip,
		"-master.port="+itoa(l.masterPort),
		"-volume.port="+itoa(l.volumePort),
		"-filer", "-filer.port="+itoa(l.filerPort),
		"-s3", "-s3.port="+itoa(l.s3Port),
		"-s3.config="+l.s3Config,
	)
	l.cmd.Stdout = l.logFile
	l.cmd.Stderr = l.logFile
	// Ensure weed is killed if foundry exits unexpectedly (no orphaned server
	// squatting on the port for the next run). Linux-only; no-op elsewhere.
	l.cmd.SysProcAttr = weedSysProcAttr()
	if err := l.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start weed: %w", err)
	}

	// Wait for the S3 port to come up.
	if err := waitForPort(ctx, l.ip, l.s3Port, 60*time.Second); err != nil {
		l.Stop()
		return fmt.Errorf("SeaweedFS S3 did not become ready (see %s): %w", logPath, err)
	}

	// Create the bucket via `weed shell` (no S3 signing needed).
	if err := l.createBucket(ctx); err != nil {
		l.Stop()
		return err
	}
	return nil
}

func (l *localS3) createBucket(ctx context.Context) error {
	master := fmt.Sprintf("%s:%d", l.ip, l.masterPort)
	// Retry: master/filer may need a moment after the S3 port opens.
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		cmd := exec.CommandContext(ctx, l.weedBin, "shell", "-master="+master)
		cmd.Stdin = stringsReader(fmt.Sprintf("s3.bucket.create -name %s\n", l.bucket))
		out, err := cmd.CombinedOutput()
		if err == nil {
			return nil
		}
		lastErr = fmt.Errorf("%v: %s", err, string(out))
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("failed to create bucket %q: %w", l.bucket, lastErr)
}

// Stop terminates the weed process. The data under dataDir is preserved.
func (l *localS3) Stop() {
	if l.cmd != nil && l.cmd.Process != nil {
		_ = l.cmd.Process.Kill()
		_, _ = l.cmd.Process.Wait()
	}
	if l.logFile != nil {
		_ = l.logFile.Close()
	}
}

// ensureWeedBinary returns a usable `weed` path: an explicit override, then a
// system `weed`, then ~/.foundry/bin/weed, otherwise it downloads the pinned
// release for this OS/arch.
func ensureWeedBinary(override, configDir string) (string, error) {
	if override != "" {
		if _, err := os.Stat(override); err != nil {
			return "", fmt.Errorf("--weed-binary %q not found: %w", override, err)
		}
		return override, nil
	}
	if p, err := exec.LookPath("weed"); err == nil {
		return p, nil
	}
	binDir := filepath.Join(configDir, "bin")
	local := filepath.Join(binDir, "weed")
	if fi, err := os.Stat(local); err == nil && fi.Mode()&0o111 != 0 {
		return local, nil
	}
	if err := downloadWeed(binDir); err != nil {
		return "", fmt.Errorf("could not obtain a 'weed' binary (install SeaweedFS or pass --weed-binary): %w", err)
	}
	return local, nil
}

func downloadWeed(binDir string) error {
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	url := fmt.Sprintf("https://github.com/seaweedfs/seaweedfs/releases/download/%s/%s_%s.tar.gz",
		pinnedWeedVersion, runtime.GOOS, runtime.GOARCH)
	fmt.Printf("Fetching SeaweedFS %s for %s/%s...\n", pinnedWeedVersion, runtime.GOOS, runtime.GOARCH)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if filepath.Base(hdr.Name) != "weed" {
			continue
		}
		out, err := os.OpenFile(filepath.Join(binDir, "weed"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	}
	return fmt.Errorf("'weed' binary not found in release archive")
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
