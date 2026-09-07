package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

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
	MetaPageID           string
	ThreadsAppID         string
	ThreadsAppSecret     string
	BusinessCalendarID   string
	GeminiAPIKey         string
	GeminiModel          string
	PrometheusURL        string
	RabbitMQURI          string
	RabbitMQExchange     string
	RabbitMQQueue        string
	StripeWebhookSecret  string
	StripeSecretKey      string
	StripeBaseURL        string
	WebhookListenAddr    string
	WeatherBaseURL       string
	MetaGraphBaseURL     string
	CalendarBaseURL      string
	GAEndpoint           string
	CalendarAPIEndpoint  string

	// Poll interval overrides (SIM-HB-01 §10). Zero means "use the provider's
	// own production default" — production behaviour is unchanged unless one
	// of these is explicitly set, which the scenario conductor does for the
	// duration of an evaluation run to compress real-world cadence (15-30 min)
	// into a runnable timeline (seconds).
	GAPollInterval                   time.Duration
	MetaPollInterval                 time.Duration
	WeatherPollInterval              time.Duration
	CalendarPollInterval             time.Duration
	PrometheusPollInterval           time.Duration
	SocialMediaPollInterval          time.Duration
	BusinessCalendarPollInterval     time.Duration
	StripeReconciliationPollInterval time.Duration
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
		MetaPageID:           os.Getenv("META_PAGE_ID"),
		ThreadsAppID:         os.Getenv("THREADS_APP_ID"),
		ThreadsAppSecret:     os.Getenv("THREADS_APP_SECRET"),
		BusinessCalendarID:   os.Getenv("BUSINESS_CALENDAR_ID"),
		GeminiAPIKey:         os.Getenv("GEMINI_API_KEY"),
		GeminiModel:          getEnv("GEMINI_MODEL", "gemini-2.5-flash"),
		PrometheusURL:        getEnv("PROMETHEUS_URL", "http://localhost:9090"),
		RabbitMQURI:          getEnv("RABBITMQ_URI", "amqp://guest:guest@localhost:5672/"),
		RabbitMQExchange:     getEnv("RABBITMQ_EXCHANGE", "business_events_exchange"),
		RabbitMQQueue:        getEnv("RABBITMQ_QUEUE", "business_events"),
		WeatherBaseURL:       getEnv("WEATHER_BASE_URL", "https://api.open-meteo.com"),
		MetaGraphBaseURL:     getEnv("META_GRAPH_BASE_URL", "https://graph.facebook.com/v19.0"),
		CalendarBaseURL:      getEnv("CALENDAR_BASE_URL", "https://date.nager.at"),
		GAEndpoint:           os.Getenv("GA_ENDPOINT"),
		CalendarAPIEndpoint:  os.Getenv("CALENDAR_API_ENDPOINT"),
		StripeWebhookSecret:  os.Getenv("STRIPE_WEBHOOK_SECRET"),
		StripeSecretKey:      os.Getenv("STRIPE_SECRET_KEY"),
		StripeBaseURL:        getEnv("STRIPE_BASE_URL", "https://api.stripe.com/v1"),
		WebhookListenAddr:    getEnv("WEBHOOK_LISTEN_ADDR", ":8090"),

		GAPollInterval:                   getEnvDuration("GA_POLL_INTERVAL", 0),
		MetaPollInterval:                 getEnvDuration("META_POLL_INTERVAL", 0),
		WeatherPollInterval:              getEnvDuration("WEATHER_POLL_INTERVAL", 0),
		CalendarPollInterval:             getEnvDuration("CALENDAR_POLL_INTERVAL", 0),
		PrometheusPollInterval:           getEnvDuration("PROMETHEUS_POLL_INTERVAL", 0),
		SocialMediaPollInterval:          getEnvDuration("SOCIAL_MEDIA_POLL_INTERVAL", 0),
		BusinessCalendarPollInterval:     getEnvDuration("BUSINESS_CALENDAR_POLL_INTERVAL", 0),
		StripeReconciliationPollInterval: getEnvDuration("STRIPE_RECONCILIATION_POLL_INTERVAL", 0),
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

// getEnvDuration parses a Go duration string (e.g. "15s", "2m"). Returning
// the zero value on unset/invalid input is deliberate: every provider
// treats a zero PollInterval as "use my own production default," so an eval
// override only ever narrows behaviour when explicitly set, never silently
// changes production cadence on a typo.
func getEnvDuration(key string, fallback time.Duration) time.Duration {
	valStr := os.Getenv(key)
	if valStr == "" {
		return fallback
	}
	val, err := time.ParseDuration(valStr)
	if err != nil {
		log.Printf("[WARN] [Config] Invalid duration for %s=%q, using default: %v", key, valStr, err)
		return fallback
	}
	return val
}
