package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/langkurt/discord-cli/internal/config"
	"github.com/langkurt/discord-cli/internal/storage"
)

const launchdLabel = "com.langkurt.discocli.follow"

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run live sync in the background so the data is always current",
	Long: `Installs ` + "`discocli sync --follow`" + ` as a launchd user agent. It starts at
login and restarts if it ever exits (no network at boot, token check failed,
crash). Each time it connects it catches up on anything missed while the
machine was asleep or offline, then stores new messages as they arrive.

It follows the default guild in <store>/guild, or every guild if none is set.

  discocli daemon install     # start it, and at every login
  discocli daemon status      # is it alive, and how current is the data
  discocli daemon log         # what it has been doing
  discocli daemon uninstall   # stop it and forget it`,
}

var daemonInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install and start the background follower",
	RunE: func(cmd *cobra.Command, args []string) error {
		if runtime.GOOS != "darwin" {
			return fmt.Errorf("daemon install uses launchd and only works on macOS; run `discocli sync --follow --quiet` under your own supervisor instead")
		}
		store := resolvedStore()
		if err := config.EnsureStore(store); err != nil {
			return err
		}
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("find the discocli binary: %w", err)
		}
		if exe, err = filepath.EvalSymlinks(exe); err != nil {
			return err
		}

		plistPath, err := launchAgentPath()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
			return err
		}
		// Replacing a running agent: stop the old one first, otherwise
		// launchd keeps the previous definition.
		if launchdLoaded() {
			if err := bootout(plistPath); err != nil {
				return err
			}
		}
		if err := os.WriteFile(plistPath, []byte(launchAgentPlist(exe, store)), 0o644); err != nil {
			return err
		}
		if err := bootstrap(plistPath); err != nil {
			return err
		}
		fmt.Printf("Installed %s\n", plistPath)
		fmt.Printf("Log: %s\n", followLogPath(store))
		fmt.Println("Check it with `discocli daemon status`.")
		return nil
	},
}

var daemonUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Stop the background follower and remove it",
	RunE: func(cmd *cobra.Command, args []string) error {
		plistPath, err := launchAgentPath()
		if err != nil {
			return err
		}
		if _, err := os.Stat(plistPath); os.IsNotExist(err) {
			fmt.Println("Not installed.")
			return nil
		}
		if launchdLoaded() {
			if err := bootout(plistPath); err != nil {
				fmt.Fprintf(os.Stderr, "warning: %v\n", err)
			}
		}
		if err := os.Remove(plistPath); err != nil {
			return err
		}
		fmt.Printf("Removed %s\n", plistPath)
		return nil
	},
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show whether the background follower is running and how current the data is",
	RunE: func(cmd *cobra.Command, args []string) error {
		store := resolvedStore()
		plistPath, err := launchAgentPath()
		if err != nil {
			return err
		}
		switch {
		case !fileExists(plistPath):
			fmt.Println("launchd agent: not installed (`discocli daemon install`)")
		case !launchdLoaded():
			fmt.Printf("launchd agent: installed but not loaded (%s)\n", plistPath)
		default:
			out, _ := runLaunchctl("print", launchdTarget())
			fmt.Printf("launchd agent: %s\n", launchdField(out, "state"))
			if pid := launchdField(out, "pid"); pid != "" {
				fmt.Printf("pid:           %s\n", pid)
			}
		}

		db, err := storage.Open(config.DBPath(store))
		if err != nil {
			return err
		}
		defer db.Close()
		guild := config.DefaultGuild(store)
		if latest, err := db.LatestMessageTime(guild); err == nil && !latest.IsZero() {
			fmt.Printf("newest message: %s ago\n", time.Since(latest).Round(time.Second))
		}
		fmt.Printf("log:           %s\n", followLogPath(store))
		return nil
	},
}

var daemonLogCmd = &cobra.Command{
	Use:   "log",
	Short: "Print the tail of the background follower's log",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := followLogPath(resolvedStore())
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			fmt.Printf("No log yet at %s\n", path)
			return nil
		}
		if err != nil {
			return err
		}
		// Only the tail is interesting; launchd appends forever.
		lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		if len(lines) > 50 {
			lines = lines[len(lines)-50:]
		}
		fmt.Println(strings.Join(lines, "\n"))
		return nil
	},
}

func launchAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist"), nil
}

func followLogPath(store string) string { return filepath.Join(store, "follow.log") }

// launchAgentPlist renders the agent. KeepAlive restarts the follower if it
// ever exits; ThrottleInterval stops a crash loop from spinning. The store
// is passed explicitly because a launchd agent does not get the shell
// environment.
func launchAgentPlist(exe, store string) string {
	args := []string{exe, "sync", "--follow", "--quiet", "--store", store}
	var argXML strings.Builder
	for _, a := range args {
		fmt.Fprintf(&argXML, "\t\t<string>%s</string>\n", xmlEscape(a))
	}
	log := xmlEscape(followLogPath(store))
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + launchdLabel + `</string>
	<key>ProgramArguments</key>
	<array>
` + argXML.String() + `	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>ThrottleInterval</key>
	<integer>60</integer>
	<key>ProcessType</key>
	<string>Background</string>
	<key>LowPriorityIO</key>
	<true/>
	<key>StandardOutPath</key>
	<string>` + log + `</string>
	<key>StandardErrorPath</key>
	<string>` + log + `</string>
</dict>
</plist>
`
}

func launchdTarget() string { return "gui/" + strconv.Itoa(os.Getuid()) + "/" + launchdLabel }

// bootstrap loads the agent into the user's launchd domain, falling back to
// the older `load` verb. `load` can exit 0 without loading anything, so
// success is judged by whether launchd actually has the service afterwards.
func bootstrap(plistPath string) error {
	out, err := runLaunchctl("bootstrap", "gui/"+strconv.Itoa(os.Getuid()), plistPath)
	if err != nil {
		out2, err2 := runLaunchctl("load", "-w", plistPath)
		if err2 != nil || !launchdLoaded() {
			return fmt.Errorf("launchctl bootstrap: %v (%s); load: %v (%s)", err, out, err2, out2)
		}
	}
	return nil
}

// bootout returns once launchd has let go of the service: bootout is
// asynchronous, and bootstrapping over a service still being torn down
// fails with an I/O error.
func bootout(plistPath string) error {
	if out, err := runLaunchctl("bootout", launchdTarget()); err != nil {
		if out2, err2 := runLaunchctl("unload", plistPath); err2 != nil {
			return fmt.Errorf("launchctl bootout: %v (%s); unload: %v (%s)", err, out, err2, out2)
		}
	}
	for deadline := time.Now().Add(10 * time.Second); launchdLoaded(); {
		if time.Now().After(deadline) {
			return fmt.Errorf("launchd still has %s 10s after bootout", launchdLabel)
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil
}

func launchdLoaded() bool {
	_, err := runLaunchctl("print", launchdTarget())
	return err == nil
}

// launchdField pulls "key = value" out of `launchctl print` output.
func launchdField(out, key string) string {
	for _, line := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), " = "); ok && k == key {
			return v
		}
	}
	return ""
}

func runLaunchctl(args ...string) (string, error) {
	out, err := exec.Command("launchctl", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func init() {
	daemonCmd.AddCommand(daemonInstallCmd, daemonUninstallCmd, daemonStatusCmd, daemonLogCmd)
	rootCmd.AddCommand(daemonCmd)
}
