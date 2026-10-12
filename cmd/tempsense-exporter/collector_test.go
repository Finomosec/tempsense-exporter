package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/vgropp/tempsense-exporter/cmd/hid"
	"testing"
)

func TestCallingDevices(t *testing.T) {
	testDevice := new(MockDevice)
	sensorA := &hid.Data{SensorCount: 2, SensorCurrent: 2, Temp: 270, SensorId: [8]byte{1, 2, 3, 4, 5, 6, 7, 8}}
	sensorB := &hid.Data{SensorCount: 2, SensorCurrent: 1, Temp: 270, SensorId: [8]byte{1, 2, 3, 4, 5, 6, 7, 7}}
	expectCycle := func() {
		testDevice.On("ReadSensor").Return(sensorA, nil).Once()
		testDevice.On("ReadSensor").Return(sensorB, nil).Once()
	}
	c := make(chan prometheus.Metric, 10)
	collector := NewTempsenseCollector()

	expectCycle()
	expectCycle()
	collector.readSensors(testDevice, c)
	testDevice.AssertNumberOfCalls(t, "ReadSensor", 4)
	assert.Len(t, c, 2, "one metric per sensor after warmup read")

	expectCycle()
	collector.readSensors(testDevice, c)
	testDevice.AssertNumberOfCalls(t, "ReadSensor", 6)
	assert.Len(t, c, 4, "known sensors are read once without warmup")
}

func TestNoSensorConnected(t *testing.T) {
	testDevice := new(MockDevice)
	testDevice.On("ReadSensor").Return(&hid.Data{SensorCount: 0}, nil).Once()
	c := make(chan prometheus.Metric, 10)

	NewTempsenseCollector().readSensors(testDevice, c)

	testDevice.AssertNumberOfCalls(t, "ReadSensor", 1)
	assert.Empty(t, c, "no metric without a connected sensor")
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
