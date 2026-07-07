package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"netatlas/internal/arpscout"
)

func main() {
	if len(os.Args) < 2 || wantsHelp(os.Args[1:]) {
		printHelp(os.Stdout)
		return
	}

	switch os.Args[1] {
	case "check":
		if err := runCheck(os.Stdout, os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "changes":
		if err := runChanges(os.Stdout, os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "daemon":
		if err := runDaemon(os.Stdout, os.Args[2:]); err != nil && err != context.Canceled {
			log.Fatal(err)
		}
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
	case "scan":
		if err := runScan(os.Stdout, os.Args[2:]); err != nil {
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

func runCheck(w io.Writer, args []string) error {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	flags.SetOutput(w)
	configPath := flags.String("config", "config.ini", "path to NetAtlas INI config")
	format := flags.String("format", "text", "output format: text or json")
	if err := flags.Parse(args); err != nil {
		return err
	}
	daemonCfg, err := arpscout.LoadDaemonConfig(*configPath)
	if err != nil {
		return err
	}
	transportCfg, err := arpscout.LoadTransportConfig(*configPath)
	if err != nil {
		return err
	}
	diag := arpscout.BuildConfigDiagnostics(daemonCfg, transportCfg)
	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		return printJSON(w, diag)
	case "text", "":
		printCheckText(w, diag)
		if !diag.OK {
			return fmt.Errorf("arpscout check failed")
		}
		return nil
	default:
		return fmt.Errorf("unknown check format %q", *format)
	}
}

func printCheckText(w io.Writer, diag arpscout.ConfigDiagnostics) {
	status := "ok"
	if !diag.OK {
		status = "failed"
	}
	fmt.Fprintf(w, "arpscout check: %s\n", status)
	fmt.Fprintf(w, "permissions: arping=%t root=%t euid=%d\n", diag.Permissions.ArpingAvailable, diag.Permissions.RunningAsRoot, diag.Permissions.EffectiveUserID)
	if diag.Permissions.ActiveScanAdvice != "" {
		fmt.Fprintf(w, "advice: %s\n", diag.Permissions.ActiveScanAdvice)
	}
	for _, err := range diag.Errors {
		fmt.Fprintf(w, "error: %s\n", err)
	}
}

func runScan(w io.Writer, args []string) error {
	flags := flag.NewFlagSet("scan", flag.ContinueOnError)
	flags.SetOutput(w)
	cidr := flags.String("cidr", "", "IPv4 CIDR range to scan, for example 192.168.1.0/24")
	iface := flags.String("iface", "", "interface to send ARP requests on")
	all := flags.Bool("all", false, "scan all discovered networks, including normally skipped ones")
	dryRun := flags.Bool("dry-run", false, "plan the scan without sending ARP requests")
	maxHosts := flags.Int("max-hosts", 256, "maximum number of targets allowed in one scan")
	timeout := flags.Duration("timeout", arpscout.DefaultScanTimeout, "per-target arping timeout")
	interval := flags.Duration("interval", arpscout.DefaultScanInterval, "delay between ARP requests")
	format := flags.String("format", "json", "output format: json or text")
	debug := flags.Bool("debug", false, "include debug details such as full planned target lists")
	flagArgs, positionalArgs := splitScanArgs(args)
	if err := flags.Parse(flagArgs); err != nil {
		return err
	}
	if len(positionalArgs) > 1 {
		return fmt.Errorf("scan accepts at most one positional CIDR")
	}
	if len(positionalArgs) == 1 {
		if strings.TrimSpace(*cidr) != "" {
			return fmt.Errorf("use either -cidr or positional CIDR, not both")
		}
		*cidr = positionalArgs[0]
	}
	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json", "text", "":
	default:
		return fmt.Errorf("unknown scan format %q", *format)
	}
	if strings.TrimSpace(*iface) != "" {
		if err := arpscout.ValidateInterfaceNames([]string{*iface}); err != nil {
			return err
		}
	}

	var plan []arpscout.ScanTarget
	var scanOptions []arpscout.ScanOptions
	if strings.TrimSpace(*cidr) != "" {
		plan = []arpscout.ScanTarget{{
			CIDR:        strings.TrimSpace(*cidr),
			Interface:   strings.TrimSpace(*iface),
			AddressType: arpscout.ClassifyNetwork(networkIPFromCIDR(*cidr), *iface).AddressType,
			NetworkType: arpscout.ClassifyNetwork(networkIPFromCIDR(*cidr), *iface).NetworkType,
			Selected:    true,
		}}
		scanOptions = append(scanOptions, arpscout.ScanOptions{
			CIDR:                  strings.TrimSpace(*cidr),
			Interface:             strings.TrimSpace(*iface),
			DryRun:                *dryRun,
			MaxHosts:              *maxHosts,
			Timeout:               *timeout,
			Interval:              *interval,
			IncludePlannedTargets: *debug,
		})
	} else {
		targets, err := arpscout.DiscoverScanTargets(arpscout.ScanTargetOptions{
			Interface: *iface,
			All:       *all,
			MaxHosts:  *maxHosts,
		})
		if err != nil {
			return err
		}
		plan = targets
		for _, target := range targets {
			if !target.Selected {
				continue
			}
			scanOptions = append(scanOptions, arpscout.ScanOptions{
				CIDR:                  target.CIDR,
				Interface:             target.Interface,
				DryRun:                *dryRun,
				MaxHosts:              *maxHosts,
				Timeout:               *timeout,
				Interval:              *interval,
				IncludePlannedTargets: *debug,
			})
			if !*all {
				break
			}
		}
		if len(scanOptions) == 0 {
			return fmt.Errorf("no valid scan target found; use -iface, explicit CIDR, or -all to override skipped discovered networks")
		}
	}

	var results []arpscout.ScanResult
	if !*dryRun {
		if err := arpscout.ActiveScanReadinessError(); err != nil {
			return err
		}
	}
	for _, options := range scanOptions {
		result, err := arpscout.RunActiveScan(options)
		if err != nil {
			return err
		}
		results = append(results, result)
	}

	output := scanCommandOutput{Plan: plan, Results: results}
	if strings.EqualFold(strings.TrimSpace(*format), "text") {
		printScanOutputText(w, output)
		return nil
	}
	if len(results) == 1 && strings.TrimSpace(*cidr) != "" {
		return printJSON(w, results[0])
	}
	return printJSON(w, output)
}

type scanCommandOutput struct {
	Plan    []arpscout.ScanTarget `json:"plan"`
	Results []arpscout.ScanResult `json:"results"`
}

func printScanOutputText(w io.Writer, output scanCommandOutput) {
	fmt.Fprintln(w, "Scan plan:")
	for _, target := range output.Plan {
		status := "skip"
		if target.Selected {
			status = "scan"
		}
		reason := ""
		if target.SkipReason != "" {
			reason = " reason=" + target.SkipReason
		}
		fmt.Fprintf(w, "  %s %s iface=%s address=%s network=%s%s\n", status, target.CIDR, target.Interface, target.AddressType, target.NetworkType, reason)
	}
	for _, result := range output.Results {
		fmt.Fprintf(w, "Result: cidr=%s iface=%s dry_run=%t targets=%d scanned=%d replies=%d duration=%s\n",
			result.CIDR,
			result.Interface,
			result.DryRun,
			result.Statistics.TargetCount,
			result.Statistics.ScannedCount,
			result.Statistics.ReplyCount,
			result.Statistics.Duration,
		)
		if result.DryRun {
			fmt.Fprintf(w, "  range: first=%s last=%s\n", result.Plan.FirstTarget, result.Plan.LastTarget)
			continue
		}
		for _, observation := range result.Discoveries {
			vendor := ""
			if observation.Vendor != nil {
				vendor = " vendor=" + *observation.Vendor
			}
			fmt.Fprintf(w, "  discovered ip=%s mac=%s iface=%s%s\n", observation.IP, observation.MAC, observation.Interface, vendor)
		}
		if len(result.Statistics.NonResponsiveRanges) > 0 {
			fmt.Fprintf(w, "  non-responsive ranges=%d\n", len(result.Statistics.NonResponsiveRanges))
		}
	}
}

func networkIPFromCIDR(cidr string) string {
	parts := strings.SplitN(cidr, "/", 2)
	return strings.TrimSpace(parts[0])
}

func splitScanArgs(args []string) ([]string, []string) {
	valueFlags := map[string]struct{}{
		"-cidr":      {},
		"-iface":     {},
		"-max-hosts": {},
		"-timeout":   {},
		"-interval":  {},
		"-format":    {},
	}
	var flagArgs []string
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}
		flagArgs = append(flagArgs, arg)
		name := arg
		if idx := strings.IndexByte(arg, '='); idx >= 0 {
			name = arg[:idx]
		}
		if _, ok := valueFlags[name]; ok && !strings.Contains(arg, "=") && i+1 < len(args) {
			i++
			flagArgs = append(flagArgs, args[i])
		}
	}
	return flagArgs, positional
}

func runChanges(w io.Writer, args []string) error {
	flags := flag.NewFlagSet("changes", flag.ContinueOnError)
	flags.SetOutput(w)
	previousPath := flags.String("previous", "", "previous observation JSON file")
	currentPath := flags.String("current", "", "current observation JSON file")
	gatewayIP := flags.String("gateway", "", "gateway IP to monitor for MAC changes")
	format := flags.String("format", "json", "output format: json or text")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*previousPath) == "" || strings.TrimSpace(*currentPath) == "" {
		return fmt.Errorf("changes requires -previous and -current")
	}

	previous, err := readObservationsFile(*previousPath)
	if err != nil {
		return err
	}
	current, err := readObservationsFile(*currentPath)
	if err != nil {
		return err
	}
	events := arpscout.DetectChanges(previous, current, arpscout.ChangeOptions{GatewayIP: *gatewayIP})
	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json", "":
		return printJSON(w, events)
	case "text":
		printChangeEventsText(w, events)
		return nil
	default:
		return fmt.Errorf("unknown changes format %q", *format)
	}
}

func runDaemon(w io.Writer, args []string) error {
	flags := flag.NewFlagSet("daemon", flag.ContinueOnError)
	flags.SetOutput(w)
	configPath := flags.String("config", "config.ini", "path to NetAtlas INI config")
	ifaces := flags.String("iface", "", "comma-separated interfaces to read")
	includeIncomplete := flags.Bool("include-incomplete", false, "include INCOMPLETE neighbour entries")
	interval := flags.Duration("interval", 0, "passive read interval")
	statusInterval := flags.Duration("status-interval", 0, "runtime status interval")
	gatewayIP := flags.String("gateway", "", "gateway IP to monitor for MAC changes")
	activeScan := flags.Bool("active-scan", false, "enable periodic active ARP scan")
	scanCIDR := flags.String("scan-cidr", "", "IPv4 CIDR for periodic active scan")
	scanInterval := flags.Duration("scan-interval", 0, "periodic active scan interval")
	maxScanHosts := flags.Int("max-scan-hosts", 0, "maximum active scan targets")
	format := flags.String("format", "text", "output format: text or json")
	once := flags.Bool("once", false, "run one daemon iteration and exit")
	if err := flags.Parse(args); err != nil {
		return err
	}

	cfg, err := arpscout.LoadDaemonConfig(*configPath)
	if err != nil {
		return err
	}
	identityCfg, err := arpscout.LoadIdentityConfig(*configPath)
	if err != nil {
		return err
	}
	transportCfg, err := arpscout.LoadTransportConfig(*configPath)
	if err != nil {
		return err
	}
	if values := splitList(*ifaces); len(values) > 0 {
		cfg.Interfaces = values
		identityCfg.Interfaces = values
	}
	if *includeIncomplete {
		cfg.IncludeIncomplete = true
	}
	if *interval > 0 {
		cfg.PassiveInterval = *interval
	}
	if *statusInterval > 0 {
		cfg.StatusInterval = *statusInterval
	}
	if strings.TrimSpace(*gatewayIP) != "" {
		cfg.GatewayIP = strings.TrimSpace(*gatewayIP)
	}
	if *activeScan {
		cfg.ActiveScan = true
	}
	if strings.TrimSpace(*scanCIDR) != "" {
		cfg.ScanCIDR = strings.TrimSpace(*scanCIDR)
	}
	if *scanInterval > 0 {
		cfg.ScanInterval = *scanInterval
	}
	if *maxScanHosts > 0 {
		cfg.MaxScanHosts = *maxScanHosts
	}
	if err := arpscout.ValidateInterfaceNames(cfg.Interfaces); err != nil {
		return err
	}
	if err := arpscout.ValidateDaemonConfig(cfg); err != nil {
		return err
	}
	if err := arpscout.ValidateTransportConfig(transportCfg); err != nil {
		return err
	}
	if cfg.ActiveScan {
		if err := arpscout.ActiveScanReadinessError(); err != nil {
			return err
		}
	}
	identity, err := arpscout.BuildIdentity(identityCfg)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cancelOnce := func() {}
	if *once {
		onceCtx, cancel := context.WithCancel(ctx)
		ctx = onceCtx
		cancelOnce = cancel
		defer cancelOnce()
	}
	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json", "text", "":
	default:
		return fmt.Errorf("unknown daemon format %q", *format)
	}

	output := func(event arpscout.DaemonEvent) {
		if *once && event.Kind == arpscout.DaemonEventBatch {
			cancelOnce()
		}
		switch strings.ToLower(strings.TrimSpace(*format)) {
		case "json":
			_ = printJSON(w, event)
		case "text", "":
			printDaemonEventText(w, event)
		}
	}
	var transport arpscout.Transport
	if transportCfg.Enabled || transportCfg.DryRunPath != "" {
		var core arpscout.Transport
		if transportCfg.Enabled {
			client, err := arpscout.NewCoreClient(transportCfg)
			if err != nil {
				return err
			}
			core = client
		}
		transport = &arpscout.Uploader{
			Transport:  core,
			SpoolPath:  transportCfg.SpoolPath,
			DryRunPath: transportCfg.DryRunPath,
			Retries:    transportCfg.Retries,
		}
	}

	return arpscout.RunDaemon(ctx, arpscout.DaemonOptions{
		Config:    cfg,
		Identity:  identity,
		Transport: transport,
		Output:    output,
	})
}

func printDaemonEventText(w io.Writer, event arpscout.DaemonEvent) {
	switch event.Kind {
	case arpscout.DaemonEventStatus:
		fmt.Fprintf(w, "status iterations=%d observations=%d changes=%d last_run=%s\n",
			event.Status.Iterations,
			event.Status.Observations,
			event.Status.Changes,
			formatOptionalTime(event.Status.LastRunAt),
		)
	case arpscout.DaemonEventBatch:
		if event.Error != "" {
			fmt.Fprintf(w, "batch error=%q iterations=%d\n", event.Error, event.Status.Iterations)
			return
		}
		fmt.Fprintf(w, "batch observations=%d changes=%d iterations=%d\n",
			len(event.Observations),
			len(event.Changes),
			event.Status.Iterations,
		)
		for _, change := range event.Changes {
			fmt.Fprintf(w, "  %s: %s\n", change.Type, change.Message)
		}
	}
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Format(time.RFC3339)
}

func readObservationsFile(path string) ([]arpscout.Observation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read observations %q: %w", path, err)
	}
	var observations []arpscout.Observation
	if err := json.Unmarshal(data, &observations); err != nil {
		return nil, fmt.Errorf("parse observations %q: %w", path, err)
	}
	return observations, nil
}

func printChangeEventsText(w io.Writer, events []arpscout.ChangeEvent) {
	if len(events) == 0 {
		fmt.Fprintln(w, "no changes")
		return
	}
	for _, event := range events {
		fmt.Fprintf(w, "%s: %s\n", event.Type, event.Message)
	}
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
