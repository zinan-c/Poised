package springair

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zinan-c/Poised/internal/core"
)

func TestSpringAirSearchReturnsNormalizedObservations(t *testing.T) {
	var sawWarmup bool
	var sawSearch bool

	testServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		switch httpRequest.URL.Path {
		case "/SHA-CAN.html":
			sawWarmup = true
			responseWriter.WriteHeader(http.StatusOK)
			_, _ = responseWriter.Write([]byte("<html></html>"))
		case searchByTimePath:
			sawSearch = true
			if got := httpRequest.Header.Get("X-Requested-With"); got != "XMLHttpRequest" {
				t.Fatalf("unexpected X-Requested-With: %s", got)
			}
			if err := httpRequest.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			if got := httpRequest.PostForm.Get("DepartureDate"); got != "2026-07-10" {
				t.Fatalf("unexpected departure date: %s", got)
			}
			if got := httpRequest.PostForm.Get("SeatsNum"); got != "1" {
				t.Fatalf("unexpected seats num: %s", got)
			}
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`{
				"Code": "0",
				"ErrorMessage": "0",
				"MinPrice": 390,
				"Route": [[{
					"No": "9C8855",
					"CompanyName": "Spring Airlines",
					"Type": "Airbus 321",
					"MinCabinPrice": 390,
					"MinCabinPriceForDisplay": 390,
					"MinCabinLevel": 5,
					"FlightsTime": "2 hours 35 minutes",
					"FlightTimeM": "2h35m",
					"Departure": "Shanghai",
					"DepartureCode": "SHA",
					"DepartureAirportCode": "SHA",
					"DepartureStation": "Hongqiao International Airport T1",
					"DepartureTime": "2026-07-10 12:50:00",
					"Arrival": "Guangzhou",
					"ArrivalCode": "CAN",
					"ArrivalAirportCode": "CAN",
					"ArrivalStation": "Baiyun International Airport T3",
					"ArrivalTime": "2026-07-10 15:25:00",
					"AircraftCabins": [{
						"CabinLevel": 5,
						"CabinLevelName": "Basic Economy",
						"SortNo": 1,
						"AircraftCabinInfos": [{
							"Name": "PB",
							"Price": 390,
							"Remain": 0,
							"AirportConstructionFees": 50,
							"FuelSurcharge": 100,
							"OtherFees": 0,
							"Baggage": 0,
							"HandBaggage": 0
						}]
					}]
				}]]
			}`))
		default:
			t.Fatalf("unexpected path: %s", httpRequest.URL.Path)
		}
	}))
	defer testServer.Close()

	payload, err := json.Marshal(Payload{
		RouteName:   "sha-can-test",
		BaseURL:     testServer.URL,
		Departure:   "Shanghai",
		Arrival:     "Guangzhou",
		DepCityCode: "SHA",
		ArrCityCode: "CAN",
		DepDate:     "2026-07-10",
		AdultCount:  1,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	adapter := New()
	adapter.now = func() time.Time {
		return time.Date(2026, 7, 12, 1, 2, 3, 0, time.UTC)
	}

	result, err := adapter.Run(context.Background(), core.RunInput{
		JobID:   "springair-test",
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("run adapter: %v", err)
	}
	if !sawWarmup {
		t.Fatal("expected route page warmup")
	}
	if !sawSearch {
		t.Fatal("expected SearchByTime request")
	}
	if result.Status != core.RunStatusSuccess {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if result.Data["observation_count"] != 1 {
		t.Fatalf("unexpected observation count: %#v", result.Data["observation_count"])
	}
}

func TestSpringAirFailsOnAPIErrorCode(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		responseWriter.Header().Set("Content-Type", "application/json")
		if httpRequest.URL.Path == "/SHA-CAN.html" {
			_, _ = responseWriter.Write([]byte("<html></html>"))
			return
		}
		_, _ = responseWriter.Write([]byte(`{"Code": "429", "ErrorMessage": "blocked_by_waf"}`))
	}))
	defer testServer.Close()

	payload, err := json.Marshal(Payload{
		BaseURL:     testServer.URL,
		Departure:   "Shanghai",
		Arrival:     "Guangzhou",
		DepCityCode: "SHA",
		ArrCityCode: "CAN",
		DepDate:     "2026-07-10",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	result, err := New().Run(context.Background(), core.RunInput{
		JobID:   "springair-test",
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("run adapter: %v", err)
	}
	if result.Status != core.RunStatusFailed {
		t.Fatalf("unexpected status: %s", result.Status)
	}
}

func TestSpringAirValidatesRequiredPayload(t *testing.T) {
	payload, err := json.Marshal(Payload{})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	result, err := New().Run(context.Background(), core.RunInput{
		JobID:   "springair-test",
		Payload: payload,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if result.Status != core.RunStatusFailed {
		t.Fatalf("unexpected status: %s", result.Status)
	}
}
