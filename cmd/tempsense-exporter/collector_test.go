package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/vgropp/tempsense-exporter/cmd/hid"
)

func sensor(nr byte, decicelsius uint16) *hid.Data {
	return &hid.Data{SensorCount: 2, SensorCurrent: nr, Temp: decicelsius, SensorId: [8]byte{0x28, nr, 0, 0, 0, 0, 0, 0xaa}}
}

var idA, idB = sensorID(sensor(1, 0)), sensorID(sensor(2, 0))

func expectReads(dev *MockDevice, reads ...*hid.Data) {
	for _, r := range reads {
		dev.On("ReadSensor").Return(r, nil).Once()
	}
}

func scrape(t *testing.T, collector *TempsenseCollector, dev *MockDevice) map[string]float64 {
	c := make(chan prometheus.Metric, 10)
	collector.readSensors(dev, c)
	close(c)
	values := make(map[string]float64)
	for m := range c {
		var pb dto.Metric
		require.NoError(t, m.Write(&pb))
		for _, l := range pb.GetLabel() {
			if l.GetName() == "id" {
				values[l.GetValue()] = pb.GetGauge().GetValue()
			}
		}
	}
	return values
}

func TestCallingDevices(t *testing.T) {
	dev := new(MockDevice)
	collector := NewTempsenseCollector()

	expectReads(dev, sensor(2, 250), sensor(1, 270), sensor(2, 251))
	assert.Equal(t, map[string]float64{idA: 27.0, idB: 25.1}, scrape(t, collector, dev))
	dev.AssertNumberOfCalls(t, "ReadSensor", 3)

	expectReads(dev, sensor(1, 270), sensor(2, 252), sensor(1, 271))
	assert.Equal(t, map[string]float64{idA: 27.1, idB: 25.2}, scrape(t, collector, dev))
	dev.AssertNumberOfCalls(t, "ReadSensor", 6)
}

func TestStaleValueIsReplacedByReread(t *testing.T) {
	dev := new(MockDevice)
	collector := NewTempsenseCollector()
	expectReads(dev, sensor(2, 250), sensor(1, 196), sensor(2, 259))
	scrape(t, collector, dev)

	expectReads(dev, sensor(2, 259), sensor(1, 259), sensor(2, 259), sensor(1, 196), sensor(2, 259))
	assert.Equal(t, map[string]float64{idA: 19.6, idB: 25.9}, scrape(t, collector, dev),
		"A repeating B's value is the firmware's stale buffer, not a measurement")
	dev.AssertNumberOfCalls(t, "ReadSensor", 8)
}

func TestRealJumpIsAcceptedWhenConfirmed(t *testing.T) {
	dev := new(MockDevice)
	collector := NewTempsenseCollector()
	expectReads(dev, sensor(2, 250), sensor(1, 200), sensor(2, 250))
	scrape(t, collector, dev)

	expectReads(dev, sensor(2, 250), sensor(1, 230), sensor(2, 251), sensor(1, 231), sensor(2, 251))
	assert.Equal(t, map[string]float64{idA: 23.1, idB: 25.1}, scrape(t, collector, dev))
}

func TestUnconfirmedValueIsDropped(t *testing.T) {
	dev := new(MockDevice)
	collector := NewTempsenseCollector()
	expectReads(dev, sensor(2, 250), sensor(1, 196), sensor(2, 259))
	scrape(t, collector, dev)

	expectReads(dev,
		sensor(2, 259), sensor(1, 259), sensor(2, 259),
		sensor(1, 259), sensor(2, 259),
		sensor(1, 259), sensor(2, 259))
	assert.Equal(t, map[string]float64{idB: 25.9}, scrape(t, collector, dev),
		"a value that stays stale over all re-reads must not be exported")
	dev.AssertNumberOfCalls(t, "ReadSensor", 10)
}

func TestFirmwareInvalidMarkerIsNeverExported(t *testing.T) {
	dev := new(MockDevice)
	collector := NewTempsenseCollector()
	expectReads(dev,
		sensor(2, 250), sensor(1, 2000), sensor(2, 251),
		sensor(1, 2000), sensor(2, 252),
		sensor(1, 2000), sensor(2, 253))

	assert.Equal(t, map[string]float64{idB: 25.1}, scrape(t, collector, dev),
		"200.0°C is the patched firmware's marker for a failed read")
}

func TestSensorsCsvPathIsConfigurable(t *testing.T) {
	csvPath := filepath.Join(t.TempDir(), "sensors.csv")
	require.NoError(t, os.WriteFile(csvPath, []byte("nr,id,name,type,batch\n7,"+idA+",Testraum,room,1\n"), 0o644))
	collector := NewTempsenseCollector()
	collector.csvPath = csvPath
	dev := new(MockDevice)
	expectReads(dev, sensor(2, 250), sensor(1, 270), sensor(2, 251))
	c := make(chan prometheus.Metric, 10)

	collector.readSensors(dev, c)
	close(c)

	names := make(map[string]string)
	for m := range c {
		var pb dto.Metric
		require.NoError(t, m.Write(&pb))
		labels := make(map[string]string)
		for _, l := range pb.GetLabel() {
			labels[l.GetName()] = l.GetValue()
		}
		names[labels["id"]] = labels["display_name"]
	}
	assert.Equal(t, map[string]string{idA: "Testraum", idB: "Unknown"}, names)
}

func TestNoSensorConnected(t *testing.T) {
	dev := new(MockDevice)
	expectReads(dev, &hid.Data{SensorCount: 0})

	assert.Empty(t, scrape(t, NewTempsenseCollector(), dev), "no metric without a connected sensor")
	dev.AssertNumberOfCalls(t, "ReadSensor", 1)
}

func TestReadErrorCountersFromPatchedFirmware(t *testing.T) {
	dev := new(MockDevice)
	withCounters := func(nr byte, decicelsius, retried, failed uint16) *hid.Data {
		d := sensor(nr, decicelsius)
		d.ReportFormat, d.Retried, d.Failed = 1, retried, failed
		return d
	}
	expectReads(dev, withCounters(2, 250, 0, 0), withCounters(1, 270, 3, 1), withCounters(2, 251, 0, 0))

	c := make(chan prometheus.Metric, 10)
	NewTempsenseCollector().readSensors(dev, c)
	close(c)
	errors := make(map[string]float64)
	for m := range c {
		var pb dto.Metric
		require.NoError(t, m.Write(&pb))
		if pb.GetCounter() == nil {
			continue
		}
		labels := map[string]string{}
		for _, l := range pb.GetLabel() {
			labels[l.GetName()] = l.GetValue()
		}
		errors[labels["id"]+"/"+labels["result"]] = pb.GetCounter().GetValue()
	}
	assert.Equal(t, map[string]float64{
		idA + "/retried": 3, idA + "/failed": 1,
		idB + "/retried": 0, idB + "/failed": 0,
	}, errors)
}

func TestNoReadErrorMetricWithOriginalFirmware(t *testing.T) {
	dev := new(MockDevice)
	expectReads(dev, sensor(2, 250), sensor(1, 270), sensor(2, 251))
	c := make(chan prometheus.Metric, 10)
	NewTempsenseCollector().readSensors(dev, c)
	close(c)
	for m := range c {
		var pb dto.Metric
		require.NoError(t, m.Write(&pb))
		assert.Nil(t, pb.GetCounter(), "original firmware sends no counters: %v", m.Desc())
	}
}

type MockDevice struct {
	mock.Mock
}

func (dev *MockDevice) ReadSensor() (*hid.Data, error) {
	args := dev.Called()
	return args.Get(0).(*hid.Data), nil
}

func (dev *MockDevice) GetNum() int {
	return 1
}
