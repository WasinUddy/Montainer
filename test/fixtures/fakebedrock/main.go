// fakebedrock is a deterministic stand-in for the Minecraft Bedrock server.
//
// It deliberately implements only the process contract Montainer depends on:
// a long-running executable that writes logs to stdout/stderr and accepts
// newline-delimited commands on stdin. Acceptance tests configure its behavior
// through environment variables; production code only sees a normal process.
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultStartupLine = "Fake Bedrock server started."
	defaultStoppedLine = "Fake Bedrock server stopped."
)

var releaseActiveOnce sync.Once

func main() {
	claimActiveSlot()
	defer releaseActiveSlot()
	writePID()
	appendLine(os.Getenv("FAKE_BEDROCK_STARTS_FILE"), strconv.Itoa(os.Getpid()))
	installSignalHandler()

	sleepFromEnv("FAKE_BEDROCK_READY_DELAY")
	writeConfiguredLine(os.Stdout, "FAKE_BEDROCK_STARTUP_STDOUT", defaultStartupLine)
	writeConfiguredLine(os.Stderr, "FAKE_BEDROCK_STARTUP_STDERR", "")
	startExitTimer()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		command := scanner.Text()
		appendLine(os.Getenv("FAKE_BEDROCK_COMMANDS_FILE"), command)
		handleCommand(command)
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "fake Bedrock stdin error: %v\n", err)
		exit(70)
	}
}

func handleCommand(command string) {
	name, argument, _ := strings.Cut(command, " ")

	switch name {
	case "stop":
		if boolFromEnv("FAKE_BEDROCK_IGNORE_STOP") {
			fmt.Fprintln(os.Stdout, "Fake Bedrock ignored stop command.")
			return
		}
		sleepFromEnv("FAKE_BEDROCK_STOP_DELAY")
		appendLine(os.Getenv("FAKE_BEDROCK_GRACEFUL_EXITS_FILE"), strconv.Itoa(os.Getpid()))
		fmt.Fprintln(os.Stdout, envOrDefault("FAKE_BEDROCK_STOPPED_STDOUT", defaultStoppedLine))
		exit(0)
	case "emit":
		fmt.Fprintln(os.Stdout, argument)
	case "emiterr":
		fmt.Fprintln(os.Stderr, argument)
	case "crash":
		code := 23
		if parsed, err := strconv.Atoi(strings.TrimSpace(argument)); err == nil && parsed != 0 {
			code = parsed
		}
		fmt.Fprintf(os.Stderr, "Fake Bedrock crashed with exit code %d.\n", code)
		exit(code)
	default:
		fmt.Fprintf(os.Stdout, "Fake Bedrock received command: %s\n", command)
	}
}

func writePID() {
	path := os.Getenv("FAKE_BEDROCK_PID_FILE")
	if path == "" {
		return
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "cannot write fake Bedrock PID file: %v\n", err)
		exit(71)
	}
}

func claimActiveSlot() {
	path := os.Getenv("FAKE_BEDROCK_ACTIVE_FILE")
	if path == "" {
		return
	}

	for attempt := 0; attempt < 2; attempt++ {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, writeErr := fmt.Fprintln(file, os.Getpid())
			closeErr := file.Close()
			if writeErr != nil || closeErr != nil {
				fmt.Fprintf(os.Stderr, "cannot write fake Bedrock active file: %v %v\n", writeErr, closeErr)
				exit(76)
			}
			return
		}
		if !os.IsExist(err) {
			fmt.Fprintf(os.Stderr, "cannot create fake Bedrock active file: %v\n", err)
			exit(76)
		}

		existingPID := processIDFromFile(path)
		if existingPID > 1 && processExists(existingPID) {
			appendLine(os.Getenv("FAKE_BEDROCK_OVERLAPS_FILE"), fmt.Sprintf("existing=%d new=%d", existingPID, os.Getpid()))
			fmt.Fprintf(os.Stderr, "fake Bedrock overlap detected: process %d is still active\n", existingPID)
			// This process does not own the active file, so it must not remove it.
			os.Exit(76)
		}
		_ = os.Remove(path)
	}

	fmt.Fprintln(os.Stderr, "cannot claim fake Bedrock active slot")
	os.Exit(76)
}

func releaseActiveSlot() {
	releaseActiveOnce.Do(func() {
		path := os.Getenv("FAKE_BEDROCK_ACTIVE_FILE")
		if path != "" && processIDFromFile(path) == os.Getpid() {
			_ = os.Remove(path)
		}
	})
}

func processIDFromFile(path string) int {
	contents, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(contents)))
	return pid
}

func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func exit(code int) {
	releaseActiveSlot()
	os.Exit(code)
}

func appendLine(path, line string) {
	if path == "" {
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot open fake Bedrock record file: %v\n", err)
		exit(72)
	}
	defer file.Close()
	if _, err := fmt.Fprintln(file, line); err != nil {
		fmt.Fprintf(os.Stderr, "cannot write fake Bedrock record file: %v\n", err)
		exit(73)
	}
}

func writeConfiguredLine(file *os.File, key, fallback string) {
	line := envOrDefault(key, fallback)
	if line != "" {
		fmt.Fprintln(file, line)
	}
}

func envOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func boolFromEnv(key string) bool {
	value, err := strconv.ParseBool(os.Getenv(key))
	return err == nil && value
}

func sleepFromEnv(key string) {
	value := os.Getenv(key)
	if value == "" {
		return
	}
	delay, err := time.ParseDuration(value)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid %s duration %q: %v\n", key, value, err)
		exit(74)
	}
	time.Sleep(delay)
}

func startExitTimer() {
	value := os.Getenv("FAKE_BEDROCK_EXIT_AFTER")
	if value == "" {
		return
	}
	delay, err := time.ParseDuration(value)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid FAKE_BEDROCK_EXIT_AFTER duration %q: %v\n", value, err)
		exit(75)
	}
	code := 24
	if parsed, err := strconv.Atoi(os.Getenv("FAKE_BEDROCK_EXIT_CODE")); err == nil && parsed != 0 {
		code = parsed
	}
	go func() {
		time.Sleep(delay)
		fmt.Fprintf(os.Stderr, "Fake Bedrock timed exit with code %d.\n", code)
		exit(code)
	}()
}

func installSignalHandler() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-signals
		appendLine(os.Getenv("FAKE_BEDROCK_SIGNALS_FILE"), sig.String())
		fmt.Fprintf(os.Stderr, "Fake Bedrock received signal: %s\n", sig)
		exit(128 + int(sig.(syscall.Signal)))
	}()
}
