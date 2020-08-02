package libwebsocketd

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// WebsocketdHandler is a single request information and processing structure, it handles WS requests out of all that daemon can handle
type WebsocketdHandler struct {
	server *WebsocketdServer

	Id string
	*RemoteInfo
	*URLInfo // TODO: I cannot find where it's used except in one single place as URLInfo.FilePath
	Env      []string

	command string
}

// NewWebsocketdHandler constructs the struct and parses all required things in it...
func NewWebsocketdHandler(s *WebsocketdServer, req *http.Request, log *LogScope) (wsh *WebsocketdHandler, err error) {
	wsh = &WebsocketdHandler{server: s, Id: generateId()}
	log.Associate("id", wsh.Id)

	wsh.RemoteInfo, err = GetRemoteInfo(req.RemoteAddr, s.Config.ReverseLookup)
	if err != nil {
		log.Error("session", "Could not understand remote address '%s': %s", req.RemoteAddr, err)
		return nil, err
	}
	log.Associate("remote", wsh.RemoteInfo.Host)

	wsh.URLInfo, err = GetURLInfo(req.URL.Path, s.Config)
	if err != nil {
		log.Access("session", "NOT FOUND: %s", err)
		return nil, err
	}

	wsh.command = s.Config.CommandName
	log.Associate("command", wsh.command)

	wsh.Env = createEnv(wsh, req, log)

	return wsh, nil
}

func (wsh *WebsocketdHandler) accept(ws *websocket.Conn, log *LogScope) {
	defer ws.Close()

	log.Access("session", "CONNECT")
	defer log.Access("session", "DISCONNECT")

	launched, err := launchCmd(wsh.command, wsh.server.Config.CommandArgs, wsh.Env)
	if err != nil {
		log.Error("process", "Could not launch process %s %s (%s)", wsh.command, strings.Join(wsh.server.Config.CommandArgs, " "), err)
		return
	}

	log.Associate("pid", strconv.Itoa(launched.cmd.Process.Pid))

	binary := wsh.server.Config.Binary
	process := NewProcessEndpoint(launched, binary, log)
	if cms := wsh.server.Config.CloseMs; cms != 0 {
		process.closetime += time.Duration(cms) * time.Millisecond
	}
	wsEndpoint := NewWebSocketEndpoint(ws, binary, log)

	PipeEndpoints(process, wsEndpoint)
}

// RemoteInfo holds information about remote http client
type RemoteInfo struct {
	Addr, Host, Port string
}

// GetRemoteInfo creates RemoteInfo structure and fills its fields appropriately
func GetRemoteInfo(remote string, doLookup bool) (*RemoteInfo, error) {
	addr, port, err := net.SplitHostPort(remote)
	if err != nil {
		return nil, err
	}

	var host string
	if doLookup {
		hosts, err := net.LookupAddr(addr)
		if err != nil || len(hosts) == 0 {
			host = addr
		} else {
			host = hosts[0]
		}
	} else {
		host = addr
	}

	return &RemoteInfo{Addr: addr, Host: host, Port: port}, nil
}

// URLInfo - structure carrying information about current request and it's mapping to filesystem
type URLInfo struct {
	PathInfo   string
	FilePath   string
}

// GetURLInfo is a function that parses path and provides URL info according to libwebsocketd.Config fields
func GetURLInfo(path string, config *Config) (*URLInfo, error) {
	return &URLInfo{path, ""}, nil
}

func generateId() string {
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}
