package ceair

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
	adapterName       = "ceair"
	defaultBaseURL    = "https://www.ceair.com"
	defaultLanguage   = "zh"
	defaultCurrency   = "CNY"
	defaultRouteType  = "OW"
	defaultUserAgent  = "Mozilla/5.0 (compatible; Poised/0.1; +https://github.com/zinan-c/Poised)"
	briefInfoEndpoint = "/portal/v3/shopping/briefInfo"
)

type Adapter struct {
	clientFactory func(payload Payload) (*airfare.JSONClient, error)
	now           func() time.Time
}

type Payload struct {
	RouteName      string   `json:"route_name"`
	BaseURL        string   `json:"base_url"`
	ShoppingURL    string   `json:"shopping_url"`
	SkipWarmup     bool     `json:"skip_warmup"`
	LanguageCode   string   `json:"language_code"`
	CurrencyCode   string   `json:"currency_code"`
	AdultCount     int      `json:"adult_count"`
	ChildCount     int      `json:"child_count"`
	InfantCount    int      `json:"infant_count"`
	DepCityCode    string   `json:"dep_city_code"`
	DepCode        []string `json:"dep_code"`
	DepStationCode []string `json:"dep_station_code"`
	ArrCityCode    string   `json:"arr_city_code"`
	ArrCode        []string `json:"arr_code"`
	ArrStationCode []string `json:"arr_station_code"`
	DepDate        string   `json:"dep_date"`
	ArrDate        string   `json:"arr_date"`
	RouteType      string   `json:"route_type"`
	OnlyPlaneFlag  bool     `json:"only_plane_flag"`
	CabinLevel     string   `json:"cabin_level"`
	HideModal      bool     `json:"hide_modal"`
}

type briefInfoRequest struct {
	AdultCount     int    `json:"adultCount"`
	ArrCityCode    string `json:"arrCityCode"`
	ArrCode        string `json:"arrCode"`
	ArrStationCode string `json:"arrStationCode"`
	ArrDate        string `json:"arrDate"`
	ChildCount     int    `json:"childCount"`
	DepCityCode    string `json:"depCityCode"`
	DepCode        string `json:"depCode"`
	DepStationCode string `json:"depStationCode"`
	DepDate        string `json:"depDate"`
	InfantCount    int    `json:"infantCount"`
	RouteType      string `json:"routeType"`
	OnlyPlaneFlag  bool   `json:"onlyPlaneFlag"`
	CabinLevel     string `json:"cabinLevel"`
	HideModal      bool   `json:"hideModal"`
	VerifyURL      string `json:"verifyUrl"`
}

type briefInfoResponse struct {
	ResultCode   string         `json:"resultCode"`
	ResultMsg    string         `json:"resultMsg"`
	CurrencyCode string         `json:"currencyCode"`
	Data         briefInfoData  `json:"data"`
	Raw          map[string]any `json:"-"`
}

type briefInfoData struct {
	FlightItems  []flightItem `json:"flightItems"`
	ProductInfos []any        `json:"productInfos"`
}

type flightItem struct {
	FlightInfos    []flightInfo    `json:"flightInfos"`
	CabinInfoDescs []cabinInfoDesc `json:"cabinInfoDescs"`
}

type flightInfo struct {
	FlightSegments []flightSegment `json:"flightSegments"`
	FlightSort     flightSort      `json:"flightSort"`
}

type flightSegment struct {
	OrgCode         string `json:"orgCode"`
	DestCode        string `json:"destCode"`
	AirlineCode     string `json:"airlineCode"`
	AirlineCodeName string `json:"airlineCodeName"`
	FlightNo        string `json:"flightNo"`
	FltDate         string `json:"fltDate"`
	OrgTime         string `json:"orgTime"`
	ArriDate        string `json:"arriDate"`
	DestTime        string `json:"destTime"`
	FltSpanTime     string `json:"fltSpanTime"`
	PlaneType       string `json:"planeType"`
}

type flightSort struct {
	Price        float64 `json:"price"`
	PriceWithTax float64 `json:"priceWithTax"`
	Duration     int     `json:"duration"`
}

type cabinInfoDesc struct {
	CCode            string         `json:"ccode"`
	CType            string         `json:"ctype"`
	MUCabinLevel     string         `json:"muCabinLevel"`
	CabinLevelName   string         `json:"cabinLevelName"`
	FareInfoDescList []fareInfoDesc `json:"fareInfoDescList"`
}

type fareInfoDesc struct {
	PaxType     string  `json:"paxType"`
	LPrice      float64 `json:"lprice"`
	TaxPrice    float64 `json:"taxPrice"`
	TotalPrice  float64 `json:"totalPrice"`
	PriceSource string  `json:"priceSource"`
	ProductCode string  `json:"productCode"`
	BrandLevel  string  `json:"brandLevel"`
	FareTP      string  `json:"fareTp"`
}

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

	shoppingURL := payload.shoppingURL()
	headers := payload.headers(shoppingURL)
	if !payload.SkipWarmup {
		if _, _, err := client.Get(ctx, shoppingURL, headers); err != nil {
			return core.RunResult{
				Status:  core.RunStatusFailed,
				Summary: "ceair shopping page warmup failed",
				Data: map[string]any{
					"route_name":   payload.RouteName,
					"shopping_url": shoppingURL,
				},
			}, err
		}
	}

	requestPayload := payload.briefInfoRequest()
	_, responseBody, err := client.PostJSON(ctx, briefInfoEndpoint, requestPayload, headers)
	if err != nil {
		return core.RunResult{
			Status:  core.RunStatusFailed,
			Summary: "ceair briefInfo request failed",
			Data: map[string]any{
				"route_name": payload.RouteName,
				"endpoint":   briefInfoEndpoint,
			},
		}, err
	}

	var response briefInfoResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return core.RunResult{Status: core.RunStatusFailed, Summary: "ceair response parse failed"}, err
	}
	if response.ResultCode != "" && response.ResultCode != "S200" {
		return core.RunResult{
			Status:  core.RunStatusFailed,
			Summary: fmt.Sprintf("ceair returned %s", response.ResultCode),
			Data: map[string]any{
				"result_code": response.ResultCode,
				"result_msg":  response.ResultMsg,
			},
		}, nil
	}

	observations := adapter.observations(payload, response)
	status := core.RunStatusSuccess
	summary := fmt.Sprintf("ceair fare search found %d observations", len(observations))
	if len(observations) == 0 {
		status = core.RunStatusFailed
		summary = "ceair fare search returned no observations"
	}
	data := map[string]any{
		"route_name":        payload.RouteName,
		"result_code":       response.ResultCode,
		"result_msg":        response.ResultMsg,
		"currency":          response.currency(payload),
		"flight_item_count": len(response.Data.FlightItems),
		"observation_count": len(observations),
		"observations":      observations,
	}
	if lowest, ok := airfare.LowestTotalPrice(observations); ok {
		summary = fmt.Sprintf("ceair lowest fare %.2f %s on %s", lowest.TotalPrice, lowest.Currency, lowest.FlightNo)
		data["lowest"] = lowest
	}

	return core.RunResult{
		Status:  status,
		Summary: summary,
		Data:    data,
	}, nil
}

func (adapter *Adapter) observations(payload Payload, response briefInfoResponse) []airfare.PriceObservation {
	observedAt := adapter.now().UTC()
	currency := response.currency(payload)
	observations := make([]airfare.PriceObservation, 0)
	for _, item := range response.Data.FlightItems {
		segment, sort, ok := firstSegment(item)
		if !ok {
			continue
		}
		for _, cabin := range item.CabinInfoDescs {
			for _, fare := range cabin.FareInfoDescList {
				totalPrice := fare.TotalPrice
				if totalPrice == 0 && (fare.LPrice > 0 || fare.TaxPrice > 0) {
					totalPrice = fare.LPrice + fare.TaxPrice
				}
				if totalPrice == 0 && sort.PriceWithTax > 0 {
					totalPrice = sort.PriceWithTax
				}
				observations = append(observations, airfare.PriceObservation{
					Adapter:         adapterName,
					RouteName:       payload.RouteName,
					Airline:         firstNonEmpty(segment.AirlineCode, segment.AirlineCodeName),
					FlightNo:        segment.FlightNo,
					Origin:          segment.OrgCode,
					Destination:     segment.DestCode,
					DepartureTime:   combineDateTime(segment.FltDate, segment.OrgTime),
					ArrivalTime:     combineDateTime(segment.ArriDate, segment.DestTime),
					DurationMinutes: firstPositive(parseInt(segment.FltSpanTime), sort.Duration),
					AircraftType:    segment.PlaneType,
					CabinCode:       firstNonEmpty(cabin.CCode, cabin.MUCabinLevel),
					CabinName:       firstNonEmpty(cabin.CabinLevelName, cabin.CType),
					BasePrice:       firstPositiveFloat(fare.LPrice, sort.Price),
					TaxPrice:        fare.TaxPrice,
					TotalPrice:      totalPrice,
					Currency:        currency,
					RawProductCode:  fare.ProductCode,
					RawPriceSource:  fare.PriceSource,
					ObservedAt:      observedAt,
				})
			}
		}
	}
	return observations
}

func newClient(payload Payload) (*airfare.JSONClient, error) {
	return airfare.NewJSONClient(payload.BaseURL, nil, map[string]string{
		"Accept":     "application/json, text/plain, */*",
		"User-Agent": defaultUserAgent,
	})
}

func (payload *Payload) withDefaults() {
	if payload.BaseURL == "" {
		payload.BaseURL = defaultBaseURL
	}
	if payload.LanguageCode == "" {
		payload.LanguageCode = defaultLanguage
	}
	if payload.CurrencyCode == "" {
		payload.CurrencyCode = defaultCurrency
	}
	if payload.RouteType == "" {
		payload.RouteType = defaultRouteType
	}
	if payload.AdultCount == 0 {
		payload.AdultCount = 1
	}
	payload.HideModal = true
}

func (payload Payload) validate() error {
	if payload.DepCityCode == "" {
		return fmt.Errorf("dep_city_code is required")
	}
	if payload.ArrCityCode == "" {
		return fmt.Errorf("arr_city_code is required")
	}
	if len(payload.DepCode) == 0 && len(payload.DepStationCode) == 0 {
		return fmt.Errorf("dep_code or dep_station_code is required")
	}
	if len(payload.ArrCode) == 0 && len(payload.ArrStationCode) == 0 {
		return fmt.Errorf("arr_code or arr_station_code is required")
	}
	if payload.DepDate == "" {
		return fmt.Errorf("dep_date is required")
	}
	return nil
}

func (payload Payload) headers(referer string) map[string]string {
	return map[string]string{
		"Content-Type": "application/json; charset=utf-8",
		"Origin":       strings.TrimRight(payload.BaseURL, "/"),
		"Referer":      referer,
		"languageCode": payload.LanguageCode,
		"currencyCode": payload.CurrencyCode,
		"platform":     "",
	}
}

func (payload Payload) briefInfoRequest() briefInfoRequest {
	return briefInfoRequest{
		AdultCount:     payload.AdultCount,
		ArrCityCode:    payload.ArrCityCode,
		ArrCode:        strings.Join(payload.ArrCode, ","),
		ArrStationCode: strings.Join(payload.ArrStationCode, ","),
		ArrDate:        payload.ArrDate,
		ChildCount:     payload.ChildCount,
		DepCityCode:    payload.DepCityCode,
		DepCode:        strings.Join(payload.DepCode, ","),
		DepStationCode: strings.Join(payload.DepStationCode, ","),
		DepDate:        payload.DepDate,
		InfantCount:    payload.InfantCount,
		RouteType:      payload.RouteType,
		OnlyPlaneFlag:  payload.OnlyPlaneFlag,
		CabinLevel:     payload.CabinLevel,
		HideModal:      payload.HideModal,
		VerifyURL:      fmt.Sprintf("%s?%s%s%s", briefInfoEndpoint, payload.DepCityCode, payload.ArrCityCode, payload.DepDate),
	}
}

func (payload Payload) shoppingURL() string {
	if payload.ShoppingURL != "" {
		return payload.ShoppingURL
	}
	routeKind := "oneway"
	if payload.RouteType == "RT" {
		routeKind = "roundtrip"
	}
	origin := encodeLocation(payload.DepCode, payload.DepStationCode)
	destination := encodeLocation(payload.ArrCode, payload.ArrStationCode)
	currency := strings.ToLower(payload.CurrencyCode)
	return fmt.Sprintf("%s/%s/%s/shopping/%s/%s-%s/%s.",
		strings.TrimRight(payload.BaseURL, "/"),
		payload.LanguageCode,
		currency,
		routeKind,
		origin,
		destination,
		payload.DepDate,
	)
}

func encodeLocation(codes []string, stationCodes []string) string {
	values := make([]string, 0, len(codes)+len(stationCodes))
	values = append(values, codes...)
	for _, code := range stationCodes {
		values = append(values, code+"(R)")
	}
	return strings.Join(values, ",")
}

func (response briefInfoResponse) currency(payload Payload) string {
	if response.CurrencyCode != "" {
		return response.CurrencyCode
	}
	return payload.CurrencyCode
}

func firstSegment(item flightItem) (flightSegment, flightSort, bool) {
	for _, info := range item.FlightInfos {
		if len(info.FlightSegments) > 0 {
			return info.FlightSegments[0], info.FlightSort, true
		}
	}
	return flightSegment{}, flightSort{}, false
}

func combineDateTime(date string, timeValue string) string {
	if date == "" {
		return timeValue
	}
	if timeValue == "" {
		return date
	}
	return date + " " + timeValue
}

func parseInt(value string) int {
	parsed, err := strconv.Atoi(value)
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

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func firstPositiveFloat(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
