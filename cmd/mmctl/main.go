// mmctl is the command line client for mmapi. Its commands are the GPFS mm*
// commands an administrator already uses on a cluster node — mmlsfs,
// mmlsfileset, mmlsquota, mmsetquota and friends — so what works there works
// here: the same options, the same operands, the same output layout. mmctl
// speaks to mmapi, which forwards to the Scale GUI REST API, so the commands
// work from anywhere the proxy is reachable and stay inside the filesystems the
// token allows.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// progName is the mm command being run. It prefixes diagnostics, the way each
// mm command prefixes its own.
var progName = "mmctl"

type command struct {
	name    string
	summary string
	usage   string
	spec    optSpec
	run     func(o options) error
}

func commands() []*command {
	return []*command{
		cmdMmlscluster(),
		cmdMmlsfs(),
		cmdMmlsfileset(),
		cmdMmcrfileset(),
		cmdMmchfileset(),
		cmdMmdelfileset(),
		cmdMmlinkfileset(),
		cmdMmunlinkfileset(),
		cmdMmlsquota(),
		cmdMmrepquota(),
		cmdMmsetquota(),
		cmdMmcrtoken(),
		cmdMmlstoken(),
		cmdMmdeltoken(),
	}
}

func lookup(name string) *command {
	for _, c := range commands() {
		if c.name == name {
			return c
		}
	}
	return nil
}

// legacyCommands maps the pre-GPFS-alignment subcommands to their replacements
// so an operator running the old form is told what to run instead.
var legacyCommands = map[string]string{
	"cluster":    "mmlscluster",
	"fs":         "mmlsfs",
	"filesystem": "mmlsfs",
	"fileset":    "mmlsfileset, mmcrfileset, mmchfileset, mmdelfileset, mmlinkfileset, mmunlinkfileset",
	"quota":      "mmlsquota, mmrepquota, mmsetquota",
	"token":      "mmcrtoken, mmlstoken, mmdeltoken",
}

func main() {
	name, args := dispatch(os.Args)

	switch name {
	case "", "help", "-h", "--help":
		printUsage()
		if name == "" {
			os.Exit(1)
		}
		return
	}

	cmd := lookup(name)
	if cmd == nil {
		if replacement, ok := legacyCommands[name]; ok {
			fmt.Fprintf(os.Stderr, "mmctl: %q is no longer an mmctl command; use %s.\n", name, replacement)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "mmctl: Unknown command: %s\n", name)
		printUsage()
		os.Exit(1)
	}

	progName = cmd.name
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			fmt.Println(cmd.usage)
			return
		}
	}

	o, err := parseOptions(cmd.spec, args)
	if err != nil {
		fatal(cmd, err)
	}
	if err := cmd.run(o); err != nil {
		fatal(cmd, err)
	}
}

// dispatch resolves the command name. mmctl is normally called as
// "mmctl mmlsquota ...", but a symlink named after the command works too
// (ln -s mmctl mmlsquota; mmlsquota fs0), which is how the mm commands are
// invoked on a cluster node.
func dispatch(argv []string) (string, []string) {
	self := filepath.Base(argv[0])
	if strings.HasPrefix(self, "mm") && self != "mmctl" && lookup(self) != nil {
		return self, argv[1:]
	}
	if len(argv) < 2 {
		return "", nil
	}
	return argv[1], argv[2:]
}

// fatal reports a failure the way mm commands do: a diagnostic prefixed with
// the command name, the synopsis when the operator got the syntax wrong, and a
// "Command failed" summary when the cluster rejected the operation.
func fatal(cmd *command, err error) {
	var usageErr *usageError
	if errors.As(err, &usageErr) {
		fmt.Fprintf(os.Stderr, "%s: %s\n", progName, usageErr.msg)
		if cmd != nil {
			fmt.Fprintln(os.Stderr, cmd.usage)
		}
		os.Exit(1)
	}

	var jobErr *jobFailedError
	if errors.As(err, &jobErr) {
		fmt.Fprintf(os.Stderr, "%s: Command failed. Examine previous error messages to determine cause.\n", progName)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "%s: %s\n", progName, err)
	os.Exit(1)
}

func printUsage() {
	fmt.Println(`mmctl - GPFS command line client for mmapi

Usage: mmctl <mmcommand> [options] [operands]
       mmctl <mmcommand> --help

Commands:`)
	for _, c := range commands() {
		fmt.Printf("  %-16s %s\n", c.name, c.summary)
	}
	fmt.Println(`
Each command takes the options and operands of the GPFS command it is named
after, and prints what that command prints. A symlink named after a command
invokes it directly: ln -s mmctl mmlsquota; mmlsquota fs0.

Environment:
  MMAPI_URL         mmapi server URL (default: https://localhost:8443)
  MMAPI_TOKEN       mmapi access token (GPFS commands)
  MMAPI_ADMIN_TOKEN mmapi admin token (mm*token commands)`)
}
