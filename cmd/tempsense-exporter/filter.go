package main

import (
	"fmt"
	"math"
	"sort"

	"github.com/vgropp/tempsense-exporter/cmd/hid"
)

const (
	historySize       = 5
	maxStep           = 1.5
	minStaleDeviation = 0.15
	confirmTolerance  = 0.3
	maxRereads        = 2

	firmwareInvalidDecicelsius = 2000
)

type reading struct {
	data  *hid.Data
	id    string
	stale bool
}

type sensorHistory struct {
	values []float64
}

func (h *sensorHistory) add(v float64) {
	h.values = append(h.values, v)
	if len(h.values) > historySize {
		h.values = h.values[1:]
	}
}

func (h *sensorHistory) median() (float64, bool) {
	if len(h.values) == 0 {
		return 0, false
	}
	sorted := append([]float64(nil), h.values...)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2], true
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2, true
}

func isSuspicious(r reading, h *sensorHistory) bool {
	m, ok := h.median()
	if !ok {
		return r.stale
	}
	deviation := math.Abs(r.data.Temperature() - m)
	return deviation > maxStep || (r.stale && deviation > minStaleDeviation)
}

func readPass(device hid.Device, prev *hid.Data) ([]reading, *hid.Data) {
	if prev == nil {
		data, err := device.ReadSensor()
		if err != nil {
			fmt.Printf("error reading device %v: %v\n", device.GetNum(), err)
			return nil, nil
		}
		if data.SensorCount == 0 {
			return nil, nil
		}
		prev = data
	}
	var pass []reading
	for {
		data, err := device.ReadSensor()
		if err != nil {
			fmt.Printf("error reading device %v: %v\n", device.GetNum(), err)
			break
		}
		if data.SensorCount == 0 {
			break
		}
		unusable := data.Temp == prev.Temp || data.Temp == firmwareInvalidDecicelsius
		pass = append(pass, reading{data: data, id: sensorID(data), stale: unusable})
		prev = data
		if len(pass) >= int(data.SensorCount) {
			break
		}
	}
	return pass, prev
}
