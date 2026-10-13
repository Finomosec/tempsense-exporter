package main

import (
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vgropp/tempsense-exporter/cmd/hid"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"sync"
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
	csvPath      string
	history       map[string]*sensorHistory
	mu            sync.Mutex
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

func convertAddress(input string) string {
	// Entfernen des Präfixes "28-" und Sufix "aa", falls vorhanden
	prefix := ""
	// if input[:3] == "28-" && input[len(input)-2:] == "aa" {
	if input[:3] == "28-" && len(input) == 17 {
		prefix = "28-"
		input = input[3 : len(input)-2]
	} else {
		fmt.Println("wrong input: " + input)
		return input
	}

	// Umwandlung des Hexadezimalstrings in Bytes
	data, err := hex.DecodeString(input)
	if err!= nil {
		panic(err)
	}

	// Umkehrung der Bytes
	reversedData := make([]byte, len(data))
	for i, v := range data {
		reversedData[len(data)-i-1] = v
	}

	// Umwandlung der umgekehrten Bytes zurück in einen Hexadezimalstring
	reversedHexString := hex.EncodeToString(reversedData)

	// Zusammenfügen des Präfixes und der umgekehrten Hexadezimal-Zeichenkette
	result := prefix + reversedHexString

	return result
}

func (collector *TempsenseCollector) sensorsCsvPath() (string, error) {
	if collector.csvPath != "" {
		return collector.csvPath, nil
	}
	ex, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(ex), "..", "cfg", "sensors.csv"), nil
}

func (collector *TempsenseCollector) readSensorsCsv() error {
	fileName, err := collector.sensorsCsvPath()
	if err != nil {
		return err
	}
	lastModifiedFromFile, error := getLastModified(fileName)
	if error != nil {
		fmt.Printf("Error checking file: %s: %s\n", fileName, error)
		return error
	}
	if collector.lastModified!= lastModifiedFromFile {
		collector.lastModified = lastModifiedFromFile
		// Perform some actions here.
		fmt.Printf("File has been modified. Reading it: %s\n", fileName)
	} else {
		// fmt.Println("File is unchanged: %s", fileName)
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
		history: make(map[string]*sensorHistory),
	}
	fmt.Println("Collector created.")
	return collector
}

func (collector *TempsenseCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.tempsenseMetric
}

func (collector *TempsenseCollector) Collect(ch chan<- prometheus.Metric) {
	collector.mu.Lock()
	defer collector.mu.Unlock()
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

func sensorID(data *hid.Data) string {
	return convertAddress(insertAt(data.SensorsIdHex(), 2, "-"))
}

func (collector *TempsenseCollector) readSensors(device hid.Device, ch chan<- prometheus.Metric) {
	pass, prev := readPass(device, nil)
	var order []string
	accepted := make(map[string]*hid.Data)
	candidates := make(map[string][]float64)
	pending := make(map[string]bool)
	for _, r := range pass {
		order = append(order, r.id)
		collector.judge(r, accepted, candidates, pending)
	}
	for i := 0; i < maxRereads && len(pending) > 0 && prev != nil; i++ {
		var retry []reading
		retry, prev = readPass(device, prev)
		for _, r := range retry {
			if pending[r.id] {
				collector.judge(r, accepted, candidates, pending)
			}
		}
	}
	for id := range pending {
		log.Printf("dropping sensor %s: unconfirmed values %v", id, candidates[id])
	}
	for _, id := range order {
		data, ok := accepted[id]
		if !ok {
			continue
		}
		collector.historyOf(id).add(data.Temperature())
		collector.addToMetric(ch, data, device.GetNum())
	}
}

func (collector *TempsenseCollector) historyOf(id string) *sensorHistory {
	h, exists := collector.history[id]
	if !exists {
		h = &sensorHistory{}
		collector.history[id] = h
	}
	return h
}

func (collector *TempsenseCollector) judge(r reading, accepted map[string]*hid.Data, candidates map[string][]float64, pending map[string]bool) {
	t := r.data.Temperature()
	if !isSuspicious(r, collector.historyOf(r.id)) {
		accepted[r.id] = r.data
		delete(pending, r.id)
		return
	}
	if !r.stale {
		for _, c := range candidates[r.id] {
			if math.Abs(c-t) <= confirmTolerance {
				log.Printf("sensor %s: jump to %.1f\u00b0C confirmed", r.id, t)
				accepted[r.id] = r.data
				delete(pending, r.id)
				return
			}
		}
		candidates[r.id] = append(candidates[r.id], t)
	}
	log.Printf("sensor %s: suspicious %.1f\u00b0C (stale=%v), re-reading", r.id, t, r.stale)
	pending[r.id] = true
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
    id := convertAddress(insertAt(data.SensorsIdHex(), 2, "-"))
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
