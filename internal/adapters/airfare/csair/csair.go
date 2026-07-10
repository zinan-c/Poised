package csair

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/zinan-c/Poised/internal/adapters/airfare"
	"github.com/zinan-c/Poised/internal/core"
)

const (
	adapterName      = "csair"
	defaultBaseURL   = "https://b2c.csair.com"
	defaultLanguage  = "zh"
	defaultRouteType = "S"
	defaultUserAgent = "Mozilla/5.0 (compatible; Poised/0.1; +https://github.com/zinan-c/Poised)"
	directQueryPath  = "/portal/main/flight/direct/query"
	bookingPagePath  = "/B2C40/newTrips/static/main/page/booking/index.html"
)

type Adapter struct {
	clientFactory func(payload Payload) (*airfare.JSONClient, error)
	now           func() time.Time
}

type Payload struct {
	RouteName       string   `json:"route_name"`
	BaseURL         string   `json:"base_url"`
	BookingURL      string   `json:"booking_url"`
	SkipWarmup      bool     `json:"skip_warmup"`
	Language        string   `json:"language"`
	RouteType       string   `json:"route_type"`
	DepCityCode     string   `json:"dep_city_code"`
	DepAirportCodes []string `json:"dep_airport_codes"`
	ArrCityCode     string   `json:"arr_city_code"`
	ArrAirportCodes []string `json:"arr_airport_codes"`
	DepDate         string   `json:"dep_date"`
	AdultCount      int      `json:"adult_count"`
	ChildCount      int      `json:"child_count"`
	InfantCount     int      `json:"infant_count"`
	OrderChannel    string   `json:"order_channel"`
	BusinessType    string   `json:"business_type"`
	Cache           int      `json:"cache"`
	PreURL          string   `json:"pre_url"`
}

type directQueryRequest struct {
	DepCity       string `json:"depCity"`
	ArrCity       string `json:"arrCity"`
	FlightDate    string `json:"flightDate"`
	AdultNum      string `json:"adultNum"`
	ChildNum      string `json:"childNum"`
	InfantNum     string `json:"infantNum"`
	CabinOrder    string `json:"cabinOrder"`
	AirLine       int    `json:"airLine"`
	FlyType       int    `json:"flyType"`
	International string `json:"international"`
	Action        string `json:"action"`
	SegType       string `json:"segType"`
	Cache         int    `json:"cache"`
	PreURL        string `json:"preUrl"`
	TariffRules   []any  `json:"tariffRules"`
	IsMember      string `json:"isMember"`
	Language      string `json:"language"`
	BusinessType  string `json:"businessType"`
	IsMultipass   int    `json:"isMultipass"`
}

type directQueryResponse struct {
	Success   bool            `json:"success"`
	ErrorCode string          `json:"errorCode"`
	ErrorMsg  string          `json:"errorMsg"`
	Message   string          `json:"message"`
	Data      directQueryData `json:"data"`
}

type directQueryData struct {
	ID         string    `json:"id"`
	CreateTime string    `json:"createTime"`
	Segment    []segment `json:"segment"`
	Segments   []segment `json:"segments"`
}

type segment struct {
	DepCity       string     `json:"depCity"`
	ArrCity       string     `json:"arrCity"`
	Date          string     `json:"date"`
	AdultFuelTax  string     `json:"adultFuelTax"`
	ChildFuelTax  string     `json:"childFuelTax"`
	InfantFuelTax string     `json:"infantFuelTax"`
	DateFlight    dateFlight `json:"dateFlight"`
}

type dateFlight struct {
	Flight []flight `json:"flight"`
}

type flight struct {
	FlightNo           string  `json:"flightNo"`
	AirLine            string  `json:"airLine"`
	CodeShare          string  `json:"codeShare"`
	DepPort            string  `json:"depPort"`
	ArrPort            string  `json:"arrPort"`
	DepDate            string  `json:"depDate"`
	ArrDate            string  `json:"arrDate"`
	DepTime            string  `json:"depTime"`
	ArrTime            string  `json:"arrTime"`
	TimeDuringFlight   string  `json:"timeDuringFlight"`
	TimeDuringFlightEn string  `json:"timeDuringFlightEn"`
	Plane              string  `json:"plane"`
	StopNumber         string  `json:"stopNumber"`
	Meal               string  `json:"meal"`
	Term               string  `json:"term"`
	Rate               string  `json:"rate"`
	DepartureTerminal  string  `json:"departureTerminal"`
	ArrivalTerminal    string  `json:"arrivalTerminal"`
	Cabin              []cabin `json:"cabin"`
}

type cabin struct {
	Name                      string        `json:"name"`
	AdultPrice                numberString  `json:"adultPrice"`
	ChildPrice                numberString  `json:"childPrice"`
	InfantPrice               numberString  `json:"infantPrice"`
	Discount                  string        `json:"discount"`
	AdultFareBasis            string        `json:"adultFareBasis"`
	ChildFareBasis            string        `json:"childFareBasis"`
	InfantFareBasis           string        `json:"infantFareBasis"`
	FareReference             string        `json:"fareReference"`
	GBAdultPrice              string        `json:"gbAdultPrice"`
	Info                      string        `json:"info"`
	BrandType                 string        `json:"brandType"`
	AdultBaggageAllowance     string        `json:"adultbaggageallowance"`
	AdultBaggageAllowanceUnit string        `json:"adultbaggageallowanceunit"`
	SecondPrices              []secondPrice `json:"secondPrices"`
}

type secondPrice struct {
	Code  string             `json:"code"`
	Price []secondPriceValue `json:"price"`
}

type secondPriceValue struct {
	Adult         numberString `json:"adult"`
	Discount      string       `json:"discount"`
	FareReference string       `json:"fareReference"`
}

type pairError struct {
	DepCity   string `json:"dep_city"`
	ArrCity   string `json:"arr_city"`
	ErrorCode string `json:"error_code,omitempty"`
	ErrorMsg  string `json:"error_msg,omitempty"`
	Message   string `json:"message,omitempty"`
}

type numberString float64

func New() *Adapter {
	return &Adapter{
		clientFactory: newClient,
		now:           time.Now,
	}
}

func NewWithClient(client *airfare.JSONClient) *Adapter {
	return &Adapter{
		clientFactory: func(Payload) (*airfare.JSONClient, error) {
			return client, nil
		},
		now: time.Now,
	}
}

func (adapter *Adapter) Name() string {
	return adapterName
}

func (adapter *Adapter) Kind() string {
	return "airfare"
}

func (adapter *Adapter) Run(ctx context.Context, input core.RunInput) (core.RunResult, error) {
	var payload Payload
	if err := json.Unmarshal(input.Payload, &payload); err != nil {
		return core.RunResult{Status: core.RunStatusFailed}, err
	}
	payload.withDefaults()
	if err := payload.validate(); err != nil {
		return core.RunResult{Status: core.RunStatusFailed}, err
	}

	client, err := adapter.clientFactory(payload)
	if err != nil {
		return core.RunResult{Status: core.RunStatusFailed}, err
	}

	bookingURL := payload.bookingURL()
	headers := payload.headers(bookingURL)
	if !payload.SkipWarmup {
		if _, _, err := client.Get(ctx, bookingURL, headers); err != nil {
			return core.RunResult{
				Status:  core.RunStatusFailed,
				Summary: "csair booking page warmup failed",
				Data: map[string]any{
					"route_name":  payload.RouteName,
					"booking_url": bookingURL,
				},
			}, err
		}
	}

	var observations []airfare.PriceObservation
	var errors []pairError
	for _, depAirport := range payload.DepAirportCodes {
		for _, arrAirport := range payload.ArrAirportCodes {
			requestPayload := payload.directQueryRequest(depAirport, arrAirport)
			_, responseBody, err := client.PostJSON(ctx, directQueryPath, requestPayload, headers)
			if err != nil {
				errors = append(errors, pairError{DepCity: depAirport, ArrCity: arrAirport, Message: err.Error()})
				continue
			}

			var response directQueryResponse
			if err := json.Unmarshal(responseBody, &response); err != nil {
				errors = append(errors, pairError{DepCity: depAirport, ArrCity: arrAirport, Message: err.Error()})
				continue
			}
			if !response.Success {
				errors = append(errors, pairError{
					DepCity:   depAirport,
					ArrCity:   arrAirport,
					ErrorCode: response.ErrorCode,
					ErrorMsg:  response.ErrorMsg,
					Message:   response.Message,
				})
				continue
			}
			observations = append(observations, adapter.observations(payload, response)...)
		}
	}

	status := core.RunStatusSuccess
	summary := fmt.Sprintf("csair fare search found %d observations", len(observations))
	if len(observations) == 0 {
		status = core.RunStatusFailed
		summary = "csair fare search returned no observations"
	}
	data := map[string]any{
		"route_name":        payload.RouteName,
		"pair_error_count":  len(errors),
		"pair_errors":       errors,
		"observation_count": len(observations),
		"observations":      observations,
	}
	if lowest, ok := airfare.LowestTotalPrice(observations); ok {
		summary = fmt.Sprintf("csair lowest fare %.2f %s on %s", lowest.TotalPrice, lowest.Currency, lowest.FlightNo)
		data["lowest"] = lowest
	}

	return core.RunResult{
		Status:  status,
		Summary: summary,
		Data:    data,
	}, nil
}

func (adapter *Adapter) observations(payload Payload, response directQueryResponse) []airfare.PriceObservation {
	observedAt := adapter.now().UTC()
	segments := response.Data.Segment
	if len(segments) == 0 {
		segments = response.Data.Segments
	}
	observations := make([]airfare.PriceObservation, 0)
	for _, segment := range segments {
		for _, flight := range segment.DateFlight.Flight {
			for _, cabin := range flight.Cabin {
				basePrice := float64(cabin.AdultPrice)
				if basePrice <= 0 {
					continue
				}
				taxPrice := parseFloat(segment.AdultFuelTax)
				totalPrice := basePrice + taxPrice
				observations = append(observations, airfare.PriceObservation{
					Adapter:        adapterName,
					RouteName:      payload.RouteName,
					Airline:        firstNonEmpty(flight.AirLine, "CZ"),
					FlightNo:       flight.FlightNo,
					Origin:         flight.DepPort,
					Destination:    flight.ArrPort,
					DepartureTime:  combineDateTime(flight.DepDate, flight.DepTime),
					ArrivalTime:    combineDateTime(flight.ArrDate, flight.ArrTime),
					AircraftType:   flight.Plane,
					CabinCode:      cabin.Name,
					BasePrice:      basePrice,
					TaxPrice:       taxPrice,
					TotalPrice:     totalPrice,
					Currency:       "CNY",
					Availability:   cabin.Info,
					RawProductCode: cabin.BrandType,
					RawPriceSource: cabin.FareReference,
					ObservedAt:     observedAt,
				})
			}
		}
	}
	return observations
}

func newClient(payload Payload) (*airfare.JSONClient, error) {
	return airfare.NewJSONClient(payload.BaseURL, nil, map[string]string{
		"Accept":     "application/json, text/javascript, */*; q=0.01",
		"User-Agent": defaultUserAgent,
	})
}

func (payload *Payload) withDefaults() {
	if payload.BaseURL == "" {
		payload.BaseURL = defaultBaseURL
	}
	if payload.Language == "" {
		payload.Language = defaultLanguage
	}
	if payload.RouteType == "" {
		payload.RouteType = defaultRouteType
	}
	if payload.AdultCount == 0 {
		payload.AdultCount = 1
	}
	if payload.BusinessType == "" {
		payload.BusinessType = "COMMON"
	}
	if len(payload.DepAirportCodes) == 0 && payload.DepCityCode != "" {
		payload.DepAirportCodes = []string{payload.DepCityCode}
	}
	if len(payload.ArrAirportCodes) == 0 && payload.ArrCityCode != "" {
		payload.ArrAirportCodes = []string{payload.ArrCityCode}
	}
}

func (payload Payload) validate() error {
	if payload.DepCityCode == "" {
		return fmt.Errorf("dep_city_code is required")
	}
	if payload.ArrCityCode == "" {
		return fmt.Errorf("arr_city_code is required")
	}
	if len(payload.DepAirportCodes) == 0 {
		return fmt.Errorf("dep_airport_codes is required")
	}
	if len(payload.ArrAirportCodes) == 0 {
		return fmt.Errorf("arr_airport_codes is required")
	}
	if payload.DepDate == "" {
		return fmt.Errorf("dep_date is required")
	}
	return nil
}

func (payload Payload) headers(referer string) map[string]string {
	return map[string]string{
		"Content-Type": "application/json",
		"Origin":       strings.TrimRight(payload.BaseURL, "/"),
		"Referer":      referer,
	}
}

func (payload Payload) directQueryRequest(depAirport string, arrAirport string) directQueryRequest {
	return directQueryRequest{
		DepCity:       depAirport,
		ArrCity:       arrAirport,
		FlightDate:    strings.ReplaceAll(payload.DepDate, "-", ""),
		AdultNum:      strconv.Itoa(payload.AdultCount),
		ChildNum:      strconv.Itoa(payload.ChildCount),
		InfantNum:     strconv.Itoa(payload.InfantCount),
		CabinOrder:    "0",
		AirLine:       1,
		FlyType:       0,
		International: "0",
		Action:        "0",
		SegType:       "1",
		Cache:         payload.Cache,
		PreURL:        payload.PreURL,
		TariffRules:   []any{},
		IsMember:      "",
		Language:      payload.Language,
		BusinessType:  payload.BusinessType,
		IsMultipass:   1,
	}
}

func (payload Payload) bookingURL() string {
	if payload.BookingURL != "" {
		return payload.BookingURL
	}
	values := make([]string, 0, 10)
	values = append(values,
		"t="+payload.RouteType,
		"c1="+payload.DepCityCode,
		"c2="+payload.ArrCityCode,
		"d1="+payload.DepDate,
		"at="+strconv.Itoa(payload.AdultCount),
		"ct="+strconv.Itoa(payload.ChildCount),
		"it="+strconv.Itoa(payload.InfantCount),
	)
	if len(payload.DepAirportCodes) > 0 {
		values = append(values, "b1="+strings.Join(payload.DepAirportCodes, "-"))
	}
	if len(payload.ArrAirportCodes) > 0 {
		values = append(values, "b2="+strings.Join(payload.ArrAirportCodes, "-"))
	}
	if payload.OrderChannel != "" {
		values = append(values, "orderChannel="+payload.OrderChannel)
	}
	return strings.TrimRight(payload.BaseURL, "/") + bookingPagePath + "?" + strings.Join(values, "&")
}

func combineDateTime(date string, timeValue string) string {
	if date == "" {
		return timeValue
	}
	if timeValue == "" {
		return date
	}
	if len(date) == 8 {
		date = date[:4] + "-" + date[4:6] + "-" + date[6:]
	}
	if len(timeValue) == 4 {
		timeValue = timeValue[:2] + ":" + timeValue[2:]
	}
	return date + " " + timeValue
}

func parseFloat(value string) float64 {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (number *numberString) UnmarshalJSON(data []byte) error {
	var numeric float64
	if err := json.Unmarshal(data, &numeric); err == nil {
		*number = numberString(numeric)
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	if text == "" {
		*number = 0
		return nil
	}
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return err
	}
	*number = numberString(parsed)
	return nil
}
