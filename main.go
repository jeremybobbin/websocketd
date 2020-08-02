// Copyright 2013 Joe Walnes and the websocketd team.
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/joewalnes/websocketd/libwebsocketd"
)

func logfunc(l *libwebsocketd.LogScope, level libwebsocketd.LogLevel, levelName string, category string, msg string, args ...interface{}) {
	if level < l.MinLevel {
		return
	}
	fullMsg := fmt.Sprintf(msg, args...)

	assocDump := ""
	for index, pair := range l.Associated {
		if index > 0 {
			assocDump += " "
		}
		assocDump += fmt.Sprintf("%s:'%s'", pair.Key, pair.Value)
	}

	l.Mutex.Lock()
	fmt.Printf("%s | %-6s | %-10s | %s | %s\n", libwebsocketd.Timestamp(), levelName, category, assocDump, fullMsg)
	l.Mutex.Unlock()
}

func main() {
	config := parseCommandLine()

	log := libwebsocketd.RootLogScope(config.LogLevel, logfunc)

	if runtime.GOOS != "windows" { // windows relies on env variables to find its libs... e.g. socket stuff
		os.Clearenv() // it's ok to wipe it clean, we already read env variables from passenv into config
	}
	handler := libwebsocketd.NewWebsocketdServer(config.Config, log, config.MaxForks)
	http.Handle("/", handler)

	log.Info("server", "Serving using application   : %s %s", config.CommandName, strings.Join(config.CommandArgs, " "))

	rejects := make(chan error, 1)
	for _, addrSingle := range config.Addr {
		log.Info("server", "Starting WebSocket server   : ws://%s/", addrSingle)
		// ListenAndServe is blocking function. Let's run it in
		// go routine, reporting result to control channel.
		// Since it's blocking it'll never return non-error.

		go func(addr string) {
			rejects <- http.ListenAndServe(addr, nil)
		}(addrSingle)
	}
	err := <-rejects
	if err != nil {
		log.Fatal("server", "Can't start server: %s", err)
		os.Exit(3)
	}
}
