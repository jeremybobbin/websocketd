// Copyright 2013 Joe Walnes and the websocketd team.
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/joewalnes/websocketd/libwebsocketd"
)

type Config struct {
	Addr      []string // TCP addresses to listen on. e.g. ":1234", "1.2.3.4:1234" or "[::1]:1234"
	MaxForks  int      // Number of allowable concurrent forks
	LogLevel  libwebsocketd.LogLevel
	RedirPort int
	*libwebsocketd.Config
}

type Arglist []string

func (al *Arglist) String() string {
	return fmt.Sprintf("%v", []string(*al))
}

func (al *Arglist) Set(value string) error {
	*al = append(*al, value)
	return nil
}

// Borrowed from net/http/cgi
var defaultPassEnv = map[string]string{
	"darwin":  "PATH,DYLD_LIBRARY_PATH",
	"freebsd": "PATH,LD_LIBRARY_PATH",
	"hpux":    "PATH,LD_LIBRARY_PATH,SHLIB_PATH",
	"irix":    "PATH,LD_LIBRARY_PATH,LD_LIBRARYN32_PATH,LD_LIBRARY64_PATH",
	"linux":   "PATH,LD_LIBRARY_PATH",
	"openbsd": "PATH,LD_LIBRARY_PATH",
	"solaris": "PATH,LD_LIBRARY_PATH,LD_LIBRARY_PATH_32,LD_LIBRARY_PATH_64",
	"windows": "PATH,SystemRoot,COMSPEC,PATHEXT,WINDIR",
}

func parseCommandLine() *Config {
	var mainConfig Config
	var config libwebsocketd.Config

	flag.Usage = func() {}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	// If adding new command line options, also update the help text in help.go.
	// The flag library's auto-generate help message isn't pretty enough.

	addrlist := Arglist(make([]string, 0, 1)) // pre-reserve for 1 address
	flag.Var(&addrlist, "address", "Interfaces to bind to (e.g. 127.0.0.1 or [::1]).")

	// server config options
	portFlag := flag.Int("port", 0, "HTTP port to listen on")
	versionFlag := flag.Bool("version", false, "Print version and exit")
	licenseFlag := flag.Bool("license", false, "Print license and exit")
	logLevelFlag := flag.String("loglevel", "access", "Log level, one of: debug, trace, access, info, error, fatal")
	maxForksFlag := flag.Int("maxforks", 0, "Max forks, zero means unlimited")
	closeMsFlag := flag.Uint("closems", 0, "Time to start sending signals (0 never)")
	redirPortFlag := flag.Int("redirport", 0, "HTTP port to redirect to canonical --port address")

	// lib config options
	binaryFlag := flag.Bool("binary", false, "Set websocketd to experimental binary mode (default is line by line)")
	reverseLookupFlag := flag.Bool("reverselookup", false, "Perform reverse DNS lookups on remote clients")
	passEnvFlag := flag.String("passenv", defaultPassEnv[runtime.GOOS], "List of envvars to pass to subprocesses (others will be cleaned out)")
	sameOriginFlag := flag.Bool("sameorigin", false, "Restrict upgrades if origin and host headers differ")
	allowOriginsFlag := flag.String("origin", "", "Restrict upgrades if origin does not match the list")

	headers := Arglist(make([]string, 0))
	flag.Var(&headers, "header", "Custom headers for any response.")

	err := flag.CommandLine.Parse(os.Args[1:])
	if err != nil {
		if err == flag.ErrHelp {
			PrintHelp()
			os.Exit(0)
		} else {
			ShortHelp()
			os.Exit(2)
		}
	}

	port := *portFlag
	if port == 0 {
		port = 80
	}

	if socknum := len(addrlist); socknum != 0 {
		mainConfig.Addr = make([]string, socknum)
		for i, addrSingle := range addrlist {
			mainConfig.Addr[i] = fmt.Sprintf("%s:%d", addrSingle, port)
		}
	} else {
		mainConfig.Addr = []string{fmt.Sprintf(":%d", port)}
	}
	mainConfig.MaxForks = *maxForksFlag
	mainConfig.RedirPort = *redirPortFlag
	mainConfig.LogLevel = libwebsocketd.LevelFromString(*logLevelFlag)
	if mainConfig.LogLevel == libwebsocketd.LogUnknown {
		fmt.Printf("Incorrect loglevel flag '%s'. Use --help to see allowed values.\n", *logLevelFlag)
		ShortHelp()
		os.Exit(1)
	}

	config.Headers = []string(headers)

	config.CloseMs = *closeMsFlag
	config.Binary = *binaryFlag
	config.ReverseLookup = *reverseLookupFlag
	config.StartupTime = time.Now()
	config.ServerSoftware = fmt.Sprintf("websocketd/%s", Version())
	config.HandshakeTimeout = time.Millisecond * 1500 // only default for now

	if len(os.Args) == 1 {
		fmt.Printf("Command line arguments are missing.\n")
		ShortHelp()
		os.Exit(1)
	}

	if *versionFlag {
		fmt.Printf("%s %s\n", HelpProcessName(), Version())
		os.Exit(0)
	}

	if *licenseFlag {
		fmt.Printf("%s %s\n", HelpProcessName(), Version())
		fmt.Printf("%s\n", libwebsocketd.License)
		os.Exit(0)
	}

	// Building config.ParentEnv to avoid calling Environ all the time in the scripts
	// (caller is responsible for wiping environment if desired)
	config.ParentEnv = make([]string, 0)
	newlineCleaner := strings.NewReplacer("\n", " ", "\r", " ")
	for _, key := range strings.Split(*passEnvFlag, ",") {
		if key != "HTTPS" {
			if v := os.Getenv(key); v != "" {
				// inevitably adding flavor of libwebsocketd appendEnv func.
				// it's slightly nicer than in net/http/cgi implementation
				if clean := strings.TrimSpace(newlineCleaner.Replace(v)); clean != "" {
					config.ParentEnv = append(config.ParentEnv, fmt.Sprintf("%s=%s", key, clean))
				}
			}
		}
	}

	if *allowOriginsFlag != "" {
		config.AllowOrigins = strings.Split(*allowOriginsFlag, ",")
	}
	config.SameOrigin = *sameOriginFlag

	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Please specify COMMAND.\n")
		ShortHelp()
		os.Exit(1)
	}

	if path, err := exec.LookPath(args[0]); err == nil {
		config.CommandName = path // This can be command in PATH that we are able to execute
		config.CommandArgs = flag.Args()[1:]
	} else {
		fmt.Fprintf(os.Stderr, "Unable to locate specified COMMAND '%s' in OS path.\n", args[0])
		ShortHelp()
		os.Exit(1)
	}

	mainConfig.Config = &config

	return &mainConfig
}
