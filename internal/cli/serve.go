package cli

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/DotNaos/moodle-services/internal/api"
	"github.com/spf13/cobra"
)

var serveAddr string
var serveShutdownTimeout time.Duration
var serveRequestTimeout time.Duration
var serveSchool string
var serveUsername string
var servePassword string

type serveEvent struct {
	Type   string `json:"type" yaml:"type"`
	Addr   string `json:"addr,omitempty" yaml:"addr,omitempty"`
	Docs   string `json:"docs,omitempty" yaml:"docs,omitempty"`
	Signal string `json:"signal,omitempty" yaml:"signal,omitempty"`
	Error  string `json:"error,omitempty" yaml:"error,omitempty"`
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the REST API server",
	Long:  "Start a long-running HTTP server that exposes Moodle data as JSON over a REST API.",
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		runtimeLoginOverrides = loginInputOverrides{
			School:   serveSchool,
			Username: serveUsername,
			Password: servePassword,
		}

		if err := emitServeStatus(cmd, serveEvent{Type: "starting", Addr: serveAddr}); err != nil {
			return err
		}
		if strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
			if err := ensureServeSession(); err != nil {
				_ = emitServeStatus(cmd, serveEvent{Type: "fatal", Addr: serveAddr, Error: err.Error()})
				return markErrorEmitted(err)
			}
		}

		router, err := api.NewRouter(api.ServerOptions{
			ClientProvider: func() (api.Client, error) {
				return ensureAPIClient()
			},
			CommandRoutes:  buildAPICommandRoutes(),
			CommandRunner:  runAPICommand,
			LogWriter:      cmd.ErrOrStderr(),
			RequestTimeout: serveRequestTimeout,
		})
		if err != nil {
			_ = emitServeStatus(cmd, serveEvent{Type: "fatal", Addr: serveAddr, Error: err.Error()})
			return markErrorEmitted(err)
		}

		server := &http.Server{
			Addr:              serveAddr,
			Handler:           router,
			ReadHeaderTimeout: 10 * time.Second,
		}

		listener, err := net.Listen("tcp", serveAddr)
		if err != nil {
			_ = emitServeStatus(cmd, serveEvent{Type: "fatal", Addr: serveAddr, Error: err.Error()})
			return markErrorEmitted(err)
		}
		readyAddr := listener.Addr().String()
		if err := emitServeStatus(cmd, serveEvent{Type: "ready", Addr: readyAddr, Docs: serveDocsURL(readyAddr)}); err != nil {
			listener.Close()
			return err
		}

		errCh := make(chan error, 1)
		go func() {
			if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
				errCh <- err
			}
		}()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

		select {
		case err := <-errCh:
			_ = emitServeStatus(cmd, serveEvent{Type: "fatal", Addr: listener.Addr().String(), Error: err.Error()})
			return markErrorEmitted(err)
		case sig := <-sigCh:
			if err := emitServeStatus(cmd, serveEvent{Type: "shutdown", Addr: listener.Addr().String(), Signal: sig.String()}); err != nil {
				return err
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), serveShutdownTimeout)
		defer cancel()
		return server.Shutdown(ctx)
	},
}

func init() {
	markAPIOptional(serveCmd)

	defaultAddr := ":8080"
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		if strings.HasPrefix(port, ":") {
			defaultAddr = port
		} else {
			defaultAddr = ":" + port
		}
	}

	serveCmd.Flags().StringVar(&serveAddr, "addr", defaultAddr, "Address to bind the API server to (e.g. :8080 or 127.0.0.1:8080)")
	serveCmd.Flags().DurationVar(&serveShutdownTimeout, "shutdown-timeout", 10*time.Second, "Grace period for graceful shutdown")
	serveCmd.Flags().DurationVar(&serveRequestTimeout, "request-timeout", 30*time.Minute, "Maximum duration for one API request")
	serveCmd.Flags().StringVar(&serveSchool, "school", "", "School id override used for a fresh login. Only fhgr is currently active; multi-school support is not active")
	serveCmd.Flags().StringVar(&serveUsername, "username", "", "Username/email used for a fresh login before starting the server")
	serveCmd.Flags().StringVar(&servePassword, "password", "", "Password used for a fresh login before starting the server")

	serveCmd.RegisterFlagCompletionFunc("school", completeSchoolIDs)
}

func emitServeStatus(cmd *cobra.Command, event serveEvent) error {
	if isMachineOutput() {
		return writeStreamEvent(cmd.OutOrStdout(), event)
	}
	return renderServeStatus(cmd.OutOrStdout(), event)
}

func renderServeStatus(w io.Writer, event serveEvent) error {
	switch event.Type {
	case "starting":
		_, err := fmt.Fprintf(w, "Starting Moodle API server on %s\n", event.Addr)
		return err
	case "ready":
		if _, err := fmt.Fprintf(w, "Moodle API server ready on %s\n", event.Addr); err != nil {
			return err
		}
		if strings.TrimSpace(event.Docs) != "" {
			_, err := fmt.Fprintf(w, "API docs: %s\n", event.Docs)
			return err
		}
		return nil
	case "shutdown":
		_, err := fmt.Fprintf(w, "Received %s, shutting down...\n", event.Signal)
		return err
	case "fatal":
		_, err := fmt.Fprintf(w, "Server failed: %s\n", event.Error)
		return err
	default:
		return nil
	}
}

func serveDocsURL(addr string) string {
	host, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return ""
	}
	host = strings.Trim(strings.TrimSpace(host), "[]")
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/docs"
}
