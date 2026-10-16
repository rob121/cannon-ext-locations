package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type geocoder interface {
	Lookup(address string) (lat, lng float64, err error)
}

type nominatimGeocoder struct {
	client *http.Client
}

func (g nominatimGeocoder) Lookup(address string) (float64, float64, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return 0, 0, fmt.Errorf("address is required for geocoding")
	}
	client := g.client
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	endpoint := "https://nominatim.openstreetmap.org/search?format=json&limit=1&q=" + url.QueryEscape(address)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("User-Agent", "cannon-ext-locations/0.1.0 (https://github.com/rob121/cannon-ext-locations)")
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("geocoder returned status %d", resp.StatusCode)
	}
	var results []struct {
		Lat string `json:"lat"`
		Lon string `json:"lon"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return 0, 0, err
	}
	if len(results) == 0 {
		return 0, 0, fmt.Errorf("no geocode result for address")
	}
	lat, err := strconv.ParseFloat(results[0].Lat, 64)
	if err != nil {
		return 0, 0, err
	}
	lng, err := strconv.ParseFloat(results[0].Lon, 64)
	if err != nil {
		return 0, 0, err
	}
	return lat, lng, nil
}
