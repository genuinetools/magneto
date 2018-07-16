package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"

	units "github.com/docker/go-units"
	"github.com/genuinetools/magneto/types"
	"github.com/genuinetools/magneto/version"
	"github.com/genuinetools/pkg/cli"
	"github.com/opencontainers/runc/libcontainer/system"
	"github.com/sirupsen/logrus"
)

const (
	nanoSecondsPerSecond = 1e9
)

var (
	debug bool
)

type event struct {
	Type string      `json:"type"`
	ID   string      `json:"id"`
	Data types.Stats `json:"data,omitempty"`
}

type containerStats struct {
	CPUPercentage       float64
	Memory              float64
	MemoryLimit         float64
	MemoryPercentage    float64
	NetworkRx           float64
	NetworkTx           float64
	BlockRead           float64
	BlockWrite          float64
	PidsCurrent         uint64
	mu                  sync.RWMutex
	bufReader           *bufio.Reader
	clockTicksPerSecond uint64
	err                 error
}

func main() {
	// Create a new cli program.
	p := cli.NewProgram()
	p.Name = "magneto"
	p.Description = "Pipe runc events to a stats TUI (Text User Interface)"

	// Set the GitCommit and Version.
	p.GitCommit = version.GITCOMMIT
	p.Version = version.VERSION

	// Setup the global flags.
	p.FlagSet = flag.NewFlagSet("global", flag.ExitOnError)
	p.FlagSet.BoolVar(&debug, "d", false, "enable debug logging")

	// Set the before function.
	p.Before = func(ctx context.Context) error {
		// Set the log level.
		if debug {
			logrus.SetLevel(logrus.DebugLevel)
		}

		return nil
	}

	// Set the main program action.
	p.Action = func(ctx context.Context, args []string) error {
		// On ^C, or SIGTERM handle exit.
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		signal.Notify(c, syscall.SIGTERM)
		go func() {
			for sig := range c {
				logrus.Infof("Received %s, exiting.", sig.String())
				os.Exit(0)
			}
		}()
		// create the writer
		w := tabwriter.NewWriter(os.Stdout, 20, 1, 3, ' ', 0)
		printHeader := func() {
			fmt.Fprint(os.Stdout, "\033[2J")
			fmt.Fprint(os.Stdout, "\033[H")
			io.WriteString(w, "CPU %\tMEM USAGE / LIMIT\tMEM %\tNET I/O\tBLOCK I/O\tPIDS\n")
		}

		// collect the stats
		s := &containerStats{
			clockTicksPerSecond: uint64(system.GetClockTicks()),
			bufReader:           bufio.NewReaderSize(nil, 128),
		}

		go s.collect()

		for range time.Tick(5 * time.Second) {
			printHeader()
			if err := s.Display(w); err != nil {
				logrus.Error(err)
			}
			w.Flush()
		}

		return nil
	}

	// Run our program.
	p.Run()
}

func (s *containerStats) collect() {
	var (
		previousCPU    uint64
		previousSystem uint64
		dec            = json.NewDecoder(os.Stdin)
		u              = make(chan error, 1)
	)

	go func() {
		for {
			var (
				e                      event
				memPercent, cpuPercent float64
				blkRead, blkWrite      uint64 // Only used on Linux
				mem, memLimit          float64
				netRx, netTx           float64
				pidsCurrent            uint64
			)

			if err := dec.Decode(&e); err != nil {
				u <- err
				time.Sleep(100 * time.Millisecond)
				continue
			}

			if e.Type != "stats" {
				// do nothing since there are no other events yet
				continue
			}
			v := e.Data

			systemUsage, err := s.getSystemCPUUsage()
			if err != nil {
				u <- fmt.Errorf("collecting system cpu usage failed: %v", err)
				continue
			}

			cpuPercent = calculateCPUPercent(previousCPU, previousSystem, systemUsage, v)
			previousCPU = v.CPU.Usage.Total
			previousSystem = systemUsage

			blkRead, blkWrite = calculateBlockIO(v.Blkio)

			mem = calculateMemUsageNoCache(v.Memory)
			memLimit = float64(v.Memory.Usage.Limit)
			memPercent = calculateMemPercentNoCache(s.MemoryLimit, s.Memory)

			pidsCurrent = v.Pids.Current

			// set the stats
			s.mu.Lock()
			s.CPUPercentage = cpuPercent
			s.BlockRead = float64(blkRead)
			s.BlockWrite = float64(blkWrite)
			s.Memory = mem
			s.MemoryLimit = memLimit
			s.MemoryPercentage = memPercent
			s.NetworkRx = netRx
			s.NetworkTx = netTx
			s.PidsCurrent = pidsCurrent
			s.mu.Unlock()

			u <- nil
		}
	}()

	for {
		select {
		case err := <-u:
			s.setError(err)
			continue
		}
	}
}

func (s *containerStats) Display(w io.Writer) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// check the error here
	if s.err != nil {
		return s.err
	}

	fmt.Fprintf(w, "%.2f%%\t%s / %s\t%.2f%%\t%s / %s\t%s / %s\t%d\n",
		s.CPUPercentage,
		units.BytesSize(s.Memory), units.BytesSize(s.MemoryLimit),
		s.MemoryPercentage,
		units.HumanSizeWithPrecision(s.NetworkRx, 3), units.HumanSizeWithPrecision(s.NetworkTx, 3),
		units.HumanSizeWithPrecision(s.BlockRead, 3), units.HumanSizeWithPrecision(s.BlockWrite, 3),
		s.PidsCurrent)
	return nil
}

// setError sets container statistics error
func (s *containerStats) setError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

func calculateCPUPercent(previousCPU, previousSystem, systemUsage uint64, v types.Stats) float64 {
	var (
		cpuPercent = 0.0
		// calculate the change for the cpu usage of the container in between readings
		cpuDelta = float64(v.CPU.Usage.Total) - float64(previousCPU)
		// calculate the change for the entire system between readings
		systemDelta = float64(systemUsage) - float64(previousSystem)
	)

	if systemDelta > 0.0 && cpuDelta > 0.0 {
		cpuPercent = (cpuDelta / systemDelta) * float64(len(v.CPU.Usage.Percpu)) * 100.0
	}
	return cpuPercent
}

func calculateBlockIO(blkio types.Blkio) (uint64, uint64) {
	var blkRead, blkWrite uint64
	for _, bioEntry := range blkio.IoServiceBytesRecursive {
		switch strings.ToLower(bioEntry.Op) {
		case "read":
			blkRead = blkRead + bioEntry.Value
		case "write":
			blkWrite = blkWrite + bioEntry.Value
		}
	}
	return blkRead, blkWrite
}

// calculateMemUsageNoCache calculate memory usage of the container.
// Page cache is intentionally excluded to avoid misinterpretation of the output.
func calculateMemUsageNoCache(mem types.Memory) float64 {
	return float64(mem.Usage.Usage - mem.Cache)
}

func calculateMemPercentNoCache(limit float64, usedNoCache float64) float64 {
	// MemoryStats.Limit will never be 0 unless the container is not running and we haven't
	// got any data from cgroup
	if limit != 0 {
		return usedNoCache / limit * 100.0
	}
	return 0
}

// getSystemCPUUsage returns the host system's cpu usage in
// nanoseconds. An error is returned if the format of the underlying
// file does not match.
//
// Uses /proc/stat defined by POSIX. Looks for the cpu
// statistics line and then sums up the first seven fields
// provided. See `man 5 proc` for details on specific field
// information.
func (s *containerStats) getSystemCPUUsage() (uint64, error) {
	var line string
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, err
	}
	defer func() {
		s.bufReader.Reset(nil)
		f.Close()
	}()
	s.bufReader.Reset(f)
	err = nil
	for err == nil {
		line, err = s.bufReader.ReadString('\n')
		if err != nil {
			break
		}
		parts := strings.Fields(line)
		switch parts[0] {
		case "cpu":
			if len(parts) < 8 {
				return 0, fmt.Errorf("invalid number of cpu fields")
			}
			var totalClockTicks uint64
			for _, i := range parts[1:8] {
				v, err := strconv.ParseUint(i, 10, 64)
				if err != nil {
					return 0, fmt.Errorf("Unable to convert value %s to int: %s", i, err)
				}
				totalClockTicks += v
			}
			return (totalClockTicks * nanoSecondsPerSecond) /
				s.clockTicksPerSecond, nil
		}
	}

	return 0, fmt.Errorf("invalid stat format. Error trying to parse the '/proc/stat' file")
}
