package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	MongoURI             string
	MongoDatabase        string
	RunMode              string // "daemon", "oneshot", or "simulation"
	EnableSimulator      bool   // Explicitly enable test stream simulator
	CountryCode          string // e.g. "LK"
	Latitude             float64
	Longitude            float64
	GAPropertyID         string
	GoogleClientID       string
	GoogleClientSecret   string
	GoogleQuotaProjectID string
	GoogleRefreshToken   string
	MetaAccessToken      string
	MetaAdAccountID      string
	MetaAppID            string
	MetaAppSecret        string
	MetaClientToken      string
	ThreadsAppID         string
	ThreadsAppSecret     string
}

func LoadConfig() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("[INFO] No .env file found or error reading .env, reading environment variables")
	}

	mode := getEnv("RUN_MODE", "daemon")
	enableSim := getEnvBool("ENABLE_SIMULATOR", false) || strings.EqualFold(mode, "simulation")

	metaAppID := os.Getenv("META_APP_ID")
	metaAppSecret := os.Getenv("META_APP_SECRET")
	metaToken := os.Getenv("META_ACCESS_TOKEN")

	// If explicit user access token is unset, fallback to App Access Token format (AppID|AppSecret)
	if metaToken == "" && metaAppID != "" && metaAppSecret != "" {
		metaToken = fmt.Sprintf("%s|%s", metaAppID, metaAppSecret)
	}

	return &Config{
		MongoURI:             getEnv("MONGODB_URI", "mongodb://localhost:27017"),
		MongoDatabase:        getEnv("MONGODB_DATABASE", "business_signal_engine"),
		RunMode:              mode,
		EnableSimulator:      enableSim,
		CountryCode:          getEnv("COUNTRY_CODE", "LK"),
		Latitude:             getEnvFloat("LATITUDE", 6.9271),
		Longitude:            getEnvFloat("LONGITUDE", 79.8612),
		GAPropertyID:         os.Getenv("GA_PROPERTY_ID"),
		GoogleClientID:       os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret:   os.Getenv("GOOGLE_CLIENT_SECRET"),
		GoogleQuotaProjectID: os.Getenv("GOOGLE_QUOTA_PROJECT_ID"),
		GoogleRefreshToken:   os.Getenv("GOOGLE_REFRESH_TOKEN"),
		MetaAccessToken:      metaToken,
		MetaAdAccountID:      os.Getenv("META_AD_ACCOUNT_ID"),
		MetaAppID:            metaAppID,
		MetaAppSecret:        metaAppSecret,
		MetaClientToken:      os.Getenv("META_CLIENT_TOKEN"),
		ThreadsAppID:         os.Getenv("THREADS_APP_ID"),
		ThreadsAppSecret:     os.Getenv("THREADS_APP_SECRET"),
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
