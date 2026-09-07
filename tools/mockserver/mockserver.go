// Package mockserver is the schema-faithful fixture server described in
// BSAL-HB-01 §07. It replays each real provider's actual wire format — the
// Meta Graph API insights shape, Open-Meteo's forecast shape, Nager.Date's
// holiday list, the Prometheus HTTP query API, and the GA4 Data API's
// runReport/runRealtimeReport response shape — so provider tests exercise
// the real HTTP-call-and-JSON-decode path instead of bypassing it.
//
// It is deliberately not a scenario-driven business-traffic simulator: its
// only job is "does our parsing code survive contact with a real-shaped
// response," across success, empty-result, malformed-body, and
// provider-specific error cases.
package mockserver

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
)

// Scenario selects which canned response shape every route on the server
// returns. One httptest.Server is scenario-wide rather than per-route, since
// a test exercising "malformed body" wants every provider it touches to see
// a malformed body, not a mix.
type Scenario string

const (
	// ScenarioSuccess returns a realistic, well-formed payload from every route.
	ScenarioSuccess Scenario = "success"
	// ScenarioEmpty returns a well-formed payload with no rows/results — the
	// "provider is up but has nothing to report" case, distinct from an error.
	ScenarioEmpty Scenario = "empty"
	// ScenarioMalformed returns syntactically invalid JSON (a truncated body),
	// exercising the json.Decode error path in every provider.
	ScenarioMalformed Scenario = "malformed"
	// ScenarioProviderError returns each provider's own real error shape
	// (Meta's {"error":{...}} object, a non-200 status elsewhere).
	ScenarioProviderError Scenario = "provider_error"
	// ScenarioNoContent is Nager.Date's documented 204 response for a country
	// with no holidays configured — a real, valid response, not an error.
	ScenarioNoContent Scenario = "no_content"
)

// NewMux builds the routed handler for the given scenario without binding
// any listener — used by NewServer (in-process tests) and by the standalone
// cmd/main.go binary (a real bound port for CI/manual use) alike.
func NewMux(scenario Scenario) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/forecast", weatherHandler(scenario))
	mux.HandleFunc("/api/v3/PublicHolidays/", calendarHandler(scenario))
	mux.HandleFunc("/api/v1/query", prometheusHandler(scenario))
	// option.WithEndpoint replaces the SDK's entire base path (not just the
	// host), so a request against an overridden endpoint lands on
	// "{endpoint}/calendars/{id}/events", not "{endpoint}/calendar/v3/calendars/...".
	mux.HandleFunc("/calendars/", businessCalendarHandler(scenario))
	// StripeReconciliationProvider's BaseURL already includes the version
	// segment by convention (default "https://api.stripe.com/v1"), so a test
	// pointing BaseURL straight at srv.URL hits "{srv.URL}/charges", not
	// "{srv.URL}/v1/charges".
	mux.HandleFunc("/charges", stripeChargesHandler(scenario))
	// Meta's insights path carries a variable ad-account segment
	// (/act_XXXXXXXXXX/insights); GA4's REST paths are POST-only, custom-method
	// style (…:runRealtimeReport / …:runReport) that a plain ServeMux pattern
	// can't express cleanly, so both share one path-inspecting catch-all.
	mux.HandleFunc("/", catchAllHandler(scenario))

	return mux
}

// NewServer starts an in-process httptest.Server configured for the given
// scenario. Point a provider's BaseURL (or, for GA4, its Endpoint) at srv.URL.
func NewServer(scenario Scenario) *httptest.Server {
	return httptest.NewServer(NewMux(scenario))
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// ---- Weather (Open-Meteo) ----

func weatherHandler(scenario Scenario) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch scenario {
		case ScenarioMalformed:
			writeJSON(w, http.StatusOK, `{"current":{"temperature_2m":29.4,"rain"`) // truncated
		case ScenarioProviderError:
			// Open-Meteo returns a distinct error envelope with a non-200 status.
			writeJSON(w, http.StatusBadRequest, `{"error":true,"reason":"Latitude must be in range of -90 to 90°"}`)
		case ScenarioEmpty:
			writeJSON(w, http.StatusOK, `{"current":{"time":"2026-09-06T12:00","temperature_2m":0,"relative_humidity_2m":0,"precipitation":0,"rain":0,"weather_code":0,"wind_speed_10m":0}}`)
		default: // ScenarioSuccess
			writeJSON(w, http.StatusOK, `{"current":{"time":"2026-09-06T15:00","temperature_2m":29.8,"relative_humidity_2m":74,"precipitation":12.4,"rain":12.4,"weather_code":63,"wind_speed_10m":18.2}}`)
		}
	}
}

// ---- Calendar (Nager.Date) ----

func calendarHandler(scenario Scenario) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch scenario {
		case ScenarioNoContent:
			w.WriteHeader(http.StatusNoContent)
		case ScenarioMalformed:
			writeJSON(w, http.StatusOK, `[{"date":"2026-01-14","localName":"Tamil Thai Pongal`) // truncated
		case ScenarioProviderError:
			http.Error(w, "country code not recognised", http.StatusBadRequest)
		case ScenarioEmpty:
			writeJSON(w, http.StatusOK, `[]`)
		default: // ScenarioSuccess — dynamically dated so "is today a holiday" assertions are deterministic
			today := time.Now().Format("2006-01-02")
			body := fmt.Sprintf(`[{"date":%q,"localName":"Poya Day","name":"Full Moon Poya Day","countryCode":"LK","types":["Public"]}]`, today)
			writeJSON(w, http.StatusOK, body)
		}
	}
}

// ---- Prometheus ----

func prometheusHandler(scenario Scenario) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch scenario {
		case ScenarioMalformed:
			writeJSON(w, http.StatusOK, `{"status":"success","data":{"resultType":"vector","result":[{"value"`) // truncated
		case ScenarioProviderError:
			writeJSON(w, http.StatusOK, `{"status":"error","errorType":"bad_data","error":"invalid parameter \"query\""}`)
		case ScenarioEmpty:
			writeJSON(w, http.StatusOK, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
		default: // ScenarioSuccess
			ts := time.Now().Unix()
			body := fmt.Sprintf(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[%d,"23.7"]}]}}`, ts)
			writeJSON(w, http.StatusOK, body)
		}
	}
}

// ---- Google Calendar API v3 (business/marketing calendar) ----

func businessCalendarHandler(scenario Scenario) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch scenario {
		case ScenarioMalformed:
			writeJSON(w, http.StatusOK, `{"items":[{"summary":"Product Launch`) // truncated
		case ScenarioProviderError:
			writeJSON(w, http.StatusForbidden, `{"error":{"code":403,"message":"Insufficient permission to read this calendar.","errors":[{"reason":"forbidden"}]}}`)
		case ScenarioEmpty:
			writeJSON(w, http.StatusOK, `{"items":[]}`)
		default: // ScenarioSuccess — one event active right now, one starting in 2 days
			now := time.Now()
			active := now.Add(-30 * time.Minute)
			activeEnd := now.Add(90 * time.Minute)
			upcoming := now.Add(48 * time.Hour)
			upcomingEnd := now.Add(50 * time.Hour)
			body := fmt.Sprintf(`{"items":[
				{"summary":"Flash Sale Campaign — Weekend Push","start":{"dateTime":%q},"end":{"dateTime":%q}},
				{"summary":"Aqua Line Product Launch","start":{"dateTime":%q},"end":{"dateTime":%q}}
			]}`,
				active.Format(time.RFC3339), activeEnd.Format(time.RFC3339),
				upcoming.Format(time.RFC3339), upcomingEnd.Format(time.RFC3339),
			)
			writeJSON(w, http.StatusOK, body)
		}
	}
}

func stripeChargesHandler(scenario Scenario) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch scenario {
		case ScenarioMalformed:
			writeJSON(w, http.StatusOK, `{"object":"list","data":[{"id":"ch_1"`) // truncated
		case ScenarioProviderError:
			writeJSON(w, http.StatusUnauthorized, `{"error":{"message":"Invalid API Key provided","type":"invalid_request_error"}}`)
		case ScenarioEmpty:
			writeJSON(w, http.StatusOK, `{"object":"list","data":[]}`)
		default: // ScenarioSuccess — two succeeded charges, one failed (should be excluded from the rollup)
			now := time.Now().Unix()
			body := fmt.Sprintf(`{"object":"list","data":[
				{"id":"ch_1","amount":2500,"currency":"usd","status":"succeeded","created":%d},
				{"id":"ch_2","amount":1500,"currency":"usd","status":"succeeded","created":%d},
				{"id":"ch_3","amount":900,"currency":"usd","status":"failed","created":%d}
			]}`, now, now, now)
			writeJSON(w, http.StatusOK, body)
		}
	}
}

// ---- Meta Graph API insights + GA4 Data API (catch-all, path-inspected) ----

func catchAllHandler(scenario Scenario) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/insights"):
			// Real Meta API convention: an ad-account segment is prefixed
			// "act_"; a Page ID never is. Same edge name, different shape.
			segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			if len(segments) >= 2 && strings.HasPrefix(segments[len(segments)-2], "act_") {
				metaInsightsHandler(scenario)(w, r)
			} else {
				metaPageInsightsHandler(scenario)(w, r)
			}
		case strings.HasSuffix(r.URL.Path, ":runRealtimeReport"):
			ga4Handler(scenario, true)(w, r)
		case strings.HasSuffix(r.URL.Path, ":runReport"):
			ga4Handler(scenario, false)(w, r)
		default:
			http.NotFound(w, r)
		}
	}
}

func metaInsightsHandler(scenario Scenario) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch scenario {
		case ScenarioMalformed:
			writeJSON(w, http.StatusOK, `{"data":[{"spend":"342.50"`) // truncated
		case ScenarioProviderError:
			// Meta's real error envelope: HTTP 200 with an "error" object, not a non-200 status.
			writeJSON(w, http.StatusOK, `{"error":{"message":"Invalid OAuth access token.","type":"OAuthException","code":190}}`)
		case ScenarioEmpty:
			writeJSON(w, http.StatusOK, `{"data":[]}`)
		default: // ScenarioSuccess
			writeJSON(w, http.StatusOK, `{"data":[{"spend":"342.50","impressions":"18420","clicks":"612","ctr":"3.32","cpc":"0.56","date_start":"2026-09-06","date_stop":"2026-09-06"}]}`)
		}
	}
}

func metaPageInsightsHandler(scenario Scenario) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch scenario {
		case ScenarioMalformed:
			writeJSON(w, http.StatusOK, `{"data":[{"name":"page_post_engagements"`) // truncated
		case ScenarioProviderError:
			writeJSON(w, http.StatusOK, `{"error":{"message":"Invalid OAuth access token.","type":"OAuthException","code":190}}`)
		case ScenarioEmpty:
			writeJSON(w, http.StatusOK, `{"data":[]}`)
		default: // ScenarioSuccess
			body := fmt.Sprintf(`{"data":[
				{"name":"page_post_engagements","period":"day","values":[{"value":342,"end_time":%q}]},
				{"name":"page_engaged_users","period":"day","values":[{"value":128,"end_time":%q}]},
				{"name":"page_impressions","period":"day","values":[{"value":9840,"end_time":%q}]}
			]}`, time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339))
			writeJSON(w, http.StatusOK, body)
		}
	}
}

func ga4Handler(scenario Scenario, realtime bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch scenario {
		case ScenarioMalformed:
			writeJSON(w, http.StatusOK, `{"rows":[{"metricValues":[{"value"`) // truncated
		case ScenarioProviderError:
			writeJSON(w, http.StatusForbidden, `{"error":{"code":403,"message":"User does not have sufficient permissions for this property.","status":"PERMISSION_DENIED"}}`)
		case ScenarioEmpty:
			writeJSON(w, http.StatusOK, `{"rows":[]}`)
		default: // ScenarioSuccess
			if realtime {
				// activeUsers, screenPageViews, eventCount, keyEvents
				writeJSON(w, http.StatusOK, `{"rows":[{"metricValues":[{"value":"482"},{"value":"1204"},{"value":"3980"},{"value":"14"}]}]}`)
			} else {
				// engagementRate, userEngagementDuration, bounceRate, conversions
				writeJSON(w, http.StatusOK, `{"rows":[{"metricValues":[{"value":"0.63"},{"value":"215.4"},{"value":"0.37"},{"value":"9"}]}]}`)
			}
		}
	}
}
