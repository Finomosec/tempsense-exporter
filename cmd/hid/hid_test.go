package hid

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestParsing(t *testing.T) {
	var buf = []byte{2, 1, 1, 0, 19, 1, 0, 0, 40, 179, 190, 22, 43, 32, 1, 9}
	data, err := parseBuffer(buf, 16)
	assert.NoErrorf(t, err, "there should be no error")
	assert.Equal(t, byte(2), data.SensorCount, "sensors should be 2")
	assert.Equal(t, byte(1), data.SensorCurrent, "current sensor should be 1")
	assert.Equal(t, uint16(275), data.Temp, "temp should be 275")
	assert.Equal(t, float64(27.5), data.Temperature(), "temp should be 27.5")
	assert.Equal(t, "28b3be162b200109", data.SensorsIdHex(), "sensorid should be 28b3be162b200109")
}

func TestOriginalFirmwareHasNoErrorCounters(t *testing.T) {
	buf := make([]byte, 64)
	copy(buf, []byte{2, 1, 1, 0, 19, 1, 0, 0, 40, 179, 190, 22, 43, 32, 1, 9})
	data, err := parseBuffer(buf, 64)
	assert.NoError(t, err)
	assert.False(t, data.HasErrorCounters(), "zero bytes after the header mean original firmware")
	assert.Equal(t, uint16(275), data.Temp)
}

func TestPatchedFirmwareErrorCounters(t *testing.T) {
	buf := make([]byte, 64)
	copy(buf, []byte{6, 4, 1, 12, 0x80, 1, 0, 0, 40, 222, 22, 70, 212, 231, 120, 102, 0x46, 1, 0x2c, 0x01, 7, 0})
	data, err := parseBuffer(buf, 64)
	assert.NoError(t, err)
	assert.True(t, data.HasErrorCounters())
	assert.Equal(t, byte(12), data.Resolution, "resolution in bits")
	assert.Equal(t, uint16(300), data.Retried, "retried reads, little endian")
	assert.Equal(t, uint16(7), data.Failed, "failed reads")
	assert.Equal(t, float64(38.4), data.Temperature())
}

func TestShortRead(t *testing.T) {
	var buf = []byte{2, 1, 1, 0, 19, 1, 0, 0, 40, 179, 190, 22, 43, 32, 1}
	_, err := parseBuffer(buf, 15)
	assert.Error(t, err, "there should be an error")
}

func TestEmptyRead(t *testing.T) {
	var buf = []byte{}
	_, err := parseBuffer(buf, 0)
	assert.Error(t, err, "there should be an error")
}

func TestShortBuffer(t *testing.T) {
	var buf = []byte{2, 1, 1, 0, 19, 1, 0, 0, 40, 179, 190, 22, 43, 32, 1}
	_, err := parseBuffer(buf, 16)
	assert.EqualError(t, err, "unable to decode unexpected EOF", "got wrong error")
}
