package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/go-units"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/opencontainers/specs"
)

const (
	// BANNER is what is printed for help/info output
	BANNER = `                                  _
 _ __ ___   __ _  __ _ _ __   ___| |_ ___
| '_ ` + "`" + ` _ \ / _` + "`" + ` |/ _` + "`" + ` | '_ \ / _ \ __/ _ \
| | | | | | (_| | (_| | | | |  __/ || (_) |
|_| |_| |_|\__,_|\__, |_| |_|\___|\__\___/
                 |___/

 Pipe runc events to a stats TUI (Text User Interface).
 Version: %s

`
	// VERSION is the binary version.
	VERSION = "v0.1.0"

	runtimeSpecFile = "runtime.json"
)

var (
	debug   bool
	version bool
)

func init() {
	// Parse flags
	flag.BoolVar(&version, "version", false, "print version and exit")
	flag.BoolVar(&version, "v", false, "print version and exit (shorthand)")
	flag.BoolVar(&debug, "d", false, "run in debug mode")

	flag.Usage = func() {
		fmt.Fprint(os.Stderr, fmt.Sprintf(BANNER, VERSION))
		flag.PrintDefaults()
	}

	flag.Parse()

	if version {
		fmt.Printf("%s", VERSION)
		os.Exit(0)
	}

	// Set log level
	if debug {
		logrus.SetLevel(logrus.DebugLevel)
	}
}

type event struct {
	Type string             `json:"type"`
	ID   string             `json:"id"`
	Data libcontainer.Stats `json:"data,omitempty"`
}

type containerStats struct {
	CPUUsage         float64
	Memory           float64
	MemoryLimit      float64
	MemoryPercentage float64
	NetworkRx        float64
	NetworkTx        float64
	BlockRead        float64
	BlockWrite       float64
	PidsCurrent      uint64
	mu               sync.RWMutex
	err              error
}

func main() {
	// read the runtime.json for the container so we know things like limits set
	f, err := os.Open(runtimeSpecFile)
	if err != nil {
		if os.IsNotExist(err) {
			logrus.Fatalf("JSON runtime config file %s not found", runtimeSpecFile)
		}
		logrus.Fatal(err)
	}
	defer f.Close()

	var rspec specs.LinuxRuntimeSpec
	if err = json.NewDecoder(f).Decode(&rspec); err != nil {
		logrus.Fatal(err)
	}
	resources := rspec.Linux.Resources

	// create the writer
	w := tabwriter.NewWriter(os.Stdout, 20, 1, 3, ' ', 0)
	printHeader := func() {
		fmt.Fprint(os.Stdout, "\033[2J")
		fmt.Fprint(os.Stdout, "\033[H")
		io.WriteString(w, "CPU USAGE\tMEM USAGE / LIMIT\tMEM %\tNET I/O\tBLOCK I/O\tPIDS\n")
	}

	// collect the stats
	s := &containerStats{}
	go s.Collect(resources)

	for range time.Tick(500 * time.Millisecond) {
		printHeader()
		if err := s.Display(w); err != nil {
			logrus.Errorf("Displaying stats failed: %v", err)
		}
	}
}

func usageAndExit(message string, exitCode int) {
	if message != "" {
		fmt.Fprintf(os.Stderr, message)
		fmt.Fprintf(os.Stderr, "\n\n")
	}
	flag.Usage()
	fmt.Fprintf(os.Stderr, "\n")
	os.Exit(exitCode)
}

func (s *containerStats) Collect(resources *specs.Resources) {
	var (
		dec = json.NewDecoder(os.Stdin)
		u   = make(chan error, 1)
	)
	go func() {
		for {
			var e event
			if err := dec.Decode(&e); err != nil {
				u <- err
				return
			}
			if e.Type != "stats" {
				// do nothing since there are no other events yet
				return
			}

			var memPercent = 0.0

			v := e.Data.CgroupStats
			// MemoryStats.Limit will never be 0 unless the container is not running and we haven't
			// got any data from cgroup
			if resources.Memory.Limit != 0 {
				memPercent = float64(v.MemoryStats.Usage.Usage) / float64(resources.Memory.Limit) * 100.0
			}

			blkRead, blkWrite := calculateBlockIO(v.BlkioStats)
			s.mu.Lock()
			s.CPUUsage = float64(v.CpuStats.CpuUsage.TotalUsage)
			s.Memory = float64(v.MemoryStats.Usage.Usage)
			s.MemoryLimit = float64(resources.Memory.Limit)
			s.MemoryPercentage = memPercent
			s.NetworkRx, s.NetworkTx = calculateNetwork(e.Data.Interfaces)
			s.BlockRead = float64(blkRead)
			s.BlockWrite = float64(blkWrite)
			s.PidsCurrent = v.PidsStats.Current
			s.mu.Unlock()
			u <- nil
		}
	}()
	for {
		select {
		case <-time.After(2 * time.Second):
			// zero out the values if we have not received an update within
			// the specified duration.
			s.mu.Lock()
			s.CPUUsage = 0
			s.Memory = 0
			s.MemoryPercentage = 0
			s.MemoryLimit = 0
			s.NetworkRx = 0
			s.NetworkTx = 0
			s.BlockRead = 0
			s.BlockWrite = 0
			s.mu.Unlock()
		case err := <-u:
			if err != nil {
				s.mu.Lock()
				s.err = err
				s.mu.Unlock()
				return
			}
		}
	}
}

func (s *containerStats) Display(w io.Writer) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.err != nil {
		return s.err
	}
	fmt.Fprintf(w, "%.2f\t%s / %s\t%.2f%%\t%s / %s\t%s / %s\t%d\n",
		s.CPUUsage,
		units.HumanSize(s.Memory), units.HumanSize(s.MemoryLimit),
		s.MemoryPercentage,
		units.HumanSize(s.NetworkRx), units.HumanSize(s.NetworkTx),
		units.HumanSize(s.BlockRead), units.HumanSize(s.BlockWrite),
		s.PidsCurrent)
	return nil
}

func calculateBlockIO(blkio cgroups.BlkioStats) (blkRead uint64, blkWrite uint64) {
	for _, bioEntry := range blkio.IoServiceBytesRecursive {
		switch strings.ToLower(bioEntry.Op) {
		case "read":
			blkRead = blkRead + bioEntry.Value
		case "write":
			blkWrite = blkWrite + bioEntry.Value
		}
	}
	return
}

func calculateNetwork(network []*libcontainer.NetworkInterface) (float64, float64) {
	var rx, tx float64

	for _, v := range network {
		rx += float64(v.RxBytes)
		tx += float64(v.TxBytes)
	}
	return rx, tx
}
