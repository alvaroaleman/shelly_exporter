package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/yaml"
)

type opts struct {
	configFile string
}

func main() {
	var o opts
	cmd := &cobra.Command{}
	cmd.Flags().StringVar(&o.configFile, "config", "config.yaml", "Path to the config file")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return run(cmd.Context(), o)
	}

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

type configItem struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

func run(ctx context.Context, o opts) error {
	zapcfg := zap.NewProductionConfig()
	zapcfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	log, err := zapcfg.Build()
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	defer log.Sync()

	cfg, err := os.ReadFile(o.configFile)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", o.configFile, err)
	}

	var config []configItem
	if err := yaml.Unmarshal(cfg, &config); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	powerMetric := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "shelly_power_watts",
			Help: "Current power consumption in watts",
		},
		[]string{"name"},
	)
	voltageMetric := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "shelly_voltage_volts",
			Help: "Current voltage in volts",
		},
		[]string{"name"},
	)
	currentMetric := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "shelly_current_amperes",
			Help: "Current in amperes",
		},
		[]string{"name"},
	)
	temperatureMetric := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "shelly_temperature_celsius",
			Help: "Temperature in degrees Celsius",
		},
		[]string{"name"},
	)
	for _, metric := range []*prometheus.GaugeVec{
		powerMetric,
		voltageMetric,
		currentMetric,
		temperatureMetric,
	} {
		if err := prometheus.Register(metric); err != nil {
			return fmt.Errorf("failed to register metric: %w", err)
		}
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{}))

	server := &http.Server{
		Addr:    ":9090",
		Handler: mux,
	}
	go func() {
		log.Info("Starting server", zap.String("address", server.Addr))
		if err := server.ListenAndServe(); err != nil {
			log.Error("failed to run server", zap.Error(err))
		}
		cancel()
	}()

	for {
		for _, item := range config {
			resp, err := fetch(ctx, item.Address)
			if err != nil {
				log.Error("failed to fetch data", zap.String("name", item.Name), zap.Error(err))
				continue
			}
			log.Info("Successfully fetched data", zap.String("name", item.Name), zap.String("address", item.Address))
			powerMetric.WithLabelValues(item.Name).Set(resp.Power)
			voltageMetric.WithLabelValues(item.Name).Set(resp.Voltage)
			currentMetric.WithLabelValues(item.Name).Set(resp.Current)
			temperatureMetric.WithLabelValues(item.Name).Set(resp.Temperature.Celsius)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(15 * time.Second):
		}
	}
}

func fetch(ctx context.Context, address string) (*shellyResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/rpc/Switch.GetStatus?id=0", address)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", url, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch data from %s: %w", address, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got unexpected status code %d from %s", resp.StatusCode, address)
	}

	var shellyResp shellyResponse
	if err := json.NewDecoder(resp.Body).Decode(&shellyResp); err != nil {
		return nil, fmt.Errorf("failed to decode response from %s: %w", address, err)
	}

	return &shellyResp, nil
}

type shellyResponse struct {
	ID          int         `json:"id"`
	Source      string      `json:"source"`
	Output      bool        `json:"output"`
	Power       float64     `json:"apower"`
	Voltage     float64     `json:"voltage"`
	Current     float64     `json:"current"`
	Aenergy     Aenergy     `json:"aenergy"`
	Temperature Temperature `json:"temperature"`
}
type Aenergy struct {
	Total    float64   `json:"total"`
	ByMinute []float64 `json:"by_minute"`
	MinuteTs int       `json:"minute_ts"`
}
type Temperature struct {
	Celsius    float64 `json:"tC"`
	Fahrenheit float64 `json:"tF"`
}
