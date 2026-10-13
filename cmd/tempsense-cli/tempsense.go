package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/vgropp/tempsense-exporter/cmd/hid"
)

const invalidDecicelsius = 2000

// deviceState tracks the sensors of one stick to report plugged and unplugged sensors.
type deviceState struct {
	known     map[string]bool
	round     map[string]bool
	fullRound bool
	empty     bool
	prev      *hid.Data
	started   bool
}

// failing suppresses repeated error messages until a read succeeds again.
var failing bool

func main() {
	states := make(map[int]*deviceState)
	for {
		devices := waitForDevices()
		for {
			ok := true
			for _, device := range devices.Devices {
				state, exists := states[device.Num]
				if !exists {
					state = &deviceState{known: map[string]bool{}, round: map[string]bool{}}
					states[device.Num] = state
				}
				if !readNextSensor(device, state) {
					ok = false
				}
			}
			if !ok {
				time.Sleep(time.Second)
				break
			}
		}
	}
}

func waitForDevices() *hid.HidDevices {
	for {
		devices, err := hid.LookupDevices()
		if err == nil {
			if !failing {
				for i, device := range devices.Devices {
					fmt.Printf("device found %v: %s[%s]\n", i+1, device.DeviceInfo.Manufacturer, device.DeviceInfo.Product)
				}
			}
			return devices
		}
		if !failing {
			fmt.Printf("%v: %v, waiting for a device\n", stamp(), err)
			failing = true
		}
		time.Sleep(2 * time.Second)
	}
}

func stamp() string {
	return time.Now().Format(time.Stamp)
}

func readNextSensor(device hid.HidDevice, state *deviceState) bool {
	data, err := device.ReadSensor()
	if err != nil {
		if !failing {
			fmt.Printf("%v: error reading device %v: %v (in use by the exporter or unplugged?), retrying\n", stamp(), device.Num, err)
			failing = true
		}
		return false
	}
	if failing {
		fmt.Printf("%v: device %v readable again\n", stamp(), device.Num)
		failing = false
	}
	if data.SensorCount == 0 {
		if !state.empty {
			fmt.Printf("%v: device %v/%v: no sensors connected\n", stamp(), device.Num, device.DeviceCount)
			reportChanges(state, map[string]bool{})
			state.empty = true
		}
		state.prev = nil
		return true
	}
	state.empty = false

	id := data.LinuxID()
	if data.SensorCurrent == 1 {
		state.round = map[string]bool{}
		state.fullRound = true
	}
	if state.started && !state.known[id] {
		fmt.Printf("%v: + new sensor %v\n", stamp(), id)
		state.known[id] = true
	}
	state.round[id] = true
	fmt.Println(formatReport(device, data, state.prev))
	state.prev = data

	if data.SensorCurrent >= data.SensorCount {
		if state.fullRound {
			reportChanges(state, state.round)
		}
		state.round = map[string]bool{}
		state.fullRound = false
	}
	return true
}

// reportChanges compares the sensors of a finished round with the known ones.
func reportChanges(state *deviceState, round map[string]bool) {
	if !state.started {
		for id := range round {
			state.known[id] = true
		}
		state.started = true
		return
	}
	var gone []string
	for id := range state.known {
		if !round[id] {
			gone = append(gone, id)
		}
	}
	sort.Strings(gone)
	for _, id := range gone {
		fmt.Printf("%v: - sensor %v removed\n", stamp(), id)
		delete(state.known, id)
	}
}

func formatReport(device hid.HidDevice, data *hid.Data, prev *hid.Data) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%v: device %v/%v, sensor %v/%v (%v; %v", stamp(), device.Num, device.DeviceCount,
		data.SensorCurrent, data.SensorCount, data.SensorsIdHex(), powerName(data.Power))
	if data.Resolution > 0 {
		fmt.Fprintf(&b, ", %v bit", data.Resolution)
	}
	fmt.Fprintf(&b, ") %v: ", data.LinuxID())
	if data.HasErrorCounters() && data.Temp == invalidDecicelsius {
		b.WriteString("read failed")
	} else {
		fmt.Fprintf(&b, "%.1f°", data.Temperature())
	}
	if data.HasErrorCounters() {
		fmt.Fprintf(&b, "  errors: %v retried, %v failed", data.Retried, data.Failed)
	} else if prev != nil && prev.Temp == data.Temp {
		b.WriteString("  (same as previous sensor, possibly a read error)")
	}
	return b.String()
}

func powerName(power byte) string {
	if int(power) < len(hid.PowerStr) {
		return hid.PowerStr[power]
	}
	return "unknown"
}
