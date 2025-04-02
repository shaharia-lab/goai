package weaviate

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/shaharia-lab/goai/observability"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultWeaviateVersion    = "1.26.6" // Match the Python default
	githubReleaseDownloadURL  = "https://github.com/weaviate/weaviate/releases/download/"
	defaultPort               = 8079
	defaultGRPCPort           = 50060
	defaultHostname           = "127.0.0.1"
	startupTimeout            = 30 * time.Second
	checkInterval             = 100 * time.Millisecond
	defaultScheme             = "http"
	githubAPILatestReleaseURL = "https://api.github.com/repos/weaviate/weaviate/releases/latest"
	weaviateExecutableName    = "weaviate"
)

var (
	// Regex to validate version format: number.number.number optionally followed by -rc/alpha/beta.number
	versionPattern = regexp.MustCompile(`^\d+\.\d{1,2}\.\d{1,2}(?:-(?:rc|beta|alpha)\.\d{1,2})?$`)
)

// Options holds the configuration for the embedded Weaviate instance.
type Options struct {
	PersistenceDataPath string
	BinaryPath          string
	Version             string
	Port                int
	Hostname            string
	GRPCPort            int
	Scheme              string
	AdditionalEnvVars   map[string]string
}

// EmbeddedDB manages the embedded Weaviate instance.
type EmbeddedDB struct {
	options       Options
	process       *os.Process
	binaryPath    string
	downloadURL   string
	parsedVersion string
	mu            sync.Mutex
	logger        observability.Logger
}

func getDefaultBinaryPath() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		usr, err := user.Current()
		if err == nil {
			return filepath.Join(usr.HomeDir, ".cache", "weaviate-embedded")
		}
		// Fallback if everything else fails
		return filepath.Join(".", ".cache", "weaviate-embedded")
	}
	return filepath.Join(cacheDir, "weaviate-embedded")
}

func getDefaultPersistenceDataPath() string {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome != "" {
		return filepath.Join(dataHome, "weaviate")
	}

	usr, err := user.Current()
	if err == nil {
		localShare := filepath.Join(usr.HomeDir, ".local", "share")
		_ = os.MkdirAll(localShare, 0755)
		return filepath.Join(localShare, "weaviate")

	}

	return filepath.Join(".", ".local", "share", "weaviate")
}

func AsEmbedded(options Options, logger observability.Logger) (*EmbeddedDB, error) {
	db := &EmbeddedDB{
		options: options,
		logger:  logger,
	}

	if db.options.PersistenceDataPath == "" {
		db.options.PersistenceDataPath = getDefaultPersistenceDataPath()
	}
	if db.options.BinaryPath == "" {
		db.options.BinaryPath = getDefaultBinaryPath()
	}
	if db.options.Version == "" {
		db.options.Version = defaultWeaviateVersion
	}
	if db.options.Port == 0 {
		db.options.Port = defaultPort
	}
	if db.options.Hostname == "" {
		db.options.Hostname = defaultHostname
	}
	if db.options.GRPCPort == 0 {
		db.options.GRPCPort = defaultGRPCPort
	}
	if db.options.Scheme == "" {
		db.options.Scheme = defaultScheme
	}

	if err := checkSupportedPlatform(); err != nil {
		return nil, err
	}

	if err := db.ensurePathsExist(); err != nil {
		return nil, fmt.Errorf("failed to create directories: %w", err)
	}

	if err := db.determineDownloadURL(); err != nil {
		return nil, fmt.Errorf("failed to determine download URL: %w", err)
	}

	versionHash := sha256.Sum256([]byte(db.options.Version))
	db.binaryPath = filepath.Join(
		db.options.BinaryPath,
		fmt.Sprintf("%s-%s-%s", weaviateExecutableName, db.parsedVersion, hex.EncodeToString(versionHash[:])),
	)

	return db, nil
}

func (db *EmbeddedDB) Start() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.process != nil {
		listeningHTTP, listeningGRPC := db.checkListening()
		if listeningHTTP && listeningGRPC {
			db.logger.Infof("Weaviate already running with PID %d and listening on http:%d, grpc:%d", db.process.Pid, db.options.Port, db.options.GRPCPort)
			return nil
		}

		db.logger.Infof("Existing process found (PID %d) but not listening correctly. Attempting to stop and restart.", db.process.Pid)
		if err := db.stopInternal(); err != nil {
			db.logger.Infof("Warning: failed to stop existing process: %v", err)
		}
	}

	errHttp := checkPortFree(db.options.Hostname, db.options.Port)
	errGrpc := checkPortFree(db.options.Hostname, db.options.GRPCPort)
	if errHttp != nil || errGrpc != nil {
		errMsg := "cannot start Weaviate:"
		if errHttp != nil {
			errMsg += fmt.Sprintf(" port %d (HTTP) is already in use", db.options.Port)
		}
		if errGrpc != nil {
			if errHttp != nil {
				errMsg += " and"
			}
			errMsg += fmt.Sprintf(" port %d (gRPC) is already in use", db.options.GRPCPort)
		}

		if errHttp != nil && errGrpc != nil {
			errMsg += fmt.Sprintf(". If a Weaviate instance is running, try connecting using ConnectToLocal(port=%d, grpcPort=%d)", db.options.Port, db.options.GRPCPort)
		}
		return errors.New(errMsg)
	}

	if err := db.ensureBinaryExists(); err != nil {
		return fmt.Errorf("failed to ensure Weaviate binary exists: %w", err)
	}

	cmdArgs := []string{
		"--host", db.options.Hostname,
		"--port", strconv.Itoa(db.options.Port),
		"--scheme", db.options.Scheme,
		"--read-timeout=600s", // Match Python client defaults
		"--write-timeout=600s",
	}

	cmd := exec.Command(db.binaryPath, cmdArgs...)
	cmd.Env = os.Environ()

	setEnv := func(key, value string) {
		_, existsInAdditional := db.options.AdditionalEnvVars[key]
		alreadySet := false
		for _, envVar := range cmd.Env {
			if strings.HasPrefix(envVar, key+"=") {
				alreadySet = true
				break
			}
		}
		if !existsInAdditional && !alreadySet {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
		}
	}

	setEnv("AUTHENTICATION_ANONYMOUS_ACCESS_ENABLED", "true")
	setEnv("QUERY_DEFAULTS_LIMIT", "20")
	setEnv("PERSISTENCE_DATA_PATH", db.options.PersistenceDataPath)
	setEnv("GRPC_PORT", strconv.Itoa(db.options.GRPCPort))
	setEnv("PROFILING_PORT", strconv.Itoa(getRandomPort())) // Less critical, get random

	gossipBindPort := getRandomPort()
	dataBindPort := gossipBindPort + 1 // Convention from Python client
	raftPort := dataBindPort + 1       // Avoid collision with dataBindPort
	raftInternalRPCPort := raftPort + 1

	setEnv("CLUSTER_GOSSIP_BIND_PORT", strconv.Itoa(gossipBindPort))
	setEnv("CLUSTER_DATA_BIND_PORT", strconv.Itoa(dataBindPort))
	setEnv("RAFT_PORT", strconv.Itoa(raftPort))
	setEnv("RAFT_INTERNAL_RPC_PORT", strconv.Itoa(raftInternalRPCPort))

	setEnv("RAFT_BOOTSTRAP_EXPECT", "1")
	// Corrected: Python script had CLUSTER_IN_LOCALHOST, which isn't a standard Weaviate env var.
	// The combination of other settings implies a single-node setup.
	// We need a CLUSTER_HOSTNAME and RAFT_JOIN for Weaviate > 1.18
	clusterHostname := fmt.Sprintf("Embedded_at_%d", db.options.Port)
	setEnv("CLUSTER_HOSTNAME", clusterHostname)
	setEnv("RAFT_JOIN", fmt.Sprintf("%s:%d", clusterHostname, raftPort))

	setEnv("ENABLE_MODULES", "text2vec-openai,text2vec-cohere,text2vec-huggingface,ref2vec-centroid,generative-openai,qna-openai,reranker-cohere")

	// Add user-provided environment variables (potentially overwriting defaults)
	for key, value := range db.options.AdditionalEnvVars {
		// Check if we need to replace an existing var or just append
		found := false
		prefix := key + "="
		for i, envVar := range cmd.Env {
			if strings.HasPrefix(envVar, prefix) {
				cmd.Env[i] = fmt.Sprintf("%s=%s", key, value)
				found = true
				// db.logger.Infof("Overriding env var: %s=%s", key, value) // uncomment for debug
				break
			}
		}
		if !found {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
			// db.logger.Infof("Adding user env var: %s=%s", key, value) // uncomment for debug
		}
	}

	// Redirect stdout/stderr for debugging if needed
	// cmd.Stdout = os.Stdout
	// cmd.Stderr = os.Stderr

	// --- Start the process ---
	db.logger.Infof("Starting Weaviate binary: %s %v", db.binaryPath, cmdArgs)
	// db.logger.Infof("Using Environment: %v", cmd.Env) // uncomment for extensive debugging

	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start Weaviate process: %w", err)
	}
	db.process = cmd.Process
	db.logger.Infof("Weaviate process started with PID %d", db.process.Pid)

	// --- Wait for process to be ready ---
	db.logger.Infof("Waiting for Weaviate to be ready on http:%d and grpc:%d...", db.options.Port, db.options.GRPCPort)
	ready := db.waitForListening(startupTimeout)
	if !ready {
		// Process started but didn't become ready
		db.logger.Infof("Weaviate process (PID %d) did not become ready within %v.", db.process.Pid, startupTimeout)
		// Try to clean up the potentially lingering process
		_ = db.stopInternal() // Ignore error here, main error is startup failure
		return fmt.Errorf("embedded Weaviate did not start listening on ports http:%d, grpc:%d within %v",
			db.options.Port, db.options.GRPCPort, startupTimeout)
	}

	db.logger.Infof("Weaviate is ready!")

	// Start a goroutine to wait for the process to exit unexpectedly
	go db.monitorProcessExit()

	return nil
}

// Stop terminates the running Weaviate process gracefully.
func (db *EmbeddedDB) Stop() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.stopInternal()
}

// stopInternal is the non-locking version for internal use.
func (db *EmbeddedDB) stopInternal() error {
	if db.process == nil {
		db.logger.Info("Stop called but no process was running.")
		return nil // Not running, nothing to do
	}

	pid := db.process.Pid
	db.logger.Infof("Attempting to stop Weaviate process with PID %d...", pid)

	// Try graceful shutdown first (SIGTERM/Interrupt)
	sig := syscall.SIGTERM
	if runtime.GOOS == "windows" {
		sig = syscall.SIGKILL // Windows doesn't really handle SIGTERM well for console apps
	}

	err := db.process.Signal(sig)
	if err != nil {
		// Check if the error is because the process already exited
		if errors.Is(err, os.ErrProcessDone) {
			db.logger.Infof("Process with PID %d already exited.", pid)
			db.process = nil
			return nil
		}
		db.logger.Infof("Failed to send signal %v to process PID %d: %v. Attempting forceful kill.", sig, pid, err)
		// Fallback to SIGKILL if SIGTERM failed (or on Windows initially)
		if killErr := db.process.Kill(); killErr != nil {
			if errors.Is(killErr, os.ErrProcessDone) {
				db.logger.Infof("Process with PID %d already exited before kill.", pid)
				db.process = nil
				return nil
			}
			db.logger.Infof("Failed to kill process PID %d: %v", pid, killErr)
			// Don't clear db.process here, maybe it's just a permissions issue
			return fmt.Errorf("failed to stop process %d: initial signal error: %w, kill error: %v", pid, err, killErr)
		}
	}

	// Wait for the process to exit after signaling
	state, waitErr := db.process.Wait()
	if waitErr != nil && !errors.Is(waitErr, os.ErrProcessDone) {
		// Waiting failed for reasons other than it being already done.
		db.logger.Infof("Error waiting for process %d to exit: %v", pid, waitErr)
		// We might have killed it successfully anyway, hard to tell for sure without state
	} else if state != nil {
		db.logger.Infof("Process %d exited with state: %s", pid, state.String())
	} else {
		db.logger.Infof("Process %d stopped.", pid) // State might be nil if killed forcefully or already exited
	}

	db.process = nil // Mark as stopped
	return nil
}

// IsListening checks if the Weaviate instance is listening on both HTTP and gRPC ports.
func (db *EmbeddedDB) IsListening() bool {
	db.mu.Lock() // Lock is needed if process could be nil
	if db.process == nil {
		db.mu.Unlock()
		return false
	}
	pid := db.process.Pid // Get pid while locked
	db.mu.Unlock()        // Unlock before potentially slow network checks

	// First, quickly check if the process itself is still alive
	proc, err := os.FindProcess(pid)
	if err != nil || proc == nil {
		// Finding failed, highly unlikely it's alive
		return false
	}
	// On Unix-like systems, sending signal 0 checks existence without killing
	if runtime.GOOS != "windows" {
		err = proc.Signal(syscall.Signal(0))
		if err != nil {
			// Process doesn't exist or we lack permission (assume doesn't exist for our purpose)
			return false
		}
	}
	// On Windows, FindProcess followed by a check is less reliable for *running* state.
	// The port check is the primary indicator.

	// Now check the ports
	httpListening, grpcListening := db.checkListening()
	return httpListening && grpcListening
}

// --- Helper Methods ---

// ensurePathsExist creates the binary and data directories if they don't exist.
func (db *EmbeddedDB) ensurePathsExist() error {
	err := os.MkdirAll(db.options.BinaryPath, 0755)
	if err != nil {
		return fmt.Errorf("failed to create binary path %s: %w", db.options.BinaryPath, err)
	}
	err = os.MkdirAll(db.options.PersistenceDataPath, 0755)
	if err != nil {
		return fmt.Errorf("failed to create persistence data path %s: %w", db.options.PersistenceDataPath, err)
	}
	return nil
}

// checkSupportedPlatform returns an error if the current OS/Arch is not supported.
func checkSupportedPlatform() error {
	if runtime.GOOS == "windows" {
		return errors.New("Windows is not currently supported by this embedded package. See https://github.com/weaviate/weaviate/issues/3315")
	}
	// Add more checks if necessary (e.g., specific architectures)
	return nil
}

// determineDownloadURL sets the downloadURL and parsedVersion based on the options.Version.
func (db *EmbeddedDB) determineDownloadURL() error {
	version := db.options.Version

	// 1. Check if version is a direct URL
	_, err := url.ParseRequestURI(version)
	isURL := err == nil

	if isURL {
		if !strings.HasSuffix(version, ".tar.gz") && !strings.HasSuffix(version, ".zip") {
			return fmt.Errorf("invalid version: URL must end with .tar.gz or .zip: %s", version)
		}
		db.downloadURL = version
		// Try to parse version from GitHub URL structure if possible
		if strings.HasPrefix(version, githubReleaseDownloadURL) {
			parts := strings.Split(strings.TrimPrefix(version, githubReleaseDownloadURL), "/")
			if len(parts) > 0 {
				db.parsedVersion = parts[0] // e.g., "v1.26.6"
			} else {
				db.parsedVersion = "unknown-version-from-url" // Fallback if parsing fails
			}
		} else {
			db.parsedVersion = "unknown-version-from-url"
		}
		db.logger.Infof("Using direct download URL: %s (Parsed version: %s)", db.downloadURL, db.parsedVersion)
		return nil
	}

	// 2. Check if version is "latest"
	if version == "latest" {
		db.logger.Info("Fetching latest Weaviate release information from GitHub...")
		tag, err := getLatestWeaviateVersionTag()
		if err != nil {
			return fmt.Errorf("failed to get latest version tag: %w", err)
		}
		db.logger.Infof("Latest Weaviate version tag: %s", tag)
		db.parsedVersion = tag // tag already includes "v"
		// Now build the URL from the tag
		return db.buildDownloadURLFromTag(tag)
	}

	// 3. Check if version matches semantic version pattern
	if versionPattern.MatchString(version) {
		versionTag := "v" + version // Prepend 'v' for GitHub tags
		db.parsedVersion = versionTag
		db.logger.Infof("Using specific version: %s", versionTag)
		return db.buildDownloadURLFromTag(versionTag)
	}

	// 4. Invalid version format
	return fmt.Errorf("invalid version format: %s. Use 'latest', 'X.Y.Z', or a direct URL ending in .tar.gz/.zip", version)
}

// getLatestWeaviateVersionTag fetches the latest release tag from GitHub API.
func getLatestWeaviateVersionTag() (string, error) {
	resp, err := http.Get(githubAPILatestReleaseURL)
	if err != nil {
		return "", fmt.Errorf("failed to call GitHub API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get latest release from GitHub API: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var releaseInfo struct {
		TagName string `json:"tag_name"`
	}
	err = json.NewDecoder(resp.Body).Decode(&releaseInfo)
	if err != nil {
		return "", fmt.Errorf("failed to decode GitHub API response: %w", err)
	}

	if releaseInfo.TagName == "" {
		return "", errors.New("GitHub API response did not contain a tag_name")
	}

	return releaseInfo.TagName, nil
}

// buildDownloadURLFromTag constructs the download URL based on OS, Arch, and version tag.
func (db *EmbeddedDB) buildDownloadURLFromTag(versionTag string) error {
	os := runtime.GOOS     // "linux", "darwin"
	arch := runtime.GOARCH // "amd64", "arm64"

	var packageFormat string
	var machineType string = arch // Default to Go's arch name

	switch os {
	case "darwin":
		packageFormat = "zip"
		// From Python code: machine_type = "all" for Darwin zip. Seems specific? Let's verify Weaviate releases.
		// Checking https://github.com/weaviate/weaviate/releases/tag/v1.26.6
		// They provide darwin-amd64 and darwin-arm64 zips. Let's use the actual arch.
		// machineType = "all" // This might be outdated or specific to older versions/python script logic
	case "linux":
		packageFormat = "tar.gz"
		// Go arch names amd64/arm64 match Weaviate release names
	default:
		return fmt.Errorf("unsupported operating system for building download URL: %s", os)
	}

	// Weaviate uses 'amd64' and 'arm64' in filenames, matching runtime.GOARCH mostly.
	// Add mappings here if needed in the future (e.g., runtime.GOARCH "x86" -> "amd64" if that was a thing).

	// Example: https://github.com/weaviate/weaviate/releases/download/v1.26.6/weaviate-v1.26.6-linux-amd64.tar.gz
	// Example: https://github.com/weaviate/weaviate/releases/download/v1.26.6/weaviate-v1.26.6-darwin-arm64.zip
	db.downloadURL = fmt.Sprintf(
		"%s%s/weaviate-%s-%s-%s.%s",
		githubReleaseDownloadURL,
		versionTag,
		versionTag,
		os,
		machineType,
		packageFormat,
	)
	db.logger.Infof("Constructed download URL: %s", db.downloadURL)
	return nil
}

// ensureBinaryExists checks if the binary exists and downloads/extracts it if not.
func (db *EmbeddedDB) ensureBinaryExists() error {
	_, err := os.Stat(db.binaryPath)
	if err == nil {
		db.logger.Infof("Weaviate binary already exists: %s", db.binaryPath)
		// Optional: Add a checksum verification here if needed
		return nil // Binary exists
	}

	if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check for existing binary %s: %w", db.binaryPath, err)
	}

	// Binary does not exist, proceed with download and extraction
	db.logger.Infof("Weaviate binary not found at %s. Downloading from %s...", db.binaryPath, db.downloadURL)

	// --- Download ---
	resp, err := http.Get(db.downloadURL)
	if err != nil {
		return fmt.Errorf("failed to start download from %s: %w", db.downloadURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: status code %d from %s", resp.StatusCode, db.downloadURL)
	}

	// Create a temporary file for the download
	tmpFile, err := os.CreateTemp(db.options.BinaryPath, "weaviate-download-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary download file: %w", err)
	}
	tmpFilePath := tmpFile.Name()
	// Ensure temp file is cleaned up on error
	defer func() {
		if err != nil { // Only remove if there was an error during download/extract
			db.logger.Infof("Cleaning up temporary download file: %s", tmpFilePath)
			_ = os.Remove(tmpFilePath)
		}
	}()

	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		_ = tmpFile.Close() // Close before attempting removal
		return fmt.Errorf("failed to write download to temporary file %s: %w", tmpFilePath, err)
	}
	err = tmpFile.Close() // Close after successful write
	if err != nil {
		return fmt.Errorf("failed to close temporary file %s: %w", tmpFilePath, err)
	}

	db.logger.Infof("Download complete. Extracting binary from %s...", tmpFilePath)

	// --- Extract ---
	// We need to extract only the 'weaviate' executable and place it at db.binaryPath
	targetExecutablePath := db.binaryPath // This is the final destination path calculated earlier

	if strings.HasSuffix(db.downloadURL, ".tar.gz") {
		err = extractTarGz(tmpFilePath, targetExecutablePath, db.options.BinaryPath) // Pass BinaryPath for temp extraction if needed
	} else if strings.HasSuffix(db.downloadURL, ".zip") {
		err = extractZip(tmpFilePath, targetExecutablePath, db.options.BinaryPath)
	} else {
		err = fmt.Errorf("unsupported archive format: %s", db.downloadURL)
	}

	// Clean up the downloaded archive file regardless of extraction success/failure
	db.logger.Infof("Cleaning up downloaded archive: %s", tmpFilePath)
	_ = os.Remove(tmpFilePath)

	if err != nil {
		// If extraction failed, attempt to remove potentially incomplete target binary
		_ = os.Remove(targetExecutablePath)
		return fmt.Errorf("failed to extract binary: %w", err)
	}

	db.logger.Infof("Binary extracted successfully to %s", targetExecutablePath)

	// --- Set Executable Permissions ---
	err = os.Chmod(targetExecutablePath, 0755) // Read/Write/Execute for user, Read/Execute for group/others
	if err != nil {
		// Attempt cleanup if chmod fails
		_ = os.Remove(targetExecutablePath)
		return fmt.Errorf("failed to set executable permissions on %s: %w", targetExecutablePath, err)
	}
	db.logger.Infof("Executable permissions set on %s", targetExecutablePath)

	return nil
}

// extractTarGz finds the 'weaviate' executable within a .tar.gz file and extracts it to targetPath.
// tempExtractDir is used if the archive contains the binary within a subdirectory.
func extractTarGz(gzipPath, targetPath, tempExtractDir string) error {
	file, err := os.Open(gzipPath)
	if err != nil {
		return fmt.Errorf("failed to open archive %s: %w", gzipPath, err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader for %s: %w", gzipPath, err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	var found bool

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return fmt.Errorf("error reading tar header: %w", err)
		}

		// We are looking for a file named exactly "weaviate" or possibly "path/to/weaviate"
		// The Python version seems to expect it directly at the root of the tar.
		// Let's handle both cases: directly at root or inside one directory.
		baseName := filepath.Base(header.Name)

		if header.Typeflag == tar.TypeReg && baseName == weaviateExecutableName {
			// Found the executable file
			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create target file %s: %w", targetPath, err)
			}

			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close() // Close file before returning error
				return fmt.Errorf("failed to copy binary content to %s: %w", targetPath, err)
			}
			outFile.Close() // Close file successfully
			found = true
			break // Found what we needed
		}
	}

	if !found {
		return fmt.Errorf("executable '%s' not found within the archive %s", weaviateExecutableName, gzipPath)
	}

	return nil
}

// extractZip finds the 'weaviate' executable within a .zip file and extracts it to targetPath.
// tempExtractDir is used if needed but usually zip extraction handles paths directly.
func extractZip(zipPath, targetPath, tempExtractDir string) error {
	zipReader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip archive %s: %w", zipPath, err)
	}
	defer zipReader.Close()

	var found bool
	for _, file := range zipReader.File {
		// Check if the base name of the file in the archive is "weaviate"
		baseName := filepath.Base(file.Name)

		if baseName == weaviateExecutableName && !file.FileInfo().IsDir() {
			// Found the executable file
			srcFile, err := file.Open()
			if err != nil {
				return fmt.Errorf("failed to open '%s' within zip: %w", file.Name, err)
			}
			defer srcFile.Close()

			// Create the target file with permissions from the zip entry
			// Use os.O_TRUNC to overwrite if it somehow exists
			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.Mode())
			if err != nil {
				return fmt.Errorf("failed to create target file %s: %w", targetPath, err)
			}
			defer outFile.Close() // Ensure closed even on copy error

			if _, err := io.Copy(outFile, srcFile); err != nil {
				// outFile.Close() is handled by defer
				return fmt.Errorf("failed to copy content from zip entry '%s' to %s: %w", file.Name, targetPath, err)
			}

			found = true
			break // Found the executable
		}
	}

	if !found {
		return fmt.Errorf("executable '%s' not found within the zip archive %s", weaviateExecutableName, zipPath)
	}

	return nil
}

// waitForListening polls the HTTP and gRPC ports until they are available or timeout occurs.
func (db *EmbeddedDB) waitForListening(timeout time.Duration) bool {
	startTime := time.Now()
	for {
		httpListening, grpcListening := db.checkListening()
		if httpListening && grpcListening {
			return true // Both ports are ready
		}

		if time.Since(startTime) > timeout {
			db.logger.Infof("Timeout waiting for ports. HTTP Listening: %v, gRPC Listening: %v", httpListening, grpcListening)
			return false // Timeout reached
		}

		// Check if the process exited prematurely
		db.mu.Lock()
		procStillRunning := db.process != nil
		db.mu.Unlock()
		if !procStillRunning {
			db.logger.Info("Process exited unexpectedly while waiting for ports.")
			return false
		}

		time.Sleep(checkInterval)
	}
}

// checkListening performs the actual network check for both ports.
func (db *EmbeddedDB) checkListening() (http bool, grpc bool) {
	// Check HTTP port
	connHTTP, errHTTP := net.DialTimeout("tcp", net.JoinHostPort(db.options.Hostname, strconv.Itoa(db.options.Port)), checkInterval)
	if errHTTP == nil && connHTTP != nil {
		http = true
		_ = connHTTP.Close()
	}

	// Check gRPC port
	connGRPC, errGRPC := net.DialTimeout("tcp", net.JoinHostPort(db.options.Hostname, strconv.Itoa(db.options.GRPCPort)), checkInterval)
	if errGRPC == nil && connGRPC != nil {
		grpc = true
		_ = connGRPC.Close()
	}

	return http, grpc
}

// checkPortFree returns an error if the port is NOT free.
func checkPortFree(host string, port int) error {
	address := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", address, 50*time.Millisecond) // Short timeout
	if err == nil && conn != nil {
		// Connection succeeded, port is likely in use
		_ = conn.Close()
		return fmt.Errorf("port %d is already in use", port)
	}
	// If Dial fails (e.g., connection refused), the port is likely free or host is wrong
	// We assume for 127.0.0.1 the host is correct, so failure means free.
	return nil // Port seems free
}

// getRandomPort finds an available TCP port.
func getRandomPort() int {
	listener, err := net.Listen("tcp", ":0") // ":0" means assign a random available port
	if err != nil {
		// Fallback: Return a high-numbered port as a guess. Extremely unlikely to be free.
		// A better approach might be to panic or return an error.
		log.Printf("Warning: Could not get random port: %v. Using fallback.", err)
		return 0 // Or some other indicator of failure if the caller can handle it
	}
	defer listener.Close()
	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port
}

// monitorProcessExit waits for the process to exit and logs it, clearing the internal state.
func (db *EmbeddedDB) monitorProcessExit() {
	db.mu.Lock()
	if db.process == nil {
		db.mu.Unlock()
		return // Process already gone or never started
	}
	proc := db.process // Capture the process pointer while locked
	pid := proc.Pid
	db.mu.Unlock()

	state, err := proc.Wait() // This blocks until the process exits

	db.mu.Lock()
	defer db.mu.Unlock()

	// Check if the process state we are clearing matches the one we waited for.
	// This avoids clearing the state if Stop() was called followed quickly by a Start().
	if db.process != nil && db.process.Pid == pid {
		if err != nil {
			db.logger.Infof("Error waiting for process PID %d to exit: %v", pid, err)
		} else if state != nil {
			db.logger.Infof("Weaviate process PID %d exited unexpectedly with state: %s", pid, state.String())
		} else {
			db.logger.Infof("Weaviate process PID %d exited unexpectedly.", pid)
		}
		db.process = nil // Mark as no longer running
	} else {
		// Process already cleared or replaced, likely due to Stop() or another Start()
		db.logger.Infof("Process PID %d exited, but internal state was already updated.", pid)
	}
}

// Added Close function as an alias for Stop for more idiomatic resource management
func (db *EmbeddedDB) Close() error {
	return db.Stop()
}

// --- Optional: Graceful Shutdown on Signal ---

// StartAndWatch is a convenience function that starts the DB and sets up signal handling
// to automatically stop the DB on SIGINT or SIGTERM. It blocks until a signal is received
// or an error occurs during startup.
func (db *EmbeddedDB) StartAndWatch() error {
	err := db.Start()
	if err != nil {
		return fmt.Errorf("failed to start embedded Weaviate: %w", err)
	}

	// Set up channel to listen for OS signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	db.logger.Info("Embedded Weaviate running. Press Ctrl+C to stop.")

	// Block until a signal is received
	sig := <-sigs
	db.logger.Infof("Received signal: %s. Shutting down Weaviate...", sig)

	// Attempt to stop the database
	stopErr := db.Stop()
	if stopErr != nil {
		db.logger.Infof("Error stopping Weaviate: %v", stopErr)
		return stopErr // Return the error from stopping
	}

	db.logger.Info("Weaviate stopped gracefully.")
	return nil
}
