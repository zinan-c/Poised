package springair

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/zinan-c/Poised/internal/adapters/airfare"
	"github.com/zinan-c/Poised/internal/core"
)

const (
	adapterName      = "springair"
	defaultBaseURL   = "https://flights.ch.com"
	defaultCurrency  = "CNY"
	defaultUserAgent = "Mozilla/5.0 (compatible; Poised/0.1; +https://github.com/zinan-c/Poised)"
	searchByTimePath = "/Flights/SearchByTime"
)

type Adapter struct {
	clientFactory func(payload Payload) (*airfare.JSONClient, error)
	now           func() time.Time
}

type Payload struct {
	RouteName           string `json:"route_name"`
	BaseURL             string `json:"base_url"`
	SearchURL           string `json:"search_url"`
	SkipWarmup          bool   `json:"skip_warmup"`
	Departure           string `json:"departure"`
	Arrival             string `json:"arrival"`
	DepCityCode         string `json:"dep_city_code"`
	ArrCityCode         string `json:"arr_city_code"`
	DepAirportCode      string `json:"dep_airport_code"`
	ArrAirportCode      string `json:"arr_airport_code"`
	DepDate             string `json:"dep_date"`
	ReturnDate          string `json:"return_date"`
	AdultCount          int    `json:"adult_count"`
	ChildCount          int    `json:"child_count"`
	InfantCount         int    `json:"infant_count"`
	Currency            string `json:"currency"`
	CurrencyCode        string `json:"currency_code"`
	SType               string `json:"s_type"`
	IfRet               bool   `json:"if_ret"`
	IsSearchDepAirport  bool   `json:"is_search_dep_airport"`
	IsSearchArrAirport  bool   `json:"is_search_arr_airport"`
	IsShowTaxPrice      bool   `json:"is_show_tax_price"`
	IsIJFlight          bool   `json:"is_ij_flight"`
	IsBg                bool   `json:"is_bg"`
	IsEmployee          bool   `json:"is_employee"`
	IsLittleGroupFlight bool   `json:"is_little_group_flight"`
	IsUM                bool   `json:"is_um"`
	ActID               int    `json:"act_id"`
	CabinActID          string `json:"cabin_act_id"`
	SpecTravTypeID      string `json:"spec_trav_type_id"`
	IsContains9CAndIJ   bool   `json:"is_contains_9c_and_ij"`
}

type searchResponse struct {
	Route               [][]routeOption `json:"Route"`
	MinPrice            flexibleNumber  `json:"MinPrice"`
	IsInternational     bool            `json:"IsInternational"`
	IsShowTaxPrice      bool            `json:"IsShowTaxprice"`
	Code                string          `json:"Code"`
	ErrorMessage        string          `json:"ErrorMessage"`
	SearchResultMessage string          `json:"SearchResultMessage"`
}

type routeOption struct {
	No                      string          `json:"No"`
	CompanyName             string          `json:"CompanyName"`
	Type                    string          `json:"Type"`
	MinCabinPrice           flexibleNumber  `json:"MinCabinPrice"`
	MinCabinPriceForDisplay flexibleNumber  `json:"MinCabinPriceForDisplay"`
	MinCabinLevel           int             `json:"MinCabinLevel"`
	FlightsTime             string          `json:"FlightsTime"`
	FlightTimeM             string          `json:"FlightTimeM"`
	Departure               string          `json:"Departure"`
	DepartureCode           string          `json:"DepartureCode"`
	DepartureAirportCode    string          `json:"DepartureAirportCode"`
	DepartureStation        string          `json:"DepartureStation"`
	DepartureTime           string          `json:"DepartureTime"`
	Arrival                 string          `json:"Arrival"`
	ArrivalCode             string          `json:"ArrivalCode"`
	ArrivalAirportCode      string          `json:"ArrivalAirportCode"`
	ArrivalStation          string          `json:"ArrivalStation"`
	ArrivalTime             string          `json:"ArrivalTime"`
	RouteID                 int             `json:"RouteId"`
	SegmentID               int             `json:"SegmentId"`
	RouteArea               int             `json:"RouteArea"`
	RouteSegType            int             `json:"RouteSegType"`
	AircraftCabins          []aircraftCabin `json:"AircraftCabins"`
}

type aircraftCabin struct {
	CabinLevel         int                 `json:"CabinLevel"`
	CabinLevelName     string              `json:"CabinLevelName"`
	SortNo             int                 `json:"SortNo"`
	CombID             string              `json:"CombId"`
	CombType           int                 `json:"CombType"`
	IsHide             bool                `json:"IsHide"`
	IsGuide            bool                `json:"IsGuide"`
	AircraftCabinInfos []aircraftCabinInfo `json:"AircraftCabinInfos"`
}

type aircraftCabinInfo struct {
	Name                    string         `json:"Name"`
	Price                   flexibleNumber `json:"Price"`
	Remain                  int            `json:"Remain"`
	AirportConstructionFees flexibleNumber `json:"AirportConstructionFees"`
	FuelSurcharge           flexibleNumber `json:"FuelSurcharge"`
	OtherFees               flexibleNumber `json:"OtherFees"`
	Baggage                 flexibleNumber `json:"Baggage"`
	HandBaggage             flexibleNumber `json:"HandBaggage"`
	Integral                flexibleNumber `json:"Integral"`
	IsActivity              bool           `json:"IsActivity"`
	SpringPassDiscount      flexibleNumber `json:"SpringPassDiscount"`
}

type flexibleNumber float64

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

	searchURL := payload.searchURL()
	headers := payload.headers(searchURL)
	if !payload.SkipWarmup {
		if _, _, err := client.Get(ctx, searchURL, headers); err != nil {
			return core.RunResult{
				Status:  core.RunStatusFailed,
				Summary: "springair search page warmup failed",
				Data: map[string]any{
					"route_name": payload.RouteName,
					"search_url": searchURL,
				},
			}, err
		}
	}

	_, responseBody, err := client.PostForm(ctx, searchByTimePath, payload.formValues(), headers)
	if err != nil {
		return core.RunResult{
			Status:  core.RunStatusFailed,
			Summary: "springair SearchByTime request failed",
			Data: map[string]any{
				"route_name": payload.RouteName,
				"endpoint":   searchByTimePath,
			},
		}, err
	}

	var response searchResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return core.RunResult{Status: core.RunStatusFailed, Summary: "springair response parse failed"}, err
	}
	if response.Code != "" && response.Code != "0" {
		return core.RunResult{
			Status:  core.RunStatusFailed,
			Summary: fmt.Sprintf("springair returned %s", response.Code),
			Data: map[string]any{
				"code":          response.Code,
				"error_message": response.ErrorMessage,
			},
		}, nil
	}

	observations := adapter.observations(payload, response)
	status := core.RunStatusSuccess
	summary := fmt.Sprintf("springair fare search found %d observations", len(observations))
	if len(observations) == 0 {
		status = core.RunStatusFailed
		summary = "springair fare search returned no observations"
	}
	data := map[string]any{
		"route_name":        payload.RouteName,
		"code":              response.Code,
		"error_message":     response.ErrorMessage,
		"min_price":         float64(response.MinPrice),
		"route_group_count": len(response.Route),
		"observation_count": len(observations),
		"observations":      observations,
	}
	if lowest, ok := airfare.LowestTotalPrice(observations); ok {
		summary = fmt.Sprintf("springair lowest fare %.2f %s on %s", lowest.TotalPrice, lowest.Currency, lowest.FlightNo)
		data["lowest"] = lowest
	}

	return core.RunResult{
		Status:  status,
		Summary: summary,
		Data:    data,
	}, nil
}

func (adapter *Adapter) observations(payload Payload, response searchResponse) []airfare.PriceObservation {
	observedAt := adapter.now().UTC()
	observations := make([]airfare.PriceObservation, 0)
	for _, group := range response.Route {
		for _, route := range group {
			for _, cabin := range route.AircraftCabins {
				for _, info := range cabin.AircraftCabinInfos {
					basePrice := float64(info.Price)
					if basePrice <= 0 {
						continue
					}
					taxPrice := float64(info.AirportConstructionFees + info.FuelSurcharge + info.OtherFees)
					totalPrice := basePrice + taxPrice
					observations = append(observations, airfare.PriceObservation{
						Adapter:        adapterName,
						RouteName:      payload.RouteName,
						Airline:        firstNonEmpty(route.CompanyName, "Spring Airlines"),
						FlightNo:       route.No,
						Origin:         firstNonEmpty(route.DepartureAirportCode, route.DepartureCode),
						Destination:    firstNonEmpty(route.ArrivalAirportCode, route.ArrivalCode),
						DepartureTime:  route.DepartureTime,
						ArrivalTime:    route.ArrivalTime,
						AircraftType:   route.Type,
						CabinCode:      info.Name,
						CabinName:      cabin.CabinLevelName,
						BasePrice:      basePrice,
						TaxPrice:       taxPrice,
						TotalPrice:     totalPrice,
						Currency:       payload.Currency,
						Availability:   strconv.Itoa(info.Remain),
						RawProductCode: strconv.Itoa(cabin.CabinLevel),
						ObservedAt:     observedAt,
					})
				}
			}
		}
	}
	return observations
}

func newClient(payload Payload) (*airfare.JSONClient, error) {
	return airfare.NewJSONClient(payload.BaseURL, nil, map[string]string{
		"Accept":     "*/*",
		"User-Agent": defaultUserAgent,
	})
}

func (payload *Payload) withDefaults() {
	if payload.BaseURL == "" {
		payload.BaseURL = defaultBaseURL
	}
	if payload.Currency == "" {
		payload.Currency = defaultCurrency
	}
	if payload.CurrencyCode == "" {
		payload.CurrencyCode = "0"
	}
	if payload.SType == "" {
		payload.SType = "0"
	}
	if payload.AdultCount == 0 {
		payload.AdultCount = 1
	}
	if payload.ReturnDate == "" {
		payload.ReturnDate = "null"
	}
	if payload.CabinActID == "" {
		payload.CabinActID = "null"
	}
}

func (payload Payload) validate() error {
	if payload.Departure == "" {
		return fmt.Errorf("departure is required")
	}
	if payload.Arrival == "" {
		return fmt.Errorf("arrival is required")
	}
	if payload.DepCityCode == "" {
		return fmt.Errorf("dep_city_code is required")
	}
	if payload.DepDate == "" {
		return fmt.Errorf("dep_date is required")
	}
	return nil
}

func (payload Payload) headers(referer string) map[string]string {
	return map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded; charset=UTF-8",
		"Origin":           strings.TrimRight(payload.BaseURL, "/"),
		"Referer":          referer,
		"X-Requested-With": "XMLHttpRequest",
	}
}

func (payload Payload) formValues() url.Values {
	values := url.Values{}
	values.Set("Active9s", "")
	values.Set("IsJC", "false")
	values.Set("IsShowTaxprice", formatBool(payload.IsShowTaxPrice))
	values.Set("Currency", payload.CurrencyCode)
	values.Set("SType", payload.SType)
	values.Set("Departure", payload.Departure)
	values.Set("Arrival", payload.Arrival)
	values.Set("DepartureDate", payload.DepDate)
	values.Set("ReturnDate", payload.ReturnDate)
	values.Set("IsIJFlight", formatBool(payload.IsIJFlight))
	values.Set("IsBg", formatBool(payload.IsBg))
	values.Set("IsEmployee", formatBool(payload.IsEmployee))
	values.Set("IsLittleGroupFlight", formatBool(payload.IsLittleGroupFlight))
	values.Set("SeatsNum", strconv.Itoa(payload.seatsNum()))
	values.Set("ActId", strconv.Itoa(payload.ActID))
	values.Set("IfRet", formatBool(payload.IfRet))
	values.Set("IsUM", formatBool(payload.IsUM))
	values.Set("CabinActId", payload.CabinActID)
	values.Set("SpecTravTypeId", payload.SpecTravTypeID)
	values.Set("IsContains9CAndIJ", formatBool(payload.IsContains9CAndIJ))
	values.Set("DepCityCode", payload.DepCityCode)
	values.Set("ArrCityCode", payload.ArrCityCode)
	values.Set("DepAirportCode", payload.DepAirportCode)
	values.Set("ArrAirportCode", payload.ArrAirportCode)
	values.Set("IsSearchDepAirport", formatBool(payload.IsSearchDepAirport))
	values.Set("IsSearchArrAirport", formatBool(payload.IsSearchArrAirport))
	values.Set("isdisplayold", "false")
	return values
}

func (payload Payload) searchURL() string {
	if payload.SearchURL != "" {
		return payload.SearchURL
	}
	values := url.Values{}
	values.Set("Departure", payload.Departure)
	values.Set("Arrival", payload.Arrival)
	values.Set("FDate", payload.DepDate)
	values.Set("DepartCityCode", payload.DepCityCode)
	values.Set("ArriveCityCode", payload.ArrCityCode)
	values.Set("IsSearchDepAirport", formatBool(payload.IsSearchDepAirport))
	values.Set("IsSearchArrAirport", formatBool(payload.IsSearchArrAirport))
	values.Set("isOnlyZf", "false")
	values.Set("ANum", strconv.Itoa(payload.AdultCount))
	values.Set("CNum", strconv.Itoa(payload.ChildCount))
	values.Set("INum", strconv.Itoa(payload.InfantCount))
	values.Set("IfRet", formatBool(payload.IfRet))
	values.Set("SType", "01")
	values.Set("MType", "0")
	values.Set("IsNew", "1")
	route := payload.DepCityCode + "-" + firstNonEmpty(payload.ArrCityCode, payload.ArrAirportCode)
	return strings.TrimRight(payload.BaseURL, "/") + "/" + route + ".html?" + values.Encode()
}

func (payload Payload) seatsNum() int {
	total := payload.AdultCount + payload.ChildCount + payload.InfantCount
	if total == 0 {
		return 1
	}
	return total
}

func formatBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (number *flexibleNumber) UnmarshalJSON(data []byte) error {
	var numeric float64
	if err := json.Unmarshal(data, &numeric); err == nil {
		*number = flexibleNumber(numeric)
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
	*number = flexibleNumber(parsed)
	return nil
}
