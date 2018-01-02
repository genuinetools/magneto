package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	units "github.com/docker/go-units"
	"github.com/jessfraz/magneto/types"
	"github.com/jessfraz/magneto/version"
	"github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/opencontainers/runc/libcontainer/system"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
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
 Build: %s

`

	specFile    = "config.json"
	stateFile   = "state.json"
	defaultRoot = "/run/runc"
)

var (
	root string

	debug bool
	vrsn  bool
)

func init() {
	// Parse flags
	flag.StringVar(&root, "root", defaultRoot, "root directory of runc storage of container state")
	flag.BoolVar(&vrsn, "version", false, "print version and exit")
	flag.BoolVar(&vrsn, "v", false, "print version and exit (shorthand)")
	flag.BoolVar(&debug, "d", false, "run in debug mode")

	flag.Usage = func() {
		fmt.Fprint(os.Stderr, fmt.Sprintf(BANNER, version.VERSION, version.GITCOMMIT))
		flag.PrintDefaults()
	}

	flag.Parse()

	if vrsn {
		fmt.Printf("magneto version %s, build %s", version.VERSION, version.GITCOMMIT)
		os.Exit(0)
	}

	// Set log level
	if debug {
		logrus.SetLevel(logrus.DebugLevel)
	}
}

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
	go s.Collect()

	for range time.Tick(5 * time.Second) {
		printHeader()
		if err := s.Display(w); err != nil {
			logrus.Errorf("Displaying stats failed: %v", err)
		}
		w.Flush()
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

func (s *containerStats) Collect() {
	var (
		previousCPU    uint64
		previousSystem uint64
		dec            = json.NewDecoder(os.Stdin)
		u              = make(chan error, 1)
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
			var cpuPercent = 0.0

			v := e.Data.CgroupStats
			if v == nil {
				return
			}

			resources, err := getContainerResources(e.ID)
			if err != nil {
				u <- fmt.Errorf("Getting container's configured resources failed: %v", err)
				return
			}

			// MemoryStats.Limit will never be 0 unless the container is not running and we haven't
			// got any data from cgroup
			if int(*resources.Memory.Limit) != 0 {
				memPercent = float64(v.MemoryStats.Usage.Usage) / float64(*resources.Memory.Limit) * 100.0
			}

			systemUsage, err := s.getSystemCPUUsage()
			if err != nil {
				u <- fmt.Errorf("collecting system cpu usage failed: %v", err)
				continue
			}

			cpuPercent = calculateCPUPercent(previousCPU, previousSystem, systemUsage, v)
			previousCPU = v.CpuStats.CpuUsage.TotalUsage
			previousSystem = systemUsage
			blkRead, blkWrite := calculateBlockIO(v.BlkioStats)
			s.mu.Lock()
			s.CPUPercentage = cpuPercent
			s.Memory = float64(v.MemoryStats.Usage.Usage)
			s.MemoryLimit = float64(*resources.Memory.Limit)
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
		case err := <-u:
			if err != nil {
				s.mu.Lock()
				s.err = err
				s.mu.Unlock()
				logrus.Fatal(err)
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
	fmt.Fprintf(w, "%.2f%%\t%s / %s\t%.2f%%\t%s / %s\t%s / %s\t%d\n",
		s.CPUPercentage,
		units.HumanSize(s.Memory), units.HumanSize(s.MemoryLimit),
		s.MemoryPercentage,
		units.HumanSize(s.NetworkRx), units.HumanSize(s.NetworkTx),
		units.HumanSize(s.BlockRead), units.HumanSize(s.BlockWrite),
		s.PidsCurrent)
	return nil
}

func calculateCPUPercent(previousCPU, previousSystem, systemUsage uint64, v *cgroups.Stats) float64 {
	var (
		cpuPercent = 0.0
		// calculate the change for the cpu usage of the container in between readings
		cpuDelta = float64(v.CpuStats.CpuUsage.TotalUsage) - float64(previousCPU)
		// calculate the change for the entire system between readings
		systemDelta = float64(systemUsage) - float64(previousSystem)
	)

	if systemDelta > 0.0 && cpuDelta > 0.0 {
		cpuPercent = (cpuDelta / systemDelta) * float64(len(v.CpuStats.CpuUsage.PercpuUsage)) * 100.0
	}
	return cpuPercent
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

func calculateNetwork(network []*types.NetworkInterface) (float64, float64) {
	var rx, tx float64

	for _, v := range network {
		rx += float64(v.RxBytes)
		tx += float64(v.TxBytes)
	}
	return rx, tx
}

const nanoSecondsPerSecond = 1e9

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
				return 0, fmt.Errorf("Bad CPU fields")
			}
			var totalClockTicks uint64
			for _, i := range parts[1:8] {
				v, err := strconv.ParseUint(i, 10, 64)
				if err != nil {
					return 0, fmt.Errorf("Bad CPU int %s: %v", i, err)
				}
				totalClockTicks += v
			}
			return (totalClockTicks * nanoSecondsPerSecond) /
				s.clockTicksPerSecond, nil
		}
	}
	return 0, fmt.Errorf("Bad stat file format")
}

func getContainerResources(id string) (*specs.LinuxResources, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	// check to make sure a container exists with this ID
	statePath := path.Join(abs, id, stateFile)

	// read the state.json for the container so we can find out the bundle path
	f, err := os.Open(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("JSON runtime state file %s not found at %s", stateFile, statePath)
		}
	}
	defer f.Close()

	var state types.State
	if err = json.NewDecoder(f).Decode(&state); err != nil {
		return nil, err
	}

	bundle := searchLabels(state.Config.Labels, "bundle")
	specPath := path.Join(bundle, specFile)

	// read the runtime.json for the container so we know things like limits set
	// this is only if a container ID is not passed we assume we are in a directory
	// with a config.json containing the spec
	f, err = os.Open(specPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("JSON runtime config file %s not found at %s", specFile, specPath)
		}
	}
	defer f.Close()

	var spec specs.Spec
	if err = json.NewDecoder(f).Decode(&spec); err != nil {
		return nil, err
	}
	return spec.Linux.Resources, nil
}

func searchLabels(labels []string, query string) string {
	for _, l := range labels {
		parts := strings.SplitN(l, "=", 2)
		if len(parts) < 2 {
			continue
		}
		if parts[0] == query {
			return parts[1]
		}
	}
	return ""
}
