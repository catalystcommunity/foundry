package gui

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	stackcmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/stack"
	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/manager"
	foundryssh "github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/catalystcommunity/foundry/v1/internal/webui"
	"github.com/urfave/cli/v3"
)

// Command starts the optional local Foundry web interface.
var Command = &cli.Command{
	Name:  "gui",
	Usage: "Open the optional Foundry web interface",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "listen", Usage: "loopback listen address", Value: "127.0.0.1:0"},
		&cli.BoolFlag{Name: "no-open", Usage: "print the URL without opening a browser"},
		&cli.BoolFlag{Name: "manager", Usage: "forward the installed external manager over SSH"},
	},
	Action: runGUI,
}

// ServeCommand runs the persistent manager HTTP server inside its container.
var ServeCommand = &cli.Command{
	Name:   "serve",
	Usage:  "Run the external Foundry manager server",
	Hidden: true,
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "listen", Usage: "listen address", Value: "0.0.0.0:8080"},
		&cli.StringFlag{Name: "token-file", Usage: "path to the manager access token file", Required: true},
	},
	Action: runServe,
}

func runGUI(ctx context.Context, cmd *cli.Command) error {
	configPath := resolveConfigPath(cmd.String("config"))
	if cmd.Bool("manager") {
		return runManagerProxy(ctx, cmd, configPath)
	}
	auth := webui.NewAuthStore()
	token, err := auth.NewToken("local-cli", 15*time.Minute)
	if err != nil {
		return err
	}
	server, err := newServer(configPath, auth)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", cmd.String("listen"))
	if err != nil {
		return fmt.Errorf("listen for web UI: %w", err)
	}
	defer listener.Close()
	if !listener.Addr().(*net.TCPAddr).IP.IsLoopback() {
		return fmt.Errorf("foundry gui must listen on a loopback address; use the external manager for network access")
	}

	url := fmt.Sprintf("http://%s/#token=%s", listener.Addr().String(), token)
	fmt.Printf("Foundry web interface: %s\n", url)
	if !cmd.Bool("no-open") {
		if err := openBrowser(url); err != nil {
			fmt.Printf("Could not open a browser: %v\n", err)
		}
	}
	return serve(ctx, listener, server.Handler())
}

func runManagerProxy(ctx context.Context, cmd *cli.Command, configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if cfg.Management == nil {
		return fmt.Errorf("this configuration does not define an external manager")
	}
	var configuredManager *config.Host
	for _, configuredHost := range cfg.Hosts {
		if configuredHost.Hostname == cfg.Management.Host {
			configuredManager = configuredHost
			break
		}
	}
	if configuredManager == nil {
		return fmt.Errorf("management host %q is not configured", cfg.Management.Host)
	}
	keysDir, err := config.GetKeysDir()
	if err != nil {
		return err
	}
	storage, err := foundryssh.NewFilesystemKeyStorage(keysDir)
	if err != nil {
		return err
	}
	key, err := storage.Load(configuredManager.Hostname)
	if err != nil {
		return fmt.Errorf("load management host SSH key: %w", err)
	}
	authMethod, err := key.AuthMethod()
	if err != nil {
		return fmt.Errorf("use management host SSH key: %w", err)
	}
	connection, err := foundryssh.Connect(&foundryssh.ConnectionOptions{
		Host: configuredManager.Address, Port: configuredManager.Port, User: configuredManager.User,
		AuthMethod: authMethod, Timeout: 30,
	})
	if err != nil {
		return fmt.Errorf("connect to external manager: %w", err)
	}
	defer connection.Close()
	token, err := manager.ReadToken(&connectionExecutor{connection: connection}, cfg.Management.DataPath)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", cmd.String("listen"))
	if err != nil {
		return fmt.Errorf("listen for manager proxy: %w", err)
	}
	defer listener.Close()
	if !listener.Addr().(*net.TCPAddr).IP.IsLoopback() {
		return fmt.Errorf("the manager proxy must listen on a loopback address")
	}
	url := fmt.Sprintf("http://%s/#token=%s", listener.Addr().String(), token)
	fmt.Printf("Foundry manager proxy: %s\n", url)
	if !cmd.Bool("no-open") {
		if err := openBrowser(url); err != nil {
			fmt.Printf("Could not open a browser: %v\n", err)
		}
	}
	return forwardManager(ctx, listener, connection, int(cfg.Management.Port))
}

type connectionExecutor struct{ connection *foundryssh.Connection }

func (e *connectionExecutor) Execute(command string) (string, error) {
	result, err := e.connection.Exec(command)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return result.Stdout, fmt.Errorf("remote command failed: %s", result.Stderr)
	}
	return result.Stdout, nil
}

func forwardManager(ctx context.Context, listener net.Listener, connection *foundryssh.Connection, port int) error {
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	for {
		local, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept manager proxy connection: %w", err)
		}
		go proxyConnection(local, connection, port)
	}
}

func proxyConnection(local net.Conn, connection *foundryssh.Connection, port int) {
	defer local.Close()
	remote, err := connection.Client().Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return
	}
	defer remote.Close()
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(remote, local); done <- struct{}{} }()
	go func() { _, _ = io.Copy(local, remote); done <- struct{}{} }()
	<-done
}

func runServe(ctx context.Context, cmd *cli.Command) error {
	token, err := readTokenFile(cmd.String("token-file"))
	if err != nil {
		return err
	}
	auth := webui.NewAuthStore()
	auth.AddToken("manager-admin", token, 365*24*time.Hour)
	server, err := newServer(resolveConfigPath(cmd.String("config")), auth)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", cmd.String("listen"))
	if err != nil {
		return fmt.Errorf("listen for manager: %w", err)
	}
	defer listener.Close()
	fmt.Printf("Foundry manager listening on %s\n", listener.Addr())
	return serve(ctx, listener, server.Handler())
}

func newServer(configPath string, auth *webui.AuthStore) (*webui.Server, error) {
	return webui.New(webui.Options{
		ConfigPath: configPath,
		Auth:       auth,
		Apply: func(ctx context.Context, path string) error {
			return stackcmd.RunInstall(ctx, stackcmd.InstallOptions{
				ConfigPath: path,
				Yes:        true,
			})
		},
	})
}

func resolveConfigPath(requested string) string {
	if path, err := config.FindConfig(requested); err == nil {
		return path
	}
	if requested != "" && filepath.IsAbs(requested) {
		return requested
	}
	return config.DefaultConfigPath()
}

func readTokenFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("inspect token file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("token file must be a regular file")
	}
	if info.Mode().Perm()&0077 != 0 {
		return "", fmt.Errorf("token file permissions must not grant group or other access")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read token file: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if len(token) < 32 || strings.ContainsAny(token, "\r\n\t ") {
		return "", fmt.Errorf("token file must contain one high-entropy token")
	}
	return token, nil
}

func serve(ctx context.Context, listener net.Listener, handler http.Handler) error {
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		return nil
	case err := <-done:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func openBrowser(url string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", url)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		command = exec.Command("xdg-open", url)
	}
	return command.Start()
}
