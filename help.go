// Copyright 2013 Joe Walnes and the websocketd team.
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	help = `
{{binary}} ({{version}})

{{binary}} is a command line tool that will allow any executable program
that accepts input on stdin and produces output on stdout to be turned into
a WebSocket server.

Usage:

  Export a single executable program a WebSocket server:
    {{binary}} [options] COMMAND [command args]

  Or, export an entire directory of executables as WebSocket endpoints:
    {{binary}} [options] --dir=SOMEDIR

Options:

  --port=PORT                    HTTP port to listen on.

  --address=ADDRESS              Address to bind to (multiple options allowed)
                                 Use square brackets to specify IPv6 address.
                                 Default: "" (all)

  --sameorigin={true,false}      Restrict (HTTP 403) protocol upgrades if the
                                 Origin header does not match to requested HTTP
                                 Host. Default: false.

  --origin=host[:port][,host[:port]...]
                                 Restrict (HTTP 403) protocol upgrades if the
                                 Origin header does not match to one of the host
                                 and port combinations listed. If the port is not
                                 specified, any port number will match.
                                 Default: "" (allow any origin)

  --passenv VAR[,VAR...]         Lists environment variables allowed to be
                                 passed to executed scripts. Does not work for
                                 Windows since all the variables are kept there.

  --binary={true,false}          Switches communication to binary, process reads
                                 send to browser as blobs and all reads from the
                                 browser are immediately flushed to the process.
                                 Default: false

  --reverselookup={true,false}   Perform DNS reverse lookups on remote clients.
                                 Default: false

  --maxforks=N                   Limit number of processes that websocketd is
                                 able to execute with WS. When maxforks reached
                                 the server will be rejecting requests that
                                 require executing another process (unlimited
                                 when 0 or negative).
                                 Default: 0

  --closems=milliseconds         Specifies additional time process needs to gracefully
                                 finish before websocketd will send termination signals
                                 to it. Default: 0 (signals sent after 100ms, 250ms,
                                 and 500ms of waiting)

  --header="..."                 Set custom HTTP header to each answer. For
                                 example: --header="Server: someserver/0.0.1"

  --help                         Print help and exit.

  --version                      Print version and exit.

  --license                      Print license and exit.

  --loglevel=LEVEL               Log level to use (default access).
                                 From most to least verbose:
                                 debug, trace, access, info, error, fatal

Full documentation at http://websocketd.com/

Copyright 2013 Joe Walnes and the websocketd team. All rights reserved.
BSD license: Run '{{binary}} --license' for details.
`
	short = `
Usage:

  Export a single executable program a WebSocket server:
    {{binary}} [options] COMMAND [command args]

  Or, export an entire directory of executables as WebSocket endpoints:
    {{binary}} [options] --dir=SOMEDIR

  Or, show extended help message using:
    {{binary}} --help
`
)

func get_help_message(content string) string {
	msg := strings.Trim(content, " \n")
	msg = strings.Replace(msg, "{{binary}}", HelpProcessName(), -1)
	return strings.Replace(msg, "{{version}}", Version(), -1)
}

func HelpProcessName() string {
	binary := os.Args[0]
	if strings.Contains(binary, "/go-build") { // this was run using "go run", let's use something appropriate
		binary = "websocketd"
	} else {
		binary = filepath.Base(binary)
	}
	return binary
}

func PrintHelp() {
	fmt.Fprintf(os.Stderr, "%s\n", get_help_message(help))
}

func ShortHelp() {
	// Shown after some error
	fmt.Fprintf(os.Stderr, "\n%s\n", get_help_message(short))
}
