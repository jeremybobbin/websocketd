// Copyright 2013 Joe Walnes and the websocketd team.
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libwebsocketd

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"regexp"
	"strings"

	"github.com/gorilla/websocket"
)

var ForkNotAllowedError = errors.New("too many forks active")

// WebsocketdServer presents http.Handler interface for requests libwebsocketd is handling.
type WebsocketdServer struct {
	Config *Config
	Log    *LogScope
	forks  chan byte
}

// NewWebsocketdServer creates WebsocketdServer struct with pre-determined config, logscope and maxforks limit
func NewWebsocketdServer(config *Config, log *LogScope, maxforks int) *WebsocketdServer {
	mux := &WebsocketdServer{
		Config: config,
		Log:    log,
	}
	if maxforks > 0 {
		mux.forks = make(chan byte, maxforks)
	}
	return mux
}

func splitMimeHeader(s string) (string, string) {
	p := strings.IndexByte(s, ':')
	if p < 0 {
		return s, ""
	}
	key := textproto.CanonicalMIMEHeaderKey(s[:p])

	for p = p + 1; p < len(s); p++ {
		if s[p] != ' ' {
			break
		}
	}
	return key, s[p:]
}

func pushHeaders(h http.Header, hdrs []string) {
	for _, hstr := range hdrs {
		h.Add(splitMimeHeader(hstr))
	}
}

// ServeHTTP muxes between WebSocket handler or 404.
func (h *WebsocketdServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	log := h.Log.NewLevel(h.Log.LogFunc)
	log.Associate("url", "http://" + req.Host + req.RequestURI)

	if h.Config.CommandName != "" {
		hdrs := req.Header
		upgradeRe := regexp.MustCompile(`(?i)(^|[,\s])Upgrade($|[,\s])`)
		// WebSocket, limited to size of h.forks
		if strings.ToLower(hdrs.Get("Upgrade")) == "websocket" && upgradeRe.MatchString(hdrs.Get("Connection")) {
			if h.noteForkCreated() == nil {
				defer h.noteForkCompled()

				// start figuring out if we even need to upgrade
				handler, err := NewWebsocketdHandler(h, req, log)
				if err != nil {
					log.Access("session", "INTERNAL ERROR: %s", err)
					http.Error(w, "500 Internal Server Error", 500)
					return
				}

				var headers = http.Header(make(map[string][]string))
				upgrader := &websocket.Upgrader{
					HandshakeTimeout: h.Config.HandshakeTimeout,
					CheckOrigin: func(r *http.Request) bool {
						// backporting previous checkorigin for use in gorilla/websocket for now
						err := checkOrigin(req, h.Config, log)
						return err == nil
					},
				}
				conn, err := upgrader.Upgrade(w, req, headers)
				if err != nil {
					log.Access("session", "Unable to Upgrade: %s", err)
					http.Error(w, "500 Internal Error", 500)
					return
				}

				// old func was used in x/net/websocket style, we reuse it here for gorilla/websocket
				handler.accept(conn, log)
				return

			} else {
				log.Error("http", "Max of possible forks already active, upgrade rejected")
				http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
			}
			return
		}
	}

	// 404
	log.Access("http", "NOT FOUND")
	http.NotFound(w, req)
}

var canonicalHostname string

func (h *WebsocketdServer) noteForkCreated() error {
	// note that forks can be nil since the construct could've been created by
	// someone who is not using NewWebsocketdServer
	if h.forks != nil {
		select {
		case h.forks <- 1:
			return nil
		default:
			return ForkNotAllowedError
		}
	} else {
		return nil
	}
}

func (h *WebsocketdServer) noteForkCompled() {
	if h.forks != nil { // see comment in noteForkCreated
		select {
		case <-h.forks:
			return
		default:
			// This could only happen if the completion handler called more times than creation handler above
			// Code should be audited to not allow this to happen, it's desired to have test that would
			// make sure this is impossible but it is not exist yet.
			panic("Cannot deplet number of allowed forks, something is not right in code!")
		}
	}
}

func checkOrigin(req *http.Request, config *Config, log *LogScope) (err error) {
	// CONVERT GORILLA:
	// this is origin checking function, it's called from wshandshake which is from ServeHTTP main handler
	// should be trivial to reuse in gorilla's upgrader.CheckOrigin function.
	// Only difference is to parse request and fetching passed Origin header out of it instead of using
	// pre-parsed wsconf.Origin

	// check for origin to be correct in future
	// handshaker triggers answering with 403 if error was returned
	// We keep behavior of original handshaker that populates this field
	origin := req.Header.Get("Origin")
	if origin == "" || (origin == "null" && config.AllowOrigins == nil) {
		// we don't want to trust string "null" if there is any
		// enforcements are active
		origin = "file:"
	}

	originParsed, err := url.ParseRequestURI(origin)
	if err != nil {
		log.Access("session", "Origin parsing error: %s", err)
		return err
	}

	log.Associate("origin", originParsed.String())

	// If some origin restrictions are present:
	if config.SameOrigin || config.AllowOrigins != nil {
		originServer, originPort, err := tellHostPort(originParsed.Host)
		if err != nil {
			log.Access("session", "Origin hostname parsing error: %s", err)
			return err
		}
		if config.SameOrigin {
			localServer, localPort, err := tellHostPort(req.Host)
			if err != nil {
				log.Access("session", "Request hostname parsing error: %s", err)
				return err
			}
			if originServer != localServer || originPort != localPort {
				log.Access("session", "Same origin policy mismatch")
				return fmt.Errorf("same origin policy violated")
			}
		}
		if config.AllowOrigins != nil {
			matchFound := false
			for _, allowed := range config.AllowOrigins {
				if pos := strings.Index(allowed, "://"); pos > 0 {
					// allowed schema has to match
					allowedURL, err := url.Parse(allowed)
					if err != nil {
						continue // pass bad URLs in origin list
					}
					if allowedURL.Scheme != originParsed.Scheme {
						continue // mismatch
					}
					allowed = allowed[pos+3:]
				}
				allowServer, allowPort, err := tellHostPort(allowed)
				if err != nil {
					continue // unparseable
				}
				if allowPort == "80" && allowed[len(allowed)-3:] != ":80" {
					// any port is allowed, host names need to match
					matchFound = allowServer == originServer
				} else {
					// exact match of host names and ports
					matchFound = allowServer == originServer && allowPort == originPort
				}
				if matchFound {
					break
				}
			}
			if !matchFound {
				log.Access("session", "Origin is not listed in allowed list")
				return fmt.Errorf("origin list matches were not found")
			}
		}
	}
	return nil
}

func tellHostPort(host string) (server, port string, err error) {
	server, port, err = net.SplitHostPort(host)
	if err != nil {
		if addrerr, ok := err.(*net.AddrError); ok && strings.Contains(addrerr.Err, "missing port") {
			server = host
			port = "80"
			err = nil
		}
	}
	return server, port, err
}
