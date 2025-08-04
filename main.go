package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type IPMIConfig struct {
	Host     string
	Username string
	Password string
	Port     int
}

type SensorData struct {
	Name   string
	ID     string
	Status string
	Entity string
	Value  float64
	Unit   string
	Type   string
}

var (
	voltageGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ipmi_voltage_volts",
			Help: "IPMI voltage sensor readings in volts",
		},
		[]string{"sensor_name", "sensor_id", "host"},
	)

	temperatureGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ipmi_temperature_celsius",
			Help: "IPMI temperature sensor readings in celsius",
		},
		[]string{"sensor_name", "sensor_id", "host"},
	)

	fanGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ipmi_fan_speed_rpm",
			Help: "IPMI fan speed sensor readings in RPM",
		},
		[]string{"sensor_name", "sensor_id", "host"},
	)

	powerGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ipmi_power_watts",
			Help: "IPMI power sensor readings in watts",
		},
		[]string{"sensor_name", "sensor_id", "host"},
	)

	currentGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ipmi_current_amperes",
			Help: "IPMI current sensor readings in amperes",
		},
		[]string{"sensor_name", "sensor_id", "host"},
	)
)

func init() {
	prometheus.MustRegister(voltageGauge)
	prometheus.MustRegister(temperatureGauge)
	prometheus.MustRegister(fanGauge)
	prometheus.MustRegister(powerGauge)
	prometheus.MustRegister(currentGauge)
}

func getIPMIConfig() IPMIConfig {
	host := os.Getenv("IPMI_HOST")
	username := os.Getenv("IPMI_USERNAME")
	password := os.Getenv("IPMI_PASSWORD")
	if host == "" || username == "" || password == "" {
		log.Fatal("IPMI_HOST, IPMI_USERNAME, and IPMI_PASSWORD environment variables must be set")
	}
	return IPMIConfig{
		Host:     host,
		Username: username,
		Password: password,
		Port:     623,
	}
}

func executeIPMICommand(config IPMIConfig) (string, error) {
	cmd := exec.Command("ipmitool",
		"-I", "lanplus",
		"-H", config.Host,
		"-U", config.Username,
		"-P", config.Password,
		"sdr", "elist", "full")

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to execute ipmitool command: %v", err)
	}

	return string(output), nil
}

func parseSensorData(sdrData string) []SensorData {
	var sensors []SensorData
	lines := strings.Split(sdrData, "\n")

	sensorRegex := regexp.MustCompile(`^([^|]+)\s*\|\s*([^|]+)\s*\|\s*(\w+)\s*\|\s*([^|]+)\s*\|\s*(.+)$`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		matches := sensorRegex.FindStringSubmatch(line)
		if len(matches) != 6 {
			continue
		}

		name := strings.TrimSpace(matches[1])
		id := strings.TrimSpace(matches[2])
		status := strings.TrimSpace(matches[3])
		entity := strings.TrimSpace(matches[4])
		valueStr := strings.TrimSpace(matches[5])

		if status != "ok" {
			continue
		}

		if strings.Contains(valueStr, "No Reading") {
			continue
		}

		value, unit, sensorType := parseValue(valueStr)
		if value == 0 && unit == "" {
			continue
		}

		sensors = append(sensors, SensorData{
			Name:   name,
			ID:     id,
			Status: status,
			Entity: entity,
			Value:  value,
			Unit:   unit,
			Type:   sensorType,
		})
	}

	return sensors
}

func parseValue(valueStr string) (float64, string, string) {
	valueStr = strings.TrimSpace(valueStr)

	if strings.Contains(valueStr, "Volts") {
		parts := strings.Fields(valueStr)
		if len(parts) >= 1 {
			if val, err := strconv.ParseFloat(parts[0], 64); err == nil {
				return val, "volts", "voltage"
			}
		}
	}

	if strings.Contains(valueStr, "degrees C") {
		parts := strings.Fields(valueStr)
		if len(parts) >= 1 {
			if val, err := strconv.ParseFloat(parts[0], 64); err == nil {
				return val, "celsius", "temperature"
			}
		}
	}

	if strings.Contains(valueStr, "RPM") {
		parts := strings.Fields(valueStr)
		if len(parts) >= 1 {
			if val, err := strconv.ParseFloat(parts[0], 64); err == nil {
				return val, "rpm", "fan"
			}
		}
	}

	if strings.Contains(valueStr, "Watts") {
		parts := strings.Fields(valueStr)
		if len(parts) >= 1 {
			if val, err := strconv.ParseFloat(parts[0], 64); err == nil {
				return val, "watts", "power"
			}
		}
	}

	if strings.Contains(valueStr, "Amps") {
		parts := strings.Fields(valueStr)
		if len(parts) >= 1 {
			if val, err := strconv.ParseFloat(parts[0], 64); err == nil {
				return val, "amperes", "current"
			}
		}
	}

	return 0, "", ""
}

func updateMetrics(sensors []SensorData, host string) {
	for _, sensor := range sensors {
		switch sensor.Type {
		case "voltage":
			voltageGauge.WithLabelValues(sensor.Name, sensor.ID, host).Set(sensor.Value)
		case "temperature":
			temperatureGauge.WithLabelValues(sensor.Name, sensor.ID, host).Set(sensor.Value)
		case "fan":
			fanGauge.WithLabelValues(sensor.Name, sensor.ID, host).Set(sensor.Value)
		case "power":
			powerGauge.WithLabelValues(sensor.Name, sensor.ID, host).Set(sensor.Value)
		case "current":
			currentGauge.WithLabelValues(sensor.Name, sensor.ID, host).Set(sensor.Value)
		}
	}
}

func collectMetrics(config IPMIConfig) {
	output, err := executeIPMICommand(config)
	if err != nil {
		log.Printf("Failed to execute IPMI command: %v", err)
		return
	}

	sensors := parseSensorData(output)
	updateMetrics(sensors, config.Host)
	log.Printf("Updated %d sensor metrics", len(sensors))
}

func startMetricsCollection(config IPMIConfig) {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for {
			collectMetrics(config)
			<-ticker.C
		}
	}()

	collectMetrics(config)
}

func main() {
	fmt.Println("IPMI Prometheus Exporter starting...")

	config := getIPMIConfig()
	log.Printf("Connecting to IPMI host: %s", config.Host)

	startMetricsCollection(config)

	http.Handle("/metrics", promhttp.Handler())

	log.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
