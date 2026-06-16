package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printRootUsage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "master":
		err = runMaster(parseMasterArgs(os.Args[2:]))
	case "slave":
		err = runSlave(parseSlaveArgs(os.Args[2:]))
	case "-h", "--help", "help":
		printRootUsage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n\n", os.Args[1])
		printRootUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
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
