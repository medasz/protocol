package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

var errUsage = errors.New("usage")

func main() {
	if err := runCLI(os.Args[1:]); err != nil {
		if errors.Is(err, errUsage) {
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runCLI(args []string) error {
	if len(args) < 1 {
		printRootUsage()
		return errUsage
	}

	switch args[0] {
	case "master":
		return runMaster(parseMasterArgs(args[1:]))
	case "slave":
		return runSlave(parseSlaveArgs(args[1:]))
	case "-h", "--help", "help":
		printRootUsage()
		return nil
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n\n", args[0])
		printRootUsage()
		return errUsage
	}
}

func parseMasterArgs(args []string) masterConfig {
	fs := flag.NewFlagSet("master", flag.ExitOnError)
	cfg := masterConfig{}
	fs.StringVar(&cfg.src, "src", "", "Source IP address")
	fs.StringVar(&cfg.dst, "dst", "", "Destination IP address")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s master -src <ip> -dst <ip>\n", os.Args[0])
		fs.PrintDefaults()
	}
	fs.Parse(args)
	return cfg
}

func parseSlaveArgs(args []string) slaveConfig {
	fs := flag.NewFlagSet("slave", flag.ExitOnError)
	cfg := slaveConfig{
		delay:       DefaultDelay,
		timeout:     DefaultTimeout,
		maxBlanks:   DefaultMaxBlanks,
		maxDataSize: DefaultMaxDataSize,
	}
	fs.StringVar(&cfg.target, "t", "", "host ip address to send ping requests to")
	fs.BoolVar(&cfg.isTest, "r", false, "send a single test icmp request and then quit")
	fs.IntVar(&cfg.delay, "d", DefaultDelay, "delay between requests in milliseconds")
	fs.IntVar(&cfg.timeout, "o", DefaultTimeout, "timeout in milliseconds")
	fs.IntVar(&cfg.maxBlanks, "b", DefaultMaxBlanks, "maximal number of blanks (unanswered icmp requests) before quitting")
	fs.IntVar(&cfg.maxDataSize, "s", DefaultMaxDataSize, "maximal data buffer size in bytes")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s slave -t <ip> [options]\n", os.Args[0])
		fs.PrintDefaults()
	}
	fs.Parse(args)
	return cfg
}

func printRootUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s <master|slave> [options]\n\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "Subcommands:")
	fmt.Fprintln(os.Stderr, "  master   Listen for ICMP echo requests and send commands back in replies")
	fmt.Fprintln(os.Stderr, "  slave    Poll the master via ICMP and execute received commands")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "Run '%s <subcommand> -h' for subcommand options.\n", os.Args[0])
}
