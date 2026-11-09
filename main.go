package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"

	"avpnd/internal/avpnd"
)

func defaultBinary() string {
	if runtime.GOOS == "darwin" {
		return "/Applications/AWS VPN Client/AWS VPN Client.app/Contents/MacOS/aws-vpn-client"
	}
	return "aws-vpn-client"
}

func main() {
	profile := flag.String("profile", "", "VPN profile name (required unless -list-profiles)")
	bin := flag.String("bin", defaultBinary(), "Path to AWS VPN Client binary")
	listen := flag.String("listen", "127.0.0.1:35002", "Address to listen on (must not be port 35001, which the AWS client uses for its SAML callback)")
	listProfiles := flag.Bool("list-profiles", false, "List available AWS VPN Client profiles and exit")
	verbose := flag.Bool("v", false, "Log the CLI output of every call, including status polls")
	flag.Parse()

	if *listProfiles {
		cmd := exec.Command(*bin, "list-profiles")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			os.Exit(1)
		}
		return
	}

	if *profile == "" {
		flag.Usage()
		os.Exit(1)
	}

	if err := avpnd.ValidateListen(*listen); err != nil {
		fmt.Fprintln(os.Stderr, "avpnd:", err)
		os.Exit(1)
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	server := avpnd.NewServer(*profile, avpnd.NewOSRunner(*bin, logger), logger)
	if err := server.Serve(*listen); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
