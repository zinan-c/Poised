package csair

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zinan-c/Poised/internal/core"
)

func TestCSAirSearchAggregatesSuccessfulAirportPairs(t *testing.T) {
	var sawWarmup bool
	var requests []directQueryRequest

	testServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		switch httpRequest.URL.Path {
		case bookingPagePath:
			sawWarmup = true
			responseWriter.WriteHeader(http.StatusOK)
			_, _ = responseWriter.Write([]byte("<html></html>"))
		case directQueryPath:
			if got := httpRequest.Header.Get("Content-Type"); got != "application/json" {
				t.Fatalf("unexpected content type: %s", got)
			}
			var request directQueryRequest
			if err := json.NewDecoder(httpRequest.Body).Decode(&request); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			requests = append(requests, request)
			responseWriter.Header().Set("Content-Type", "application/json")
			if request.DepCity == "SHA" && request.ArrCity == "PKX" {
				_, _ = responseWriter.Write([]byte(`{
					"success": true,
					"data": {
						"id": "search-id",
						"createTime": "2026-07-08 12:00:00",
						"segment": [{
							"depCity": "SHA",
							"arrCity": "PKX",
							"date": "20260708",
							"adultFuelTax": "120",
							"dateFlight": {
								"flight": [{
									"flightNo": "CZ8886",
									"airLine": "CZ",
									"depPort": "SHA",
									"arrPort": "PKX",
									"depDate": "20260708",
									"arrDate": "20260708",
									"depTime": "1945",
									"arrTime": "2200",
									"timeDuringFlight": "2 hours 15 minutes",
									"plane": "350",
									"departureTerminal": "T2",
									"arrivalTerminal": "--",
									"cabin": [{
										"name": "Q",
										"adultPrice": 690,
										"discount": "42",
										"fareReference": "fare-ref",
										"info": "&gt;9",
										"brandType": "DX"
									}]
								}]
							}
						}]
					}
				}`))
				return
			}
			_, _ = responseWriter.Write([]byte(`{
				"success": false,
				"errorCode": "4001",
				"errorMsg": "route mismatch"
			}`))
		default:
			t.Fatalf("unexpected path: %s", httpRequest.URL.Path)
		}
	}))
	defer testServer.Close()

	payload, err := json.Marshal(Payload{
		RouteName:       "sha-bjs-test",
		BaseURL:         testServer.URL,
		DepCityCode:     "SHA",
		DepAirportCodes: []string{"SHA"},
		ArrCityCode:     "BJS",
		ArrAirportCodes: []string{"PEK", "PKX"},
		DepDate:         "2026-07-08",
		AdultCount:      1,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	adapter := New()
	adapter.now = func() time.Time {
		return time.Date(2026, 7, 10, 1, 2, 3, 0, time.UTC)
	}

	result, err := adapter.Run(context.Background(), core.RunInput{
		JobID:   "csair-test",
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("run adapter: %v", err)
	}
	if !sawWarmup {
		t.Fatal("expected booking page warmup")
	}
	if len(requests) != 2 {
		t.Fatalf("expected 2 airport-pair requests, got %d", len(requests))
	}
	if result.Status != core.RunStatusSuccess {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if result.Data["observation_count"] != 1 {
		t.Fatalf("unexpected observation count: %#v", result.Data["observation_count"])
	}
	if result.Data["pair_error_count"] != 1 {
		t.Fatalf("unexpected pair error count: %#v", result.Data["pair_error_count"])
	}
}

func TestCSAirFailsWhenAllAirportPairsFail(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		responseWriter.Header().Set("Content-Type", "application/json")
		if httpRequest.URL.Path == bookingPagePath {
			_, _ = responseWriter.Write([]byte("<html></html>"))
			return
		}
		_, _ = responseWriter.Write([]byte(`{"success": false, "errorCode": "4001", "errorMsg": "failed"}`))
	}))
	defer testServer.Close()

	payload, err := json.Marshal(Payload{
		BaseURL:         testServer.URL,
		DepCityCode:     "SHA",
		DepAirportCodes: []string{"SHA"},
		ArrCityCode:     "BJS",
		ArrAirportCodes: []string{"PEK"},
		DepDate:         "2026-07-08",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	result, err := New().Run(context.Background(), core.RunInput{
		JobID:   "csair-test",
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("run adapter: %v", err)
	}
	if result.Status != core.RunStatusFailed {
		t.Fatalf("unexpected status: %s", result.Status)
	}
}
