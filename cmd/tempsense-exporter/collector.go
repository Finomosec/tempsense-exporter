package main

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vgropp/tempsense-exporter/cmd/hid"
	"log"
	"strconv"
	"os"
	"encoding/csv"
	"time"
)

// Definieren Sie eine Struktur für Ihre Daten
type Data struct {
	Nr    string
	ID    string
	Name  string
	Type  string
}

type TempsenseCollector struct {
	tempsenseMetric *prometheus.Desc
	dataMap map[string]Data
	lastModified int64
}

// getLastModified retrieves the last modified time of a given file.
func getLastModified(fileName string) (int64, error) {
	fileInfo, err := os.Stat(fileName)
	if err!= nil {
		return 0, err
	}
	modTime := fileInfo.ModTime()
	return modTime.UnixNano() / int64(time.Millisecond), nil
}

func (collector *TempsenseCollector) readSensorsCsv() error {
	fileName := "sensors.csv"
	lastModifiedFromFile, error := getLastModified(fileName)
	if error != nil {
		fmt.Println("Error checking file: %s: %s", fileName, error)
		return error
	}
	if collector.lastModified!= lastModifiedFromFile {
		collector.lastModified = lastModifiedFromFile
		// Perform some actions here.
		fmt.Println("File has been modified. Reading it...")
	} else {
		fmt.Println("File is unchanged.")
		return nil
	}

	file, err := os.Open(fileName)
	if err!= nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	_, err = reader.Read() // Liest die Kopfzeile
	if err!= nil {
		return err
	}

	collector.dataMap = make(map[string]Data)
	for {
		record, err := reader.Read()
		if err!= nil {
			break
		}
		if len(record) >= 3 {
			id := record[1]
			data := Data{
				Nr:    record[0],
				ID:    id,
				Name:  record[2],
				Type:  record[3],
			}
			collector.dataMap[id] = data
		}
	}
	collector.PrintDataMap()
	return nil
}


func (collector *TempsenseCollector) PrintDataMap() {
	for _, data := range collector.dataMap {
		fmt.Printf("ID: %s, Nr: %s, Name: %s, Type: %s\n", data.ID, data.Nr, data.Name, data.Type)
	}
}

// Hilfsfunktion zum Parsen von Strings in Integer
func parseInt(s string) int {
	result, err := strconv.Atoi(s)
	if err!= nil {
		panic(err)
	}
	return result
}

func NewTempsenseCollector() *TempsenseCollector {
	collector := &TempsenseCollector{
		tempsenseMetric: prometheus.NewDesc("temperature_sensor_celsius",
			"shows current temperature as reported by the ds18b20",
			[]string{"display_name", "id", "sensor_nr", "sensor_type"}, nil,
		),
	}
	fmt.Println("Collector created.")
	return collector
}

func (collector *TempsenseCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.tempsenseMetric
}

func (collector *TempsenseCollector) Collect(ch chan<- prometheus.Metric) {
	devices, err := hid.LookupDevices()
	if err != nil {
		log.Print(err)
		return
	}
	collector.readDevices(ch, devices)
}

func (collector *TempsenseCollector) readDevices(ch chan<- prometheus.Metric, devices *hid.HidDevices) {
	for _, device := range devices.Devices {
		collector.readSensors(device, ch)
	}
}

func (collector *TempsenseCollector) readSensors(device hid.Device, ch chan<- prometheus.Metric) {
	numSens := 0
	for {
		numSens++
		data, err := device.ReadSensor()
		if err != nil {
			fmt.Printf("error reading device %v: %v\n", device.GetNum(), err)
			break
		}
		collector.addToMetric(ch, data, device.GetNum())
		if numSens >= int(data.SensorCount) {
			break
		}
	}
}


func (collector *TempsenseCollector) addToMetric(ch chan<- prometheus.Metric, data *hid.Data, deviceNum int) {
	select {
	case ch <- collector.sendTemperatureMetric(data, deviceNum):
	default:
	}
}

func insertAt(s string, pos int, c string) string {
	if len(s) < pos+1 {
		return s + c + s[len(s):]
	}
	return s[:pos] + c + s[pos:]
}

func (collector *TempsenseCollector) sendTemperatureMetric(data *hid.Data, deviceNum int) prometheus.Metric {
    id := insertAt(data.SensorsIdHex(), 2, "-")
    collector.readSensorsCsv()
    extData, exists := collector.dataMap[id]
    var metric prometheus.Metric
    if exists {
        metric = prometheus.MustNewConstMetric(
            collector.tempsenseMetric,
            prometheus.GaugeValue,
            data.Temperature(),
            extData.Name,
            id,
            extData.Nr,
            extData.Type,
        )
    } else {
        metric = prometheus.MustNewConstMetric(
            collector.tempsenseMetric,
            prometheus.GaugeValue,
            data.Temperature(),
            "Unknown",
            id,
            "0",
            "unknown",
        )
    }
    return metric
}

