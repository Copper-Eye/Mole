package main

import (
	"context"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

func collectTopProcesses() []ProcessInfo {
	if runtime.GOOS != "darwin" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Use ps to get top processes by CPU.
	// Use -Aeo instead of -Aceo to get full command line (command) instead of just name (comm).
	out, err := runCmd(ctx, "ps", "-Aeo", "pcpu,pmem,command", "-r")
	if err != nil {
		return nil
	}

	var procMap = make(map[string]*ProcessInfo)

	i := 0
	for line := range strings.Lines(strings.TrimSpace(out)) {
		if i == 0 {
			i++
			continue
		}
		i++
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		cpuVal, _ := strconv.ParseFloat(fields[0], 64)
		memVal, _ := strconv.ParseFloat(fields[1], 64)

		// Join all remaining fields to reconstruct the command/name.
		name := strings.Join(fields[2:], " ")

		// Clean up the name.
		// If it's a path, take the base.
		// If it has arguments (spaces), we might want to keep some context or just take the binary name.
		// Strategies:
		// 1. If it starts with / or ./, it's a path.
		// 2. If it is "Google Chrome Helper", keep it?

		// Simple approach: Take the first component of the command (the binary) and basename it.
		// But "Google Chrome Helper" is "Google Chrome Helper (GPU)" etc.
		// Let's take the basename of the first chunk if it looks like a path?
		// Or better: Aggregating by the "App Name" is hard without heuristics.

		// If we joined everything, we have "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome --type=renderer ..."
		// We want "Google Chrome" preferably.

		// Heuristic:
		// 1. If path points to .app/Contents/MacOS/Binary, use .app name.
		if idx := strings.Index(name, ".app/Contents/MacOS/"); idx >= 0 {
			// Extract App Name from path: .../AppName.app/Contents/MacOS/...
			// Find list index of .app
			prefix := name[:idx]
			if slash := strings.LastIndex(prefix, "/"); slash >= 0 {
				name = prefix[slash+1:] // "Google Chrome"
			}
		} else {
			// Just use the binary name (first field of the command section)
			// But wait, "Google Chrome Helper" might be the binary name on disk?
			// Actually "Google Chrome Helper" is the app name in Activity Monitor, but on disk it is ".../Google Chrome Helper.app/Contents/MacOS/Google Chrome Helper"

			// Let's just strip path from the whole string? No, that's messy.
			// Let's strip path from the first token.
			cmdParts := strings.Fields(name)
			binary := cmdParts[0]
			if idx := strings.LastIndex(binary, "/"); idx >= 0 {
				binary = binary[idx+1:]
			}
			// If the binary is "Google", "Chrome", "Helper", that's not great.
			// But for "Google Chrome Helper", the command might be that.

			// Let's just use the logic:
			// If it contains .app, try to extract app name.
			// Else, use the first word of the command (basename).
			name = binary
		}

		// Clean common noise
		name = strings.TrimSuffix(name, ":")

		if existing, ok := procMap[name]; ok {
			existing.CPU += cpuVal
			existing.Memory += memVal
		} else {
			procMap[name] = &ProcessInfo{
				Name:   name,
				CPU:    cpuVal,
				Memory: memVal,
			}
		}
	}

	var allProcs []ProcessInfo
	for _, p := range procMap {
		allProcs = append(allProcs, *p)
	}

	// Sort by CPU descending
	sort.Slice(allProcs, func(i, j int) bool {
		return allProcs[i].CPU > allProcs[j].CPU
	})

	if len(allProcs) > 5 {
		return allProcs[:5]
	}
	return allProcs
}
