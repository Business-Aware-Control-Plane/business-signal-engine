package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	MongoURI        string
	MongoDatabase   string
	RunMode         string // "daemon", "oneshot", or "simulation"
	EnableSimulator bool   // Explicitly enable test stream simulator
	CountryCode     string // e.g. "LK"
	Latitude        float64
	Longitude       float64
	GAPropertyID    string
	MetaAccessToken string
	MetaAdAccountID string
}

func LoadConfig() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("[INFO] No .env file found or error reading .env, reading environment variables")
	}

	mode := getEnv("RUN_MODE", "daemon")
	enableSim := getEnvBool("ENABLE_SIMULATOR", false) || strings.EqualFold(mode, "simulation")

	return &Config{
		MongoURI:        getEnv("MONGODB_URI", "mongodb://localhost:27017"),
		MongoDatabase:   getEnv("MONGODB_DATABASE", "business_signal_engine"),
		RunMode:         mode,
		EnableSimulator: enableSim,
		CountryCode:     getEnv("COUNTRY_CODE", "LK"),
		Latitude:        getEnvFloat("LATITUDE", 6.9271),
		Longitude:       getEnvFloat("LONGITUDE", 79.8612),
		GAPropertyID:    os.Getenv("GA_PROPERTY_ID"),
		MetaAccessToken: os.Getenv("META_ACCESS_TOKEN"),
		MetaAdAccountID: os.Getenv("META_AD_ACCOUNT_ID"),
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	valStr := os.Getenv(key)
	if valStr == "" {
		return fallback
	}
	val, err := strconv.ParseBool(valStr)
	if err != nil {
		return fallback
	}
	return val
}

func getEnvFloat(key string, fallback float64) float64 {
	valStr := os.Getenv(key)
	if valStr == "" {
		return fallback
	}
	val, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return fallback
	}
	return val
}
