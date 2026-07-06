package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"os"
	"strings"

	"netatlas/internal/arpscout"
)

func main() {
	if len(os.Args) < 2 || wantsHelp(os.Args[1:]) {
		printHelp(os.Stdout)
		return
	}

	switch os.Args[1] {
	case "info":
		if err := printJSON(os.Stdout, arpscout.Info()); err != nil {
			log.Fatal(err)
		}
	case "identity":
		if err := runIdentity(os.Stdout, os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "passive":
		if err := runPassive(os.Stdout, os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatalf("unknown command %q", os.Args[1])
	}
}

func wantsHelp(args []string) bool {
	return len(args) > 0 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help")
}

func runIdentity(w io.Writer, args []string) error {
	flags := flag.NewFlagSet("identity", flag.ContinueOnError)
	flags.SetOutput(w)
	configPath := flags.String("config", "config.ini", "path to NetAtlas INI config")
	ifaces := flags.String("iface", "", "comma-separated interfaces to include in identity")
	if err := flags.Parse(args); err != nil {
		return err
	}

	cfg, err := arpscout.LoadIdentityConfig(*configPath)
	if err != nil {
		return err
	}
	if values := splitList(*ifaces); len(values) > 0 {
		cfg.Interfaces = values
	}
	identity, err := arpscout.BuildIdentity(cfg)
	if err != nil {
		return err
	}
	return printJSON(w, identity)
}

func runPassive(w io.Writer, args []string) error {
	flags := flag.NewFlagSet("passive", flag.ContinueOnError)
	flags.SetOutput(w)
	ifaces := flags.String("iface", "", "comma-separated interfaces to read")
	includeIncomplete := flags.Bool("include-incomplete", false, "include INCOMPLETE entries")
	if err := flags.Parse(args); err != nil {
		return err
	}

	observations, err := arpscout.ReadPassive(arpscout.PassiveOptions{
		Interfaces:        splitList(*ifaces),
		IncludeIncomplete: *includeIncomplete,
	})
	if err != nil {
		return err
	}
	return printJSON(w, observations)
}

func printJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}
