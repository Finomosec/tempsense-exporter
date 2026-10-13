package hid

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/karalabe/hid"
)

const (
	reportLen    = 64
	headerLen    = 16
	extensionLen = 22
	reportMarker = 0x46
)

type header struct {
	SensorCount   byte
	SensorCurrent byte
	Power         byte
	Resolution    byte
	Temp          uint16
	_             uint16
	SensorId      [8]byte
}

// Data is one sensor report. Resolution and the read error counters are only
// sent by the patched firmware (diamex-temp-sensor-firmware); HasErrorCounters
// tells whether they are present.
type Data struct {
	SensorCount   byte
	SensorCurrent byte
	Power         byte
	Resolution    byte
	Temp          uint16
	SensorId      [8]byte
	ReportFormat  byte
	Retried       uint16
	Failed        uint16
}

func (data Data) HasErrorCounters() bool {
	return data.ReportFormat >= 1
}

var PowerStr = []string{"parasite", "extern"}

type HidDevices struct {
	Devices []HidDevice
}

type HidDevice struct {
	DeviceInfo  hid.DeviceInfo
	Num         int
	DeviceCount int
}

type Device interface {
	ReadSensor() (*Data, error)
	GetNum() int
}

func (data Data) SensorsIdHex() string {
	return hex.EncodeToString(data.SensorId[:])
}

// LinuxID returns the id as the Linux w1 driver names it (28-<serial reversed>), as used in sensors.csv.
func (data Data) LinuxID() string {
	b := data.SensorId
	return fmt.Sprintf("%02x-%02x%02x%02x%02x%02x%02x", b[0], b[6], b[5], b[4], b[3], b[2], b[1])
}

func (data Data) Temperature() float64 {
	return float64(data.Temp) / 10
}

func LookupDevices() (*HidDevices, error) {
	devices := hid.Enumerate(0x16c0, 0x0480)
	if len(devices) == 0 {
		return nil, fmt.Errorf("no temperature sensor found")
	}
	var hidDevices []HidDevice
	for i, devInfo := range devices {
		hidDevices = append(hidDevices, HidDevice{DeviceInfo: devInfo, Num: i + 1, DeviceCount: len(devices)})
	}
	return &HidDevices{
		Devices: hidDevices,
	}, nil
}

func (dev HidDevice) ReadSensor() (*Data, error) {
	openDevice, err := dev.DeviceInfo.Open()
	if err != nil {
		return nil, fmt.Errorf("unable to open device %v", err)
	}
	defer func(openDevice *hid.Device) {
		err := openDevice.Close()
		if err != nil {
			panic("error closing device")
		}
	}(openDevice)

	buf, read, err := readToBuffer(openDevice)
	if err != nil {
		return nil, err
	}
	return parseBuffer(buf, read)
}

func (dev HidDevice) GetNum() int {
	return dev.Num
}

func parseBuffer(buf []byte, read int) (*Data, error) {
	if read < 0 {
		return nil, fmt.Errorf("read nothing from device")
	}
	if read < headerLen {
		return nil, fmt.Errorf("read only %v bytes", read)
	}
	data, err := decode(buf[:min(read, len(buf))])
	if err != nil {
		return nil, fmt.Errorf("unable to decode %v", err)
	}
	return data, nil
}

func readToBuffer(openDevice *hid.Device) ([]byte, int, error) {
	buf := make([]byte, reportLen)
	read, err := openDevice.Read(buf)
	if err != nil {
		return nil, 0, fmt.Errorf("could not read from device: %v", err)
	}
	return buf, read, nil
}

func decode(b []byte) (*Data, error) {
	h := header{}
	if err := binary.Read(bytes.NewBuffer(b), binary.LittleEndian, &h); err != nil {
		return nil, err
	}
	data := &Data{
		SensorCount:   h.SensorCount,
		SensorCurrent: h.SensorCurrent,
		Power:         h.Power,
		Resolution:    h.Resolution,
		Temp:          h.Temp,
		SensorId:      h.SensorId,
	}
	if len(b) >= extensionLen && b[headerLen] == reportMarker {
		data.ReportFormat = b[headerLen+1]
		data.Retried = binary.LittleEndian.Uint16(b[headerLen+2:])
		data.Failed = binary.LittleEndian.Uint16(b[headerLen+4:])
	}
	return data, nil
}
