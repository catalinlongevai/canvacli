package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
)

type CallbackServer struct {
	listener net.Listener
	server   *http.Server
	codeCh   chan string
	errCh    chan error
	state    string
	port     int
}

// StartCallbackServer binds to the first free port in `ports` on 127.0.0.1
// and serves /callback. Returns an error if no port is free.
func StartCallbackServer(state string, ports []int) (*CallbackServer, error) {
	var ln net.Listener
	var port int
	var err error
	for _, p := range ports {
		ln, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			port = p
			break
		}
	}
	if ln == nil {
		return nil, fmt.Errorf("no callback port free in %v: %w", ports, err)
	}

	cs := &CallbackServer{
		listener: ln,
		state:    state,
		port:     port,
		codeCh:   make(chan string, 1),
		errCh:    make(chan error, 1),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", cs.handle)
	cs.server = &http.Server{Handler: mux}

	go cs.server.Serve(ln)
	return cs, nil
}

func (cs *CallbackServer) RedirectURI() string {
	return fmt.Sprintf("http://127.0.0.1:%d/callback", cs.port)
}

func (cs *CallbackServer) handle(w http.ResponseWriter, r *http.Request) {
	gotState := r.URL.Query().Get("state")
	if gotState != cs.state {
		http.Error(w, "state mismatch", http.StatusBadRequest)
		select {
		case cs.errCh <- errors.New("oauth state mismatch"):
		default:
		}
		return
	}
	if errStr := r.URL.Query().Get("error"); errStr != "" {
		http.Error(w, errStr, http.StatusBadRequest)
		select {
		case cs.errCh <- fmt.Errorf("oauth error: %s", errStr):
		default:
		}
		return
	}
	code := r.URL.Query().Get("code")
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintln(w, `<html><body><h1>canvacli connected</h1><p>You can close this tab and return to your terminal.</p></body></html>`)
	select {
	case cs.codeCh <- code:
	default:
	}
}

func (cs *CallbackServer) Wait(ctx context.Context) (string, error) {
	select {
	case code := <-cs.codeCh:
		return code, nil
	case err := <-cs.errCh:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (cs *CallbackServer) Close() {
	cs.server.Close()
	cs.listener.Close()
}
