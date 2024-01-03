package replicatr

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/rs/cors"
)

func (rl *Relay) Router() *http.ServeMux {
	return rl.serveMux
}

// Start creates an http server and starts listening on given host and port.
func (rl *Relay) Start(host string, port int, started ...chan bool) (e error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	var ln net.Listener
	if ln, e = net.Listen("tcp", addr); rl.E.Chk(e) {
		return
	}
	rl.Addr = ln.Addr().String()
	rl.httpServer = &http.Server{
		Handler:      cors.Default().Handler(rl),
		Addr:         addr,
		WriteTimeout: 2 * time.Second,
		ReadTimeout:  2 * time.Second,
		IdleTimeout:  30 * time.Second,
	}
	// notify caller that we're starting
	for _, s := range started {
		close(s)
	}
	if e = rl.httpServer.Serve(ln); errors.Is(e, http.ErrServerClosed) {
		return nil
	} else if rl.Log.E.Chk(e) {
		return
	}
	return
}

// Shutdown sends a websocket close control message to all connected clients.
func (rl *Relay) Shutdown(ctx Ctx) {
	rl.Log.E.Chk(rl.httpServer.Shutdown(ctx))
	rl.clients.Range(func(conn *websocket.Conn, _ struct{}) bool {
		rl.E.Chk(conn.WriteControl(websocket.CloseMessage, nil, time.Now().Add(time.Second)))
		rl.E.Chk(conn.Close())
		rl.clients.Delete(conn)
		return true
	})
}
