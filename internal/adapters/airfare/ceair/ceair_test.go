package ceair

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zinan-c/Poised/internal/core"
)

func TestCEAirSearchReturnsNormalizedObservations(t *testing.T) {
	var sawWarmup bool
	var request briefInfoRequest

	testServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		switch httpRequest.URL.Path {
		case "/zh/usd/shopping/oneway/SHA,PVG-PEK,PKX,QIP(R)/2026-07-09.":
			sawWarmup = true
			responseWriter.WriteHeader(http.StatusOK)
			_, _ = responseWriter.Write([]byte("<html></html>"))
		case briefInfoEndpoint:
			if httpRequest.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", httpRequest.Method)
			}
			if got := httpRequest.Header.Get("languageCode"); got != "zh" {
				t.Fatalf("unexpected languageCode: %s", got)
			}
			if got := httpRequest.Header.Get("currencyCode"); got != "USD" {
				t.Fatalf("unexpected currencyCode: %s", got)
			}
			if err := json.NewDecoder(httpRequest.Body).Decode(&request); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`{
				"resultCode": "S200",
				"resultMsg": "ok",
				"currencyCode": "USD",
				"data": {
					"productInfos": [{ "code": "COMMON_Y" }],
					"flightItems": [{
						"flightInfos": [{
							"flightSegments": [{
								"orgCode": "SHA",
								"destCode": "PKX",
								"airlineCode": "MU",
								"flightNo": "FM9101",
								"fltDate": "2026-07-09",
								"orgTime": "12:50",
								"arriDate": "2026-07-09",
								"destTime": "15:10",
								"fltSpanTime": "140",
								"planeType": "73E"
							}],
							"flightSort": {
								"price": 86,
								"priceWithTax": 108.1,
								"duration": 140
							}
						}],
						"cabinInfoDescs": [{
							"ccode": "S",
							"cabinLevelName": "Economy",
							"fareInfoDescList": [{
								"paxType": "ADT",
								"lprice": 86,
								"taxPrice": 22.1,
								"totalPrice": 108.1,
								"priceSource": "STANDARD",
								"productCode": "COMMON_Y"
							}]
						}]
					}]
				}
			}`))
		default:
			t.Fatalf("unexpected path: %s", httpRequest.URL.Path)
		}
	}))
	defer testServer.Close()

	payload, err := json.Marshal(Payload{
		RouteName:      "sha-bjs-test",
		BaseURL:        testServer.URL,
		LanguageCode:   "zh",
		CurrencyCode:   "USD",
		DepCityCode:    "SHA",
		DepCode:        []string{"SHA", "PVG"},
		ArrCityCode:    "BJS",
		ArrCode:        []string{"PEK", "PKX"},
		ArrStationCode: []string{"QIP"},
		DepDate:        "2026-07-09",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	adapter := New()
	adapter.now = func() time.Time {
		return time.Date(2026, 7, 10, 1, 2, 3, 0, time.UTC)
	}

	result, err := adapter.Run(context.Background(), core.RunInput{
		JobID:   "ceair-test",
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("run adapter: %v", err)
	}
	if !sawWarmup {
		t.Fatal("expected shopping page warmup")
	}
	if result.Status != core.RunStatusSuccess {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if request.DepCode != "SHA,PVG" {
		t.Fatalf("unexpected dep code: %s", request.DepCode)
	}
	if request.ArrStationCode != "QIP" {
		t.Fatalf("unexpected arr station code: %s", request.ArrStationCode)
	}

	lowest, ok := result.Data["lowest"].(map[string]any)
	if ok {
		t.Fatalf("lowest should be a typed value before JSON encoding, got map: %#v", lowest)
	}
	if result.Data["observation_count"] != 1 {
		t.Fatalf("unexpected observation count: %#v", result.Data["observation_count"])
	}
}

func TestCEAirValidatesRequiredPayload(t *testing.T) {
	payload, err := json.Marshal(Payload{})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	result, err := New().Run(context.Background(), core.RunInput{
		JobID:   "ceair-test",
		Payload: payload,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if result.Status != core.RunStatusFailed {
		t.Fatalf("unexpected status: %s", result.Status)
	}
}

func TestCEAirSearchReturnsFailedForEmptyObservations(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		switch httpRequest.URL.Path {
		case briefInfoEndpoint:
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`{
				"resultCode": "S200",
				"resultMsg": "ok",
				"currencyCode": "USD",
				"data": {
					"productInfos": [],
					"flightItems": []
				}
			}`))
		default:
			responseWriter.WriteHeader(http.StatusOK)
		}
	}))
	defer testServer.Close()

	payload, err := json.Marshal(Payload{
		RouteName:    "sha-bjs-empty",
		BaseURL:      testServer.URL,
		SkipWarmup:   true,
		DepCityCode:  "SHA",
		DepCode:      []string{"SHA"},
		ArrCityCode:  "BJS",
		ArrCode:      []string{"PKX"},
		DepDate:      "2026-07-09",
		AdultCount:   1,
		ChildCount:   0,
		InfantCount:  0,
		CurrencyCode: "USD",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	result, err := New().Run(context.Background(), core.RunInput{
		JobID:   "ceair-empty-test",
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("run adapter: %v", err)
	}
	if result.Status != core.RunStatusFailed {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if result.Summary != "ceair fare search returned no observations" {
		t.Fatalf("unexpected summary: %s", result.Summary)
	}
}
