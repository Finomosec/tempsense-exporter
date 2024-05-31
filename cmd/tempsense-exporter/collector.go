package main

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vgropp/tempsense-exporter/cmd/hid"
	"log"
	"strconv"
	"strings"
)

// Definieren Sie eine Struktur für Ihre Daten
type Data struct {
	Nr    string
	ID    string
	Name  string
	Type  string
}

func (collector *TempsenseCollector) init() {
	// Ihr Datenstring
	dataString := `nr,id,name,type
1,543854836853,Garten,outdoors`

	// Trennen Sie den String nach Zeilenumbrüchen
	lines := strings.Split(dataString, "\n")

	// Erstellen Sie ein Map, um auf die Daten basierend auf der ID zuzugreifen
	collector.dataMap = make(map[string]Data)
	for i, line := range lines {
		if i == 0 || len(line) == 0 { // Überprüfen, ob die Zeile nicht leer ist
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) >= 3 { // Stellen sicher, dass genügend Teile vorhanden sind
			id := parts[1]
			data := Data{
				Nr:    parts[0],
				ID:    id,
				Name:  parts[2],
				Type:  parts[3],
			}
			collector.dataMap[id] = data
		}
	}

	// Zugriff auf die Daten basierend auf der ID
	// fmt.Println("Daten für ID 543854836853:")
	// daten := collector.dataMap["543854836853"]
	// fmt.Printf("%+v\n", daten)
}

// Hilfsfunktion zum Parsen von Strings in Integer
func parseInt(s string) int {
	result, err := strconv.Atoi(s)
	if err!= nil {
		panic(err)
	}
	return result
}


type TempsenseCollector struct {
	tempsenseMetric *prometheus.Desc
	dataMap map[string]Data
}

func NewTempsenseCollector() *TempsenseCollector {
	return &TempsenseCollector{
		tempsenseMetric: prometheus.NewDesc("temperature_sensor_celsius",
			"shows current temperature as reported by the ds18b20",
			[]string{"display_name", "id", "sensor_nr", "sensor_type"}, nil,
		),
	}
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

