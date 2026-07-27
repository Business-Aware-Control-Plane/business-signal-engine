package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	MongoURI        string
	MongoDatabase   string
	RunMode         string // "daemon" or "oneshot"
	CountryCode     string // e.g. "LK"
	Latitude        float64
	Longitude       float64
	GAPropertyID    string
	MetaAccessToken string
	MetaAdAccountID string
}

func LoadConfig() *Config {
	// Attempt to load .env file if available
	if err := godotenv.Load(); err != nil {
		log.Println("[INFO] No .env file found or error reading .env, reading environment variables")
	}

	return &Config{
		MongoURI:        getEnv("MONGODB_URI", "mongodb://localhost:27017"),
		MongoDatabase:   getEnv("MONGODB_DATABASE", "business_signal_engine"),
		RunMode:         getEnv("RUN_MODE", "daemon"),
		CountryCode:     getEnv("COUNTRY_CODE", "LK"), // Default Sri Lanka
		Latitude:        getEnvFloat("LATITUDE", 6.9271),  // Colombo latitude
		Longitude:       getEnvFloat("LONGITUDE", 79.8612), // Colombo longitude
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
